package workers

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
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

//go:embed pg_dump_restore.sh
var pgDumpRestoreScript string

type pgDumpRestoreParams struct {
	ConnectionString string
	PgVersion        string
	PgPort           int
	DatabaseName     string
	SchemaOnly       string // "true" or "false" for template
	ParallelJobs     int
	DumpDir          string // Directory for pg_dump output (on EBS zpool)

	// PostgreSQL tuning parameters
	TuneSQL  []string // SQL statements to apply tuning
	ResetSQL []string // SQL statements to reset tuning
}

// postgresVersionToPort maps PostgreSQL major version to its port
func postgresVersionToPort(version string) int {
	switch version {
	case "14":
		return 5414
	case "15":
		return 5415
	case "16":
		return 5416
	case "17":
		return 5417
	default:
		// Default to 5432 for unknown versions
		return 5432
	}
}

// HandlePgDumpRestoreExecute starts the pg_dump/restore process for a database
// The database record should already exist before this handler is called
func HandlePgDumpRestoreExecute(ctx context.Context, t *asynq.Task, client *asynq.Client, db *gorm.DB, cfg *config.Config, logger zerolog.Logger) error {
	payload, err := tasks.ParseTaskPayload(t)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	// Load database
	var database models.Restore
	if err := db.Where("id = ?", payload.RestoreID).First(&database).Error; err != nil {
		return fmt.Errorf("failed to load database: %w", err)
	}

	// Load config (singleton)
	var configModel models.Config
	if err := db.First(&configModel).Error; err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger.Info().
		Str("restore_id", database.ID).
		Bool("schema_only", database.SchemaOnly).
		Msg("Starting pg_dump/restore process")

	// Create orchestrator
	orchestrator := restore.NewOrchestrator(logger)

	// Calculate port
	pgPort := postgresVersionToPort(configModel.PostgresVersion)

	// 1. Validate inputs
	if err := orchestrator.ValidateInputs(
		configModel.ConnectionString,
		configModel.PostgresVersion,
		pgPort,
		database.Name,
	); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// 2. Create log directory
	if err := orchestrator.CreateLogDirectory(ctx); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// 3. Check if restore is already in progress
	isRunning, pid, err := orchestrator.CheckIfRestoreInProgress(ctx, database.Name)
	if err != nil {
		return fmt.Errorf("failed to check restore status: %w", err)
	}

	if isRunning {
		logger.Info().
			Int("pid", pid).
			Str("restore_id", database.ID).
			Msg("Restore is already running, skipping")

		// Enqueue wait task to monitor existing restore
		waitTask, err := tasks.NewPgDumpRestoreWaitCompleteTask(database.ID)
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

	logger.Info().
		Int("cpu_cores", resources.CPUCores).
		Int("parallel_jobs", tuning.ParallelJobs).
		Str("maintenance_work_mem", tuning.MaintenanceWorkMem).
		Str("max_wal_size", tuning.MaxWalSize).
		Msg("Calculated optimal restore settings")

	// 5. Calculate dump directory path (on zpool EBS volume)
	// Use restore name which follows pattern: restore_{datetime}
	dumpDir := fmt.Sprintf("/opt/branchd/%s", database.Name)

	// 6. Render and execute restore script
	schemaOnlyStr := "false"
	if database.SchemaOnly {
		schemaOnlyStr = "true"
	}

	scriptParams := pgDumpRestoreParams{
		ConnectionString: configModel.ConnectionString,
		PgVersion:        configModel.PostgresVersion,
		PgPort:           pgPort,
		DatabaseName:     database.Name,
		SchemaOnly:       schemaOnlyStr,
		ParallelJobs:     tuning.ParallelJobs,
		DumpDir:          dumpDir,
		TuneSQL:          tuning.GenerateAlterSystemSQL(),
		ResetSQL:         pgtuning.GenerateResetSQL(),
	}

	script, err := renderPgDumpRestoreScript(scriptParams)
	if err != nil {
		return fmt.Errorf("failed to render pg_dump restore script: %w", err)
	}

	// Start the restore script in background using nohup
	logFile := orchestrator.GetLogFilePath(database.Name)
	pidFile := orchestrator.GetPIDFilePath(database.Name)

	// Write script to a temporary file to avoid shell quoting issues
	scriptPath := fmt.Sprintf("/tmp/branchd_restore_%s.sh", database.Name)
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

	logger.Info().
		Str("restore_id", database.ID).
		Bool("schema_only", database.SchemaOnly).
		Int("parallel_jobs", tuning.ParallelJobs).
		Int("data_phase_jobs", tuning.ParallelJobs+2).
		Str("dump_dir", dumpDir).
		Msg("Three-phase restore started in background - data phase will use +2 workers")

	// 7. Enqueue WaitComplete task to poll for completion
	waitTask, err := tasks.NewPgDumpRestoreWaitCompleteTask(database.ID)
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
		Str("restore_id", database.ID).
		Msg("Restore execute task completed - waiting for background process")

	return nil
}

// HandlePgDumpRestoreWaitComplete polls for pg_dump/restore completion
// This handler only waits for restore to complete and marks database as ready
func HandlePgDumpRestoreWaitComplete(ctx context.Context, t *asynq.Task, client *asynq.Client, db *gorm.DB, logger zerolog.Logger) error {
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
		Msg("Checking pg_dump/restore status")

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
		waitTask, err := tasks.NewPgDumpRestoreWaitCompleteTask(restoreModel.ID)
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
		if err := applyAnonymizationRules(ctx, db, &config, &restoreModel, logger); err != nil {
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

func renderPgDumpRestoreScript(params pgDumpRestoreParams) (string, error) {
	tmpl, err := template.New("pg-dump-restore").Parse(pgDumpRestoreScript)
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

// applyAnonymizationRules applies configured anonymization rules to a restored database
func applyAnonymizationRules(ctx context.Context, db *gorm.DB, config *models.Config, database *models.Restore, logger zerolog.Logger) error {
	// Load global anonymization rules
	var rules []models.AnonRule
	if err := db.Find(&rules).Error; err != nil {
		return fmt.Errorf("failed to load anon rules: %w", err)
	}

	if len(rules) == 0 {
		logger.Info().
			Str("restore_id", database.ID).
			Msg("No anonymization rules configured, skipping")
		return nil
	}

	logger.Info().
		Str("restore_id", database.ID).
		Int("rule_count", len(rules)).
		Msg("Applying anonymization rules")

	// Generate SQL from rules
	sql := anonymize.GenerateSQL(rules)
	if sql == "" {
		logger.Warn().Msg("Generated empty SQL from rules")
		return nil
	}

	// Execute anonymization SQL on the database
	pgPort := postgresVersionToPort(config.PostgresVersion)
	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

DATABASE_NAME="%s"
PG_VERSION="%s"
PG_PORT="%d"
PG_BIN="/usr/lib/postgresql/${PG_VERSION}/bin"

echo "Applying anonymization rules to database ${DATABASE_NAME}"

# Execute anonymization SQL with correct port
sudo -u postgres ${PG_BIN}/psql -p ${PG_PORT} -d "${DATABASE_NAME}" <<'ANONYMIZE_SQL'
%s
ANONYMIZE_SQL

echo "Anonymization completed successfully"
`, database.Name, config.PostgresVersion, pgPort, sql)

	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	if err != nil {
		logger.Error().
			Err(err).
			Str("output", output).
			Str("database_name", database.Name).
			Msg("Failed to execute anonymization script")
		return fmt.Errorf("anonymization script execution failed: %w", err)
	}

	logger.Info().
		Str("database_name", database.Name).
		Int("rule_count", len(rules)).
		Str("output", output).
		Msg("Anonymization rules applied successfully")

	return nil
}
