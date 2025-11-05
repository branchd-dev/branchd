package workers

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/hibiken/asynq"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/branchd-dev/branchd/internal/anonymize"
	"github.com/branchd-dev/branchd/internal/config"
	"github.com/branchd-dev/branchd/internal/models"
	"github.com/branchd-dev/branchd/internal/pgtuning"
	"github.com/branchd-dev/branchd/internal/restore"
	"github.com/branchd-dev/branchd/internal/sysinfo"
	"github.com/branchd-dev/branchd/internal/tasks"
)

//go:embed logical_restore.sh
var logicalRestoreScript string

type logicalRestoreParams struct {
	ConnectionString string
	PgVersion        string
	PgPort           int // Dynamic port for this restore's cluster
	DatabaseName     string
	SchemaOnly       string // "true" or "false" for template
	ParallelJobs     int
	DumpDir          string // Directory for pg_dump output (on EBS zpool)
	DataDir          string // PostgreSQL data directory for initdb

	// PostgreSQL tuning parameters
	TuneSQL  []string // SQL statements to apply tuning
	ResetSQL []string // SQL statements to reset tuning
}

// findAvailablePort finds an available port above 50000 for a new restore cluster
func findAvailablePort(ctx context.Context, logger zerolog.Logger) (int, error) {
	for port := 50000; port < 60000; port++ {
		cmd := exec.CommandContext(ctx, "bash", "-c", fmt.Sprintf("ss -ln | grep -q ':%d ' && echo 'in_use' || echo 'available'", port))
		output, err := cmd.Output()
		if err != nil {
			continue
		}

		if strings.TrimSpace(string(output)) == "available" {
			logger.Debug().Int("port", port).Msg("Found available port")
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available ports in range 50000-60000")
}

// HandleTriggerLogicalRestore starts the logical restore process for a database
// The database record should already exist before this handler is called
func HandleTriggerLogicalRestore(ctx context.Context, t *asynq.Task, client *asynq.Client, db *gorm.DB, cfg *config.Config, logger zerolog.Logger) error {
	payload, err := tasks.ParseTaskPayload(t)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	// Load dbRestore
	var dbRestore models.Restore
	if err := db.Where("id = ?", payload.RestoreID).First(&dbRestore).Error; err != nil {
		return fmt.Errorf("failed to load database: %w", err)
	}

	// Load config
	var configModel models.Config
	if err := db.First(&configModel).Error; err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger.Info().
		Str("restore_id", dbRestore.ID).
		Bool("schema_only", dbRestore.SchemaOnly).
		Msg("Starting restore process")

	// Create orchestrator
	orchestrator := restore.NewOrchestrator(logger)

	// 1. Find available port for this restore's PostgreSQL cluster
	pgPort, err := findAvailablePort(ctx, logger)
	if err != nil {
		return fmt.Errorf("failed to find available port: %w", err)
	}

	// Store port in database
	if err := db.Model(&dbRestore).Update("port", pgPort).Error; err != nil {
		return fmt.Errorf("failed to store port in database: %w", err)
	}

	// 2. Validate inputs
	if err := orchestrator.ValidateInputs(
		configModel.ConnectionString,
		configModel.PostgresVersion,
		pgPort,
		dbRestore.Name,
	); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// 2. Create log directory
	if err := orchestrator.CreateLogDirectory(ctx); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// 3. Check if restore is already in progress
	isRunning, pid, err := orchestrator.CheckIfRestoreInProgress(ctx, dbRestore.Name)
	if err != nil {
		return fmt.Errorf("failed to check restore status: %w", err)
	}

	if isRunning {
		logger.Info().
			Int("pid", pid).
			Str("restore_id", dbRestore.ID).
			Msg("Restore is already running, skipping")

		// Enqueue wait task to monitor existing restore
		waitTask, err := tasks.NewTriggerLogicalRestoreWaitCompleteTask(dbRestore.ID)
		if err != nil {
			return fmt.Errorf("failed to create wait complete task: %w", err)
		}

		_, err = client.Enqueue(waitTask,
			asynq.ProcessIn(10*time.Second),
			asynq.MaxRetry(4320), // Support up to 12 hours
		)
		if err != nil {
			return fmt.Errorf("failed to enqueue wait complete task: %w", err)
		}

		return nil
	}

	// 4. Detect system resources and calculate optimal settings
	resources, err := sysinfo.GetResources()
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to detect system resources, using defaults")
	}

	tuning := pgtuning.CalculateOptimalSettings(resources)

	// 5. Calculate paths for restore cluster
	// Each restore gets its own ZFS dataset: tank/restore_YYYYMMDDHHMMSS
	// Mounted at: /opt/branchd/restore_YYYYMMDDHHMMSS
	restoreDatasetPath := fmt.Sprintf("/opt/branchd/%s", dbRestore.Name)
	dataDir := fmt.Sprintf("%s/data", restoreDatasetPath)        // PostgreSQL data directory
	dumpDir := fmt.Sprintf("%s/dump.pgdump", restoreDatasetPath) // pg_dump output file

	// 6. Render and execute restore script
	schemaOnlyStr := "false"
	if dbRestore.SchemaOnly {
		schemaOnlyStr = "true"
	}

	scriptParams := logicalRestoreParams{
		ConnectionString: configModel.ConnectionString,
		PgVersion:        configModel.PostgresVersion,
		PgPort:           pgPort,
		DatabaseName:     dbRestore.Name,
		SchemaOnly:       schemaOnlyStr,
		ParallelJobs:     tuning.ParallelJobs,
		DumpDir:          dumpDir,
		DataDir:          dataDir,
		TuneSQL:          tuning.GenerateAlterSystemSQL(),
		ResetSQL:         pgtuning.GenerateResetSQL(),
	}

	script, err := renderLogicalRestoreScript(scriptParams)
	if err != nil {
		return fmt.Errorf("failed to render logical restore script: %w", err)
	}

	// Start the restore script in background using nohup
	logFile := orchestrator.GetLogFilePath(dbRestore.Name)
	pidFile := orchestrator.GetPIDFilePath(dbRestore.Name)

	// Write script to a temporary file to avoid shell quoting issues
	scriptPath := fmt.Sprintf("/tmp/branchd_restore_%s.sh", dbRestore.Name)
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("failed to write restore script: %w", err)
	}

	// Create a wrapper script that runs the restore in background and cleans up the temp file
	wrapperScript := fmt.Sprintf(`
		nohup bash -c 'bash "%s"; rm -f "%s"' > "%s" 2>&1 &
		echo $! > "%s"
	`, scriptPath, scriptPath, logFile, pidFile)

	cmd := exec.CommandContext(ctx, "bash", "-c", wrapperScript)
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	if err != nil {
		logger.Error().Err(err).Str("output", output).Msg("Failed to start restore script")
		return fmt.Errorf("restore script execution failed: %w", err)
	}

	// 7. Enqueue WaitComplete task to poll for completion
	waitTask, err := tasks.NewTriggerLogicalRestoreWaitCompleteTask(dbRestore.ID)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create wait complete task")
		return fmt.Errorf("failed to create wait complete task: %w", err)
	}

	// Schedule first check in 10 seconds
	_, err = client.Enqueue(waitTask,
		asynq.ProcessIn(10*time.Second),
		asynq.MaxRetry(4320), // 12 hours at 10s intervals
	)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to enqueue wait complete task")
		return fmt.Errorf("failed to enqueue wait complete task: %w", err)
	}

	logger.Info().
		Str("restore_id", dbRestore.ID).
		Msg("Restore triggered")

	return nil
}

