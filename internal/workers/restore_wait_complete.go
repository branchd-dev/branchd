package workers

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/branchd-dev/branchd/internal/models"
	"github.com/branchd-dev/branchd/internal/restore"
	"github.com/branchd-dev/branchd/internal/tasks"
)

// HandleRestoreWaitComplete polls for restore completion
// This handler is a thin adapter that uses the restore orchestrator
func HandleRestoreWaitComplete(ctx context.Context, t *asynq.Task, client *asynq.Client, db *gorm.DB, logger zerolog.Logger) error {
	payload, err := tasks.ParseTaskPayload(t)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	// Load restore record for logging
	var restoreModel models.Restore
	if err := db.Where("id = ?", payload.RestoreID).First(&restoreModel).Error; err != nil {
		return fmt.Errorf("failed to load restore: %w", err)
	}

	// Create orchestrator
	orchestrator := restore.NewOrchestrator(db, logger)

	// Check progress
	status, isRunning, logTail, err := orchestrator.CheckProgress(ctx, payload.RestoreID)
	if err != nil {
		return fmt.Errorf("failed to check restore progress: %w", err)
	}

	if isRunning {
		// Enqueue another wait task
		waitTask, err := tasks.NewTriggerRestoreWaitCompleteTask(restoreModel.ID)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to create wait complete task")
			return fmt.Errorf("failed to create wait complete task: %w", err)
		}

		_, err = client.Enqueue(waitTask,
			asynq.ProcessIn(10*time.Second),
			asynq.MaxRetry(4320),
		)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to enqueue next wait complete task")
			return fmt.Errorf("failed to enqueue next wait complete task: %w", err)
		}

		return nil
	}

	// Process is not running, handle based on status
	switch status {
	case restore.StatusSuccess:
		logger.Info().
			Str("restore_id", restoreModel.ID).
			Msg("Restore completed successfully")

		if err := orchestrator.Complete(ctx, payload.RestoreID); err != nil {
			return fmt.Errorf("failed to complete restore: %w", err)
		}

		return nil

	case restore.StatusFailed:
		logger.Error().
			Str("restore_id", restoreModel.ID).
			Str("log_tail", logTail).
			Msg("Restore failed")
		return fmt.Errorf("restore failed - log tail: %s", logTail)

	default:
		logger.Error().
			Str("restore_id", restoreModel.ID).
			Str("status", string(status)).
			Msg("Restore process died without clear result")
		return fmt.Errorf("restore process died - status: %s, log: %s", status, logTail)
	}
}
