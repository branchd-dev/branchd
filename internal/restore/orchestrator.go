package restore

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/branchd-dev/branchd/internal/anonymize"
	"github.com/branchd-dev/branchd/internal/models"
)

// Orchestrator coordinates all restore operations
// It manages the lifecycle of restores: start, monitor, complete, and cleanup
type Orchestrator struct {
	db             *gorm.DB
	processManager *ProcessManager
	resources      *ResourceManager
	logger         zerolog.Logger
}

// NewOrchestrator creates a new restore orchestrator
func NewOrchestrator(db *gorm.DB, logger zerolog.Logger) *Orchestrator {
	return &Orchestrator{
		db:             db,
		processManager: NewProcessManager(logger),
		resources:      NewResourceManager(logger),
		logger:         logger.With().Str("component", "restore_orchestrator").Logger(),
	}
}

// SelectProvider determines which restore provider to use based on config
func (o *Orchestrator) SelectProvider(config *models.Config) (Provider, ProviderType, error) {
	// Crunchy Bridge takes precedence if configured
	if config.CrunchyBridgeAPIKey != "" {
		return NewCrunchyBridgeProvider(o.logger), ProviderTypeCrunchyBridge, nil
	}

	// Fallback to logical restore
	if config.ConnectionString != "" {
		return NewLogicalProvider(o.logger), ProviderTypeLogical, nil
	}

	return nil, "", fmt.Errorf("no restore source configured (need either ConnectionString or CrunchyBridge credentials)")
}

// Start begins a restore operation
// It selects the appropriate provider, allocates resources, and starts the restore process
func (o *Orchestrator) Start(ctx context.Context, restoreID string) error {
	// Load restore record
	var restore models.Restore
	if err := o.db.Where("id = ?", restoreID).First(&restore).Error; err != nil {
		return fmt.Errorf("failed to load restore: %w", err)
	}

	// Load config
	var config models.Config
	if err := o.db.First(&config).Error; err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Select appropriate provider based on config
	provider, providerType, err := o.SelectProvider(&config)
	if err != nil {
		return fmt.Errorf("failed to select restore provider: %w", err)
	}

	o.logger.Info().
		Str("restore_id", restore.ID).
		Str("provider", string(providerType)).
		Bool("schema_only", restore.SchemaOnly).
		Msg("Starting restore process")

	// Validate provider-specific configuration
	if err := provider.ValidateConfig(&config); err != nil {
		return fmt.Errorf("provider validation failed: %w", err)
	}

	// Find available port for this restore's PostgreSQL cluster
	pgPort, err := o.resources.FindAvailablePort(ctx)
	if err != nil {
		return fmt.Errorf("failed to find available port: %w", err)
	}

	// Store port in database
	if err := o.db.Model(&restore).Update("port", pgPort).Error; err != nil {
		return fmt.Errorf("failed to store port in database: %w", err)
	}

	// Create log directory
	if err := o.processManager.CreateLogDirectory(ctx); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Check if restore is already in progress
	isRunning, pid, err := o.processManager.CheckIfRunning(ctx, restore.Name)
	if err != nil {
		return fmt.Errorf("failed to check restore status: %w", err)
	}

	if isRunning {
		o.logger.Info().
			Int("pid", pid).
			Str("restore_id", restore.ID).
			Msg("Restore is already running, skipping start")
		return nil
	}

	// Calculate restore dataset path
	restoreDataPath := GetRestoreDataPath(restore.Name)

	// Delegate to provider to start the restore
	params := ProviderParams{
		Restore:         &restore,
		Config:          &config,
		Port:            pgPort,
		RestoreDataPath: restoreDataPath,
		Logger:          o.logger,
		ProcessManager:  o.processManager,
	}

	if err := provider.StartRestore(ctx, params); err != nil {
		return fmt.Errorf("provider failed to start restore: %w", err)
	}

	o.logger.Info().
		Str("restore_id", restore.ID).
		Str("provider", string(providerType)).
		Msg("Restore triggered successfully")

	return nil
}

