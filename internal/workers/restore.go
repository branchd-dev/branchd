package workers

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/branchd-dev/branchd/internal/config"
	"github.com/branchd-dev/branchd/internal/models"
	"github.com/branchd-dev/branchd/internal/restore"
	"github.com/branchd-dev/branchd/internal/tasks"
)

// HandleTriggerRestore starts the restore process for a database
// This is the generic entry point that works for all restore providers
func HandleTriggerRestore(ctx context.Context, t *asynq.Task, client *asynq.Client, db *gorm.DB, cfg *config.Config, logger zerolog.Logger) error {
	payload, err := tasks.ParseTaskPayload(t)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	// Load restore record
	var dbRestore models.Restore
	if err := db.Where("id = ?", payload.RestoreID).First(&dbRestore).Error; err != nil {
		return fmt.Errorf("failed to load restore: %w", err)
	}

	// Load config
	var configModel models.Config
	if err := db.First(&configModel).Error; err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Select appropriate provider based on config
	provider, providerType, err := selectProvider(&configModel, logger)
	if err != nil {
		return fmt.Errorf("failed to select restore provider: %w", err)
	}

	logger.Info().
		Str("restore_id", dbRestore.ID).
		Str("provider", providerType).
		Bool("schema_only", dbRestore.SchemaOnly).
		Msg("Starting restore process")

	// Validate provider-specific configuration
	if err := provider.ValidateConfig(&configModel); err != nil {
		return fmt.Errorf("provider validation failed: %w", err)
	}

	// Create orchestrator for common operations
	orchestrator := restore.NewOrchestrator(logger)

	// Find available port for this restore's PostgreSQL cluster
	pgPort, err := findAvailablePort(ctx, logger)
	if err != nil {
		return fmt.Errorf("failed to find available port: %w", err)
	}

	// Store port in database
	if err := db.Model(&dbRestore).Update("port", pgPort).Error; err != nil {
		return fmt.Errorf("failed to store port in database: %w", err)
	}

	// Create log directory
	if err := orchestrator.CreateLogDirectory(ctx); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Check if restore is already in progress
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
		waitTask, err := tasks.NewTriggerRestoreWaitCompleteTask(dbRestore.ID)
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

	// Calculate restore dataset path
	// Each restore gets its own ZFS dataset: /opt/branchd/restore_YYYYMMDDHHMMSS
	restoreDatasetPath := fmt.Sprintf("/opt/branchd/%s", dbRestore.Name)

	// Delegate to provider to start the restore
	params := RestoreParams{
		Restore:         &dbRestore,
		Config:          &configModel,
		Port:            pgPort,
		RestoreDataPath: restoreDatasetPath,
		Logger:          logger,
	}

	if err := provider.StartRestore(ctx, params); err != nil {
		return fmt.Errorf("provider failed to start restore: %w", err)
	}

	// Enqueue wait task to monitor restore completion
	waitTask, err := tasks.NewTriggerRestoreWaitCompleteTask(dbRestore.ID)
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
		Str("provider", providerType).
		Msg("Restore triggered successfully")

	return nil
}

// selectProvider determines which restore provider to use based on config
func selectProvider(config *models.Config, logger zerolog.Logger) (RestoreProvider, string, error) {
	// Crunchy Bridge takes precedence if configured
	if config.CrunchyBridgeAPIKey != "" {
		return NewCrunchyBridgeProvider(logger), "crunchy_bridge", nil
	}

	// Fallback to logical restore
	if config.ConnectionString != "" {
		return NewLogicalRestoreProvider(logger), "logical", nil
	}

	return nil, "", fmt.Errorf("no restore source configured (need either ConnectionString or CrunchyBridge credentials)")
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