// HandleLogicalRestoreWaitComplete polls for logical restore completion
// This handler only waits for restore to complete and marks database as ready
func HandleLogicalRestoreWaitComplete(ctx context.Context, t *asynq.Task, client *asynq.Client, db *gorm.DB, logger zerolog.Logger) error {
	payload, err := tasks.ParseTaskPayload(t)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	// Load restore record
	var restoreModel models.Restore
	if err := db.Where("id = ?", payload.RestoreID).First(&restoreModel).Error; err != nil {
		return fmt.Errorf("failed to load database: %w", err)
	}

	// Load config (singleton)
	var config models.Config
	if err := db.First(&config).Error; err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger.Info().
		Str("restore_id", restoreModel.ID).
		Str("restore_name", restoreModel.Name).
		Bool("schema_only", restoreModel.SchemaOnly).
		Msg("Checking logical restore status")

	// Create orchestrator
	orchestrator := restore.NewOrchestrator(logger)

	// Check if restore is still running
	isRunning, _, err := orchestrator.CheckIfRestoreInProgress(ctx, restoreModel.Name)
	if err != nil {
		return fmt.Errorf("failed to check if restore process is running: %w", err)
	}

	if isRunning {
		logger.Debug().
			Str("restore_id", restoreModel.ID).
			Msg("Restore still running - scheduling next check in 10 seconds")

		// Enqueue another WaitComplete task
		waitTask, err := tasks.NewTriggerLogicalRestoreWaitCompleteTask(restoreModel.ID)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to create wait complete task")
			return fmt.Errorf("failed to create wait complete task: %w", err)
		}

		_, err = client.Enqueue(waitTask,
			asynq.ProcessIn(10*time.Second),
			asynq.MaxRetry(4320), // Support up to 12 hours
		)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to enqueue next wait complete task")
			return fmt.Errorf("failed to enqueue next wait complete task: %w", err)
		}

		// Return success - we've successfully scheduled the next check
		return nil
	}

	// Get restore result
	status, logTail, err := orchestrator.CheckRestoreStatus(ctx, restoreModel.Name)
	if err != nil {
		return fmt.Errorf("failed to get restore result: %w", err)
	}

	switch status {
	case "success":
		logger.Info().
			Str("restore_id", restoreModel.ID).
			Msg("Restore completed successfully")

		// Apply anonymization rules before marking as ready
		// Use the restore's allocated port
		_, err := anonymize.Apply(ctx, db, anonymize.ApplyParams{
			DatabaseName:    restoreModel.Name,
			PostgresVersion: config.PostgresVersion,
			PostgresPort:    restoreModel.Port,
		}, logger)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to apply anonymization rules")
			return fmt.Errorf("failed to apply anonymization rules: %w", err)
		}

		// Mark database as ready
		now := time.Now()
		updates := map[string]interface{}{
			"schema_ready": true,
			"ready_at":     now,
		}
		if !restoreModel.SchemaOnly {
			updates["data_ready"] = true
		}

		if err := db.Model(&restoreModel).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to mark database ready: %w", err)
		}

		// If this is a refresh (db count > 1), update timestamps
		var restoreCount int64
		if err := db.Model(&models.Restore{}).Count(&restoreCount).Error; err != nil {
			logger.Error().Err(err).Msg("Failed to count databases")
		} else if restoreCount > 1 {
			logger.Info().
				Int64("database_count", restoreCount).
				Msg("Refresh completed - updating timestamps")

			// Update refresh timestamps in config
			now := time.Now()
			nextRefresh := calculateNextRefresh(config.RefreshSchedule, now)
			if err := db.Model(&config).Updates(map[string]interface{}{
				"last_refreshed_at": now,
				"next_refresh_at":   nextRefresh,
			}).Error; err != nil {
				logger.Error().Err(err).Msg("Failed to update refresh timestamps")
			}
		}

		// Cleanup stale restores with no branches after successful restore
		if err := cleanupStaleRestores(ctx, db, restoreModel.ID, logger); err != nil {
			logger.Warn().Err(err).Msg("Failed to cleanup stale restores (non-fatal)")
		}

		return nil

	case "failed":
		logger.Error().
			Str("restore_id", restoreModel.ID).
			Str("log_tail", logTail).
			Msg("Restore failed")
		return fmt.Errorf("restore failed - log tail: %s", logTail)

	default:
		logger.Error().
			Str("restore_id", restoreModel.ID).
			Str("status", status).
			Msg("Restore process died without clear result")
		return fmt.Errorf("restore process died - status: %s, log: %s", status, logTail)
	}
}

func renderLogicalRestoreScript(params logicalRestoreParams) (string, error) {
	tmpl, err := template.New("logical-restore").Parse(logicalRestoreScript)
	if err != nil {
		return "", fmt.Errorf("failed to parse script template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("failed to execute script template: %w", err)
	}

	return buf.String(), nil
}

// calculateNextRefresh calculates next refresh time from cron schedule
func calculateNextRefresh(cronExpr string, from time.Time) *time.Time {
	if cronExpr == "" {
		return nil
	}

	// Parse cron expression
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(cronExpr)
	if err != nil {
		return nil
	}

	next := schedule.Next(from)
	return &next
}