// CheckProgress checks the current status of a restore operation
// Returns the status and whether the process is still running
func (o *Orchestrator) CheckProgress(ctx context.Context, restoreID string) (Status, bool, string, error) {
	// Load restore record
	var restore models.Restore
	if err := o.db.Where("id = ?", restoreID).First(&restore).Error; err != nil {
		return StatusUnknown, false, "", fmt.Errorf("failed to load restore: %w", err)
	}

	// Check if restore is still running
	isRunning, _, err := o.processManager.CheckIfRunning(ctx, restore.Name)
	if err != nil {
		return StatusUnknown, false, "", fmt.Errorf("failed to check if restore process is running: %w", err)
	}

	if isRunning {
		return StatusRunning, true, "", nil
	}

	// Process is not running, check the result
	status, logTail, err := o.processManager.CheckStatus(ctx, restore.Name)
	if err != nil {
		return StatusUnknown, false, "", fmt.Errorf("failed to get restore result: %w", err)
	}

	return status, false, logTail, nil
}

// Complete finalizes a successful restore operation
// It runs post-restore SQL, applies anonymization, and marks the restore as ready
func (o *Orchestrator) Complete(ctx context.Context, restoreID string) error {
	// Load restore record
	var restore models.Restore
	if err := o.db.Where("id = ?", restoreID).First(&restore).Error; err != nil {
		return fmt.Errorf("failed to load restore: %w", err)
	}

	// Load config
	var config models.Config
	if err := o.db.First(&config).Error; err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Determine the target database name based on restore type
	targetDatabase := config.DatabaseName // Default for logical restores
	if config.CrunchyBridgeAPIKey != "" {
		// Crunchy Bridge restore - use the configured database name
		targetDatabase = config.CrunchyBridgeDatabaseName
	}

	// Execute post-restore SQL
	if config.PostRestoreSQL != "" {
		if err := o.executePostRestoreSQL(ctx, config.PostRestoreSQL, targetDatabase, config.PostgresVersion, restore.Port); err != nil {
			o.logger.Error().Err(err).Msg("Failed to execute post-restore SQL")
			return fmt.Errorf("failed to execute post-restore SQL: %w", err)
		}
	}

	// Apply anonymization
	_, err := anonymize.Apply(ctx, o.db, anonymize.ApplyParams{
		DatabaseName:    targetDatabase,
		PostgresVersion: config.PostgresVersion,
		PostgresPort:    restore.Port,
	}, o.logger)
	if err != nil {
		o.logger.Error().Err(err).Msg("Failed to apply anonymization rules")
		return fmt.Errorf("failed to apply anonymization rules: %w", err)
	}

	// Mark database as ready
	now := time.Now()
	updates := map[string]interface{}{
		"schema_ready": true,
		"ready_at":     now,
	}
	if !restore.SchemaOnly {
		updates["data_ready"] = true
	}

	if err := o.db.Model(&restore).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to mark database ready: %w", err)
	}

	// Update refresh timestamps only if a refresh schedule is configured
	// This ensures manual restores don't affect the scheduled refresh timing
	if config.RefreshSchedule != "" {
		o.logger.Info().
			Str("refresh_schedule", config.RefreshSchedule).
			Msg("Updating refresh timestamps")

		// Calculate next refresh time based on the schedule
		nextRefresh := o.calculateNextRefresh(config.RefreshSchedule, now)
		if err := o.db.Model(&config).Updates(map[string]interface{}{
			"last_refreshed_at": now,
			"next_refresh_at":   nextRefresh,
		}).Error; err != nil {
			o.logger.Error().Err(err).Msg("Failed to update refresh timestamps")
		}
	}

	// Delete stale restores (restores without branches) after successful restore
	if err := o.DeleteStaleRestores(ctx, restore.ID); err != nil {
		o.logger.Warn().Err(err).Msg("Failed to delete stale restores (non-fatal)")
	}

	o.logger.Info().
		Str("restore_id", restore.ID).
		Msg("Restore completed successfully")

	return nil
}

// Delete removes a restore and all its resources
func (o *Orchestrator) Delete(ctx context.Context, restoreID string) error {
	// Load restore record
	var restore models.Restore
	if err := o.db.Where("id = ?", restoreID).First(&restore).Error; err != nil {
		return fmt.Errorf("failed to load restore: %w", err)
	}

	o.logger.Info().
		Str("restore_id", restore.ID).
		Str("restore_name", restore.Name).
		Int("port", restore.Port).
		Msg("Deleting restore cluster and dataset")

	// Cleanup all resources
	if err := o.resources.CleanupRestore(ctx, restore.Name, o.processManager); err != nil {
		return fmt.Errorf("failed to cleanup restore resources: %w", err)
	}

	// Delete restore record from SQLite
	if err := o.db.Delete(&restore).Error; err != nil {
		o.logger.Error().
			Err(err).
			Str("restore_id", restore.ID).
			Msg("Failed to delete restore record")
		return fmt.Errorf("failed to delete restore record: %w", err)
	}

	o.logger.Info().
		Str("restore_id", restore.ID).
		Str("restore_name", restore.Name).
		Msg("Restore deleted successfully")

	return nil
}

