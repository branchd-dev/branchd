package workers

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/branchd-dev/branchd/internal/models"
)

// deleteRestore deletes a restore database from PostgreSQL cluster and removes its record
func deleteRestore(ctx context.Context, db *gorm.DB, restore *models.Restore, logger zerolog.Logger) error {
	logger.Info().
		Str("restore_id", restore.ID).
		Str("restore_name", restore.Name).
		Msg("Deleting restore")

	// Load config to get PostgreSQL version
	var config models.Config
	if err := db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("configuration not found")
		}
		logger.Error().Err(err).Msg("Failed to load config")
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Map PostgreSQL version to port
	port := postgresVersionToPort(config.PostgresVersion)

	// Drop restore database from PostgreSQL cluster
	dropCmd := fmt.Sprintf("sudo -u postgres psql -p %d -c 'DROP DATABASE IF EXISTS \"%s\"'", port, restore.Name)
	cmd := exec.CommandContext(ctx, "bash", "-c", dropCmd)
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	if err != nil {
		logger.Error().
			Err(err).
			Str("restore_name", restore.Name).
			Str("output", output).
			Msg("Failed to drop restore from PostgreSQL")
		return fmt.Errorf("failed to drop restore from PostgreSQL: %w", err)
	}

	logger.Info().
		Str("restore_name", restore.Name).
		Int("port", port).
		Msg("Restore dropped from PostgreSQL cluster")

	// Delete restore record from SQLite
	if err := db.Delete(restore).Error; err != nil {
		logger.Error().
			Err(err).
			Str("restore_id", restore.ID).
			Msg("Failed to delete restore record")
		return fmt.Errorf("failed to delete restore record: %w", err)
	}

	logger.Info().
		Str("restore_id", restore.ID).
		Str("restore_name", restore.Name).
		Msg("Restore deleted successfully")

	return nil
}

// cleanupStaleRestores removes old restores to maintain the MaxRestores limit
// Excludes the specified restore ID (typically the one that just completed)
// Deletes oldest restores without branches first, never deletes restores with branches
func cleanupStaleRestores(ctx context.Context, db *gorm.DB, excludeRestoreID string, logger zerolog.Logger) error {
	// Load config to get MaxRestores
	var config models.Config
	if err := db.First(&config).Error; err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Find all restores with branches preloaded, ordered by creation (oldest first)
	var allRestores []models.Restore
	if err := db.Preload("Branches").Order("created_at ASC").Find(&allRestores).Error; err != nil {
		return fmt.Errorf("failed to load restores: %w", err)
	}

	// Count total restores
	totalRestores := len(allRestores)
	if totalRestores <= config.MaxRestores {
		logger.Debug().
			Int("total_restores", totalRestores).
			Int("max_restores", config.MaxRestores).
			Msg("Total restores within limit, no cleanup needed")
		return nil
	}

	// We need to delete (totalRestores - MaxRestores) restores
	toDeleteCount := totalRestores - config.MaxRestores

	// Filter restores that can be deleted (no branches, not excluded)
	// Start with oldest first to preserve newer restores
	var toDelete []models.Restore
	for _, restore := range allRestores {
		if len(toDelete) >= toDeleteCount {
			break // We've found enough to delete
		}

		hasNoBranches := len(restore.Branches) == 0
		shouldExclude := restore.ID == excludeRestoreID

		if hasNoBranches && !shouldExclude {
			toDelete = append(toDelete, restore)
		}
	}

	if len(toDelete) == 0 {
		logger.Debug().
			Int("total_restores", totalRestores).
			Int("max_restores", config.MaxRestores).
			Msg("No eligible restores to clean up (all have branches or are excluded)")
		return nil
	}

	logger.Info().
		Int("count", len(toDelete)).
		Int("total_restores", totalRestores).
		Int("max_restores", config.MaxRestores).
		Msg("Cleaning up old restores to meet MaxRestores limit")

	// Delete each old restore
	for _, restore := range toDelete {
		logger.Info().
			Str("restore_id", restore.ID).
			Str("restore_name", restore.Name).
			Msg("Deleting old restore")

		if err := deleteRestore(ctx, db, &restore, logger); err != nil {
			logger.Error().
				Err(err).
				Str("restore_id", restore.ID).
				Msg("Failed to delete restore")
			// Continue with other restores even if one fails
			continue
		}
	}

	return nil
}
