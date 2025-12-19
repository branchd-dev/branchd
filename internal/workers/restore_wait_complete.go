package workers

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/hibiken/asynq"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/branchd-dev/branchd/internal/anonymize"
	"github.com/branchd-dev/branchd/internal/models"
	"github.com/branchd-dev/branchd/internal/restore"
	"github.com/branchd-dev/branchd/internal/tasks"
)

// HandleRestoreWaitComplete polls for restore completion
// This handler is generic and works for all restore providers (logical, Crunchy Bridge, etc.)
func HandleRestoreWaitComplete(ctx context.Context, t *asynq.Task, client *asynq.Client, db *gorm.DB, logger zerolog.Logger) error {
	payload, err := tasks.ParseTaskPayload(t)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	// Load restore record
	var restoreModel models.Restore
	if err := db.Where("id = ?", payload.RestoreID).First(&restoreModel).Error; err != nil {
		return fmt.Errorf("failed to load restore: %w", err)
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
		Msg("Checking restore status")

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

		// Enqueue another wait task
		waitTask, err := tasks.NewTriggerRestoreWaitCompleteTask(restoreModel.ID)
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

		// Determine the target database name based on restore type:
		// - Crunchy Bridge: uses the database name from config (pgBackRest restores all databases)
		// - Logical: uses the source database name from connection string (restored to database with same name as source)
		targetDatabase := config.DatabaseName // Default for logical restores (extracted from connection string)
		if config.CrunchyBridgeAPIKey != "" {
			// Crunchy Bridge restore - use the configured database name
			targetDatabase = config.CrunchyBridgeDatabaseName
		}

		// Execute post-restore SQL if configured (before anonymization)
		if config.PostRestoreSQL != "" {
			if err := executePostRestoreSQL(ctx, config.PostRestoreSQL, targetDatabase, config.PostgresVersion, restoreModel.Port, logger); err != nil {
				logger.Error().Err(err).Msg("Failed to execute post-restore SQL")
				return fmt.Errorf("failed to execute post-restore SQL: %w", err)
			}
		}

		// Apply anonymization rules before marking as ready
		_, err := anonymize.Apply(ctx, db, anonymize.ApplyParams{
			DatabaseName:    targetDatabase,
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
			logger.Error().Err(err).Msg("Failed to count restores")
		} else if restoreCount > 1 {
			logger.Info().
				Int64("restore_count", restoreCount).
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

		// Delete stale restores (restores without branches) after successful restore
		if err := deleteStaleRestores(ctx, db, restoreModel.ID, logger); err != nil {
			logger.Warn().Err(err).Msg("Failed to delete stale restores (non-fatal)")
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

// executePostRestoreSQL executes custom SQL statements after restore completes
// This runs before anonymization rules are applied
func executePostRestoreSQL(ctx context.Context, sql, databaseName, postgresVersion string, port int, logger zerolog.Logger) error {
	logger.Info().
		Str("database_name", databaseName).
		Int("port", port).
		Msg("Executing post-restore SQL")

	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

DATABASE_NAME="%s"
PG_VERSION="%s"
PG_PORT="%d"
PG_BIN="/usr/lib/postgresql/${PG_VERSION}/bin"

echo "Executing post-restore SQL on database ${DATABASE_NAME}"

sudo -u postgres ${PG_BIN}/psql -p ${PG_PORT} -d "${DATABASE_NAME}" <<'POST_RESTORE_SQL'
%s
POST_RESTORE_SQL

echo "Post-restore SQL completed successfully"
`, databaseName, postgresVersion, port, sql)

	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	if err != nil {
		logger.Error().
			Err(err).
			Str("output", output).
			Str("database_name", databaseName).
			Msg("Failed to execute post-restore SQL")
		return fmt.Errorf("post-restore SQL execution failed: %w", err)
	}

	logger.Info().
		Str("database_name", databaseName).
		Str("output", output).
		Msg("Post-restore SQL executed successfully")

	return nil
}