// DeleteByModel deletes a restore using a model reference (for use when already loaded)
func (o *Orchestrator) DeleteByModel(ctx context.Context, restore *models.Restore) error {
	o.logger.Info().
		Str("restore_id", restore.ID).
		Str("restore_name", restore.Name).
		Int("port", restore.Port).
		Msg("Deleting restore cluster and dataset")

	// Cleanup all resources
	if err := o.resources.CleanupRestore(ctx, restore.Name, o.processManager); err != nil {
		return fmt.Errorf("failed to cleanup restore resources: %w", err)
	}

	// Delete restore record from SQLite
	if err := o.db.Delete(restore).Error; err != nil {
		o.logger.Error().
			Err(err).
			Str("restore_id", restore.ID).
			Msg("Failed to delete restore record")
		return fmt.Errorf("failed to delete restore record: %w", err)
	}

	o.logger.Info().
		Str("restore_id", restore.ID).
		Str("restore_name", restore.Name).
		Msg("Restore deleted successfully")

	return nil
}

// DeleteStaleRestores removes all restores that have no branches
// A "stale" restore is one that has no branches attached to it
// The excludeRestoreID parameter prevents deleting the just-completed restore
func (o *Orchestrator) DeleteStaleRestores(ctx context.Context, excludeRestoreID string) error {
	// Find all restores with branches preloaded
	var allRestores []models.Restore
	if err := o.db.Preload("Branches").Find(&allRestores).Error; err != nil {
		return fmt.Errorf("failed to load restores: %w", err)
	}

	// Find stale restores (no branches, not the just-completed one)
	var staleRestores []models.Restore
	for _, restore := range allRestores {
		hasBranches := len(restore.Branches) > 0
		isExcluded := restore.ID == excludeRestoreID

		if !hasBranches && !isExcluded {
			staleRestores = append(staleRestores, restore)
		}
	}

	if len(staleRestores) == 0 {
		o.logger.Debug().Msg("No stale restores to clean up")
		return nil
	}

	o.logger.Info().
		Int("count", len(staleRestores)).
		Msg("Deleting stale restores (restores without branches)")

	// Delete each stale restore
	for _, restore := range staleRestores {
		o.logger.Info().
			Str("restore_id", restore.ID).
			Str("restore_name", restore.Name).
			Msg("Deleting stale restore")

		if err := o.DeleteByModel(ctx, &restore); err != nil {
			o.logger.Error().
				Err(err).
				Str("restore_id", restore.ID).
				Msg("Failed to delete stale restore")
			// Continue with other restores even if one fails
			continue
		}
	}

	return nil
}

// IsRunning checks if a restore is currently in progress
func (o *Orchestrator) IsRunning(ctx context.Context, restoreID string) (bool, int, error) {
	// Load restore record
	var restore models.Restore
	if err := o.db.Where("id = ?", restoreID).First(&restore).Error; err != nil {
		return false, 0, fmt.Errorf("failed to load restore: %w", err)
	}

	return o.processManager.CheckIfRunning(ctx, restore.Name)
}

// GetProcessManager returns the process manager for external use (e.g., by workers)
func (o *Orchestrator) GetProcessManager() *ProcessManager {
	return o.processManager
}

// GetResourceManager returns the resource manager for external use
func (o *Orchestrator) GetResourceManager() *ResourceManager {
	return o.resources
}

// executePostRestoreSQL executes custom SQL statements after restore completes
func (o *Orchestrator) executePostRestoreSQL(ctx context.Context, sql, databaseName, postgresVersion string, port int) error {
	o.logger.Info().
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
		o.logger.Error().
			Err(err).
			Str("output", output).
			Str("database_name", databaseName).
			Msg("Failed to execute post-restore SQL")
		return fmt.Errorf("post-restore SQL execution failed: %w", err)
	}

	o.logger.Info().
		Str("database_name", databaseName).
		Str("output", output).
		Msg("Post-restore SQL executed successfully")

	return nil
}

// calculateNextRefresh calculates next refresh time from cron schedule
func (o *Orchestrator) calculateNextRefresh(cronExpr string, from time.Time) *time.Time {
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
