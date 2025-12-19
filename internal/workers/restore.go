package workers

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/branchd-dev/branchd/internal/config"
	"github.com/branchd-dev/branchd/internal/restore"
	"github.com/branchd-dev/branchd/internal/tasks"
)

// HandleTriggerRestore starts the restore process for a database
// This is a thin adapter that delegates to the restore orchestrator
func HandleTriggerRestore(ctx context.Context, t *asynq.Task, client *asynq.Client, db *gorm.DB, cfg *config.Config, logger zerolog.Logger) error {
	payload, err := tasks.ParseTaskPayload(t)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	// Create orchestrator
	orchestrator := restore.NewOrchestrator(db, logger)

	// Start the restore
	if err := orchestrator.Start(ctx, payload.RestoreID); err != nil {
		return fmt.Errorf("failed to start restore: %w", err)
	}

	// Check if restore is already running (orchestrator.Start returns nil if already running)
	isRunning, _, err := orchestrator.IsRunning(ctx, payload.RestoreID)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to check if restore is running")
	}

	// Enqueue wait task to monitor restore completion
	waitTask, err := tasks.NewTriggerRestoreWaitCompleteTask(payload.RestoreID)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create wait complete task")
		return fmt.Errorf("failed to create wait complete task: %w", err)
	}

	// Schedule first check - if already running, we still want to monitor it
	delay := 10 * time.Second
	if isRunning {
		logger.Info().
			Str("restore_id", payload.RestoreID).
			Msg("Restore is already running, scheduling monitoring")
	}

	_, err = client.Enqueue(waitTask,
		asynq.ProcessIn(delay),
		asynq.MaxRetry(4320), // 12 hours at 10s intervals
	)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to enqueue wait complete task")
		return fmt.Errorf("failed to enqueue wait complete task: %w", err)
	}

	logger.Info().
		Str("restore_id", payload.RestoreID).
		Msg("Restore triggered successfully")

	return nil
}
