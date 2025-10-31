package workers

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
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
	"github.com/branchd-dev/branchd/internal/tasks"
)

//go:embed pg_dump_restore.sh
var pgDumpRestoreScript string

type pgDumpRestoreParams struct {
	ConnectionString string
	PgVersion        string
	PgPort           int
	DatabaseName     string
	SchemaOnly       bool
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

	// Render and execute restore script
	scriptParams := pgDumpRestoreParams{
		ConnectionString: configModel.ConnectionString,
		PgVersion:        configModel.PostgresVersion,
		PgPort:           postgresVersionToPort(configModel.PostgresVersion),
		DatabaseName:     database.Name,
		SchemaOnly:       database.SchemaOnly,
	}

	script, err := renderPgDumpRestoreScript(scriptParams)
	if err != nil {
		return fmt.Errorf("failed to render pg_dump restore script: %w", err)
	}

	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	if err != nil {
		logger.Error().Err(err).Str("output", output).Msg("pg_dump/restore script failed")
		return fmt.Errorf("pg_dump/restore script execution failed: %w", err)
	}

	logger.Info().
		Str("restore_id", database.ID).
		Bool("schema_only", database.SchemaOnly).
		Msg("Restore process started in background - enqueueing completion check")

	// Enqueue WaitComplete task to poll for completion (every 10 seconds)
	waitTask, err := tasks.NewPgDumpRestoreWaitCompleteTask(database.ID)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create wait complete task")
		return fmt.Errorf("failed to create wait complete task: %w", err)
	}

	// Schedule first check in 10 seconds
	// MaxRetry set high to support long-running restores (4320 retries = 12 hours at 10s intervals)
	_, err = client.Enqueue(waitTask,
		asynq.ProcessIn(10*time.Second),
		asynq.MaxRetry(4320),
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
// It does NOT create databases or trigger new restores (that's done in Execute handler)
func HandlePgDumpRestoreWaitComplete(ctx context.Context, t *asynq.Task, client *asynq.Client, db *gorm.DB, logger zerolog.Logger) error {
	payload, err := tasks.ParseTaskPayload(t)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	// Load restore record
	var restore models.Restore
	if err := db.Where("id = ?", payload.RestoreID).First(&restore).Error; err != nil {
		return fmt.Errorf("failed to load database: %w", err)
	}

	// Load config (singleton)
	var config models.Config
	if err := db.First(&config).Error; err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger.Info().
		Str("restore_id", restore.ID).
		Str("restore_name", restore.Name).
		Bool("schema_only", restore.SchemaOnly).
		Msg("Checking pg_dump/restore status")

	// Check if restore is still running
	isRunning, err := isRestoreProcessRunning(ctx, &restore, logger)
	if err != nil {
		return fmt.Errorf("failed to check if restore process is running: %w", err)
	}

	if isRunning {
		logger.Debug().
			Str("restore_id", restore.ID).
			Msg("Restore still running - scheduling next check in 10 seconds")

		// Enqueue another WaitComplete task to check again in 10 seconds
		waitTask, err := tasks.NewPgDumpRestoreWaitCompleteTask(restore.ID)
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
	status, logTail, err := getRestoreResult(ctx, &restore, logger)
	if err != nil {
		return fmt.Errorf("failed to get restore result: %w", err)
	}

	switch status {
	case "success":
		logger.Info().
			Str("restore_id", restore.ID).
			Msg("Restore completed successfully")

		// Apply anonymization rules before marking as ready
		if err := applyAnonymizationRules(ctx, db, &config, &restore, logger); err != nil {
			logger.Error().Err(err).Msg("Failed to apply anonymization rules")
			return fmt.Errorf("failed to apply anonymization rules: %w", err)
		}

		// Mark database as ready
		// For full databases, set ReadyAt after anonymization completes
		// For schema-only, set ReadyAt now (after restore, though anon rules were also applied)
		now := time.Now()
		updates := map[string]interface{}{
			"schema_ready": true,
			"ready_at":     now,
		}
		if !restore.SchemaOnly {
			updates["data_ready"] = true
		}

		if err := db.Model(&restore).Updates(updates).Error; err != nil {
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
		// Exclude the restore that just completed to ensure users can create branches from it
		if err := cleanupStaleRestores(ctx, db, restore.ID, logger); err != nil {
			logger.Warn().Err(err).Msg("Failed to cleanup stale restores (non-fatal)")
			// Don't fail the task if cleanup fails - just log the warning
		}

		return nil

	case "failed":
		logger.Error().
			Str("restore_id", restore.ID).
			Str("log_tail", logTail).
			Msg("Restore failed")
		return fmt.Errorf("restore failed - log tail: %s", logTail)

	default:
		logger.Error().
			Str("restore_id", restore.ID).
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

// isRestoreProcessRunning checks if the pg_dump/restore process is still running
func isRestoreProcessRunning(ctx context.Context, database *models.Restore, logger zerolog.Logger) (bool, error) {
	pidFile := fmt.Sprintf("/var/log/branchd/restore-%s.pid", database.Name)

	// Check if PID file exists and process is running
	cmd := fmt.Sprintf("if [ -f %s ]; then pid=$(cat %s); if kill -0 $pid 2>/dev/null; then echo 'running'; else echo 'stopped'; fi; else echo 'not_found'; fi", pidFile, pidFile)

	execCmd := exec.CommandContext(ctx, "bash", "-c", cmd)
	outputBytes, err := execCmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("failed to check restore process: %w", err)
	}

	outputTrimmed := bytes.TrimSpace(outputBytes)
	isRunning := string(outputTrimmed) == "running"

	logger.Debug().
		Str("database_name", database.Name).
		Bool("is_running", isRunning).
		Str("output", string(outputTrimmed)).
		Msg("Checked restore process status")

	return isRunning, nil
}

// getRestoreResult reads the restore log to determine success/failure
// Returns: (status, logTail, error)
func getRestoreResult(ctx context.Context, database *models.Restore, logger zerolog.Logger) (string, string, error) {
	logFile := fmt.Sprintf("/var/log/branchd/restore-%s.log", database.Name)

	// Check for success marker first (most common case after restore completes)
	successCmd := fmt.Sprintf("grep -q '__BRANCHD_RESTORE_SUCCESS__' %s 2>/dev/null && echo 'success' || true", logFile)
	successExecCmd := exec.CommandContext(ctx, "bash", "-c", successCmd)
	successOutputBytes, successErr := successExecCmd.CombinedOutput()
	successOutput := string(successOutputBytes)
	if successErr == nil && strings.TrimSpace(successOutput) == "success" {
		logger.Debug().
			Str("database_name", database.Name).
			Msg("Found success marker in restore log")
		return "success", "", nil
	}

	// Check for failure marker
	failureCmd := fmt.Sprintf("grep -q '__BRANCHD_RESTORE_FAILED__' %s 2>/dev/null && echo 'failed' || true", logFile)
	failureExecCmd := exec.CommandContext(ctx, "bash", "-c", failureCmd)
	failureOutputBytes, failureErr := failureExecCmd.CombinedOutput()
	failureOutput := string(failureOutputBytes)
	if failureErr == nil && strings.TrimSpace(failureOutput) == "failed" {
		logger.Debug().
			Str("database_name", database.Name).
			Msg("Found failure marker in restore log")
		// Read log tail for failure case
		tailCmd := fmt.Sprintf("tail -n 50 %s 2>&1 || echo 'Failed to read log'", logFile)
		tailExecCmd := exec.CommandContext(ctx, "bash", "-c", tailCmd)
		logTailBytes, _ := tailExecCmd.CombinedOutput()
		logTail := string(logTailBytes)
		return "failed", strings.TrimSpace(logTail), nil
	}

	// Check if log file exists
	checkCmd := fmt.Sprintf("test -f %s && echo 'unknown' || echo 'not_found'", logFile)
	checkExecCmd := exec.CommandContext(ctx, "bash", "-c", checkCmd)
	outputBytes, err := checkExecCmd.CombinedOutput()
	output := string(outputBytes)
	if err != nil {
		logger.Error().
			Err(err).
			Str("database_name", database.Name).
			Str("log_file", logFile).
			Msg("Failed to check if restore log exists")
		return "", "", fmt.Errorf("failed to check restore log: %w", err)
	}

	result := strings.TrimSpace(output)

	logger.Debug().
		Str("database_name", database.Name).
		Str("result", result).
		Msg("Restore log exists but no success/failure marker found yet")

	// If status is unknown or failed, read the log tail for debugging
	var logTail string
	if result == "unknown" || result == "failed" {
		tailCmd := fmt.Sprintf("tail -n 50 %s 2>&1 || echo 'Failed to read log'", logFile)
		tailExecCmd := exec.CommandContext(ctx, "bash", "-c", tailCmd)
		logTailBytes, err := tailExecCmd.CombinedOutput()
		if err != nil {
			logger.Warn().Err(err).Msg("Failed to read restore log tail")
		} else {
			logTail = strings.TrimSpace(string(logTailBytes))
		}
	}

	logger.Debug().
		Str("database_name", database.Name).
		Str("restore_result", result).
		Str("log_tail_length", fmt.Sprintf("%d", len(logTail))).
		Msg("Got restore result")

	return result, logTail, nil
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
	// Must specify port since each PostgreSQL version runs on a different port
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
