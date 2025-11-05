package workers

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/branchd-dev/branchd/internal/models"
)

// deleteRestore stops the restore's PostgreSQL cluster, removes its ZFS dataset, and deletes the record
func deleteRestore(ctx context.Context, db *gorm.DB, restore *models.Restore, logger zerolog.Logger) error {
	logger.Info().
		Str("restore_id", restore.ID).
		Str("restore_name", restore.Name).
		Int("port", restore.Port).
		Msg("Deleting restore cluster and dataset")

	serviceName := fmt.Sprintf("branchd-restore-%s", restore.Name)
	zfsDataset := fmt.Sprintf("tank/%s", restore.Name)

	// 1. Stop and disable systemd service
	logger.Info().Str("service", serviceName).Msg("Stopping PostgreSQL service")
	stopCmd := fmt.Sprintf("sudo systemctl stop %s || true", serviceName)
	cmd := exec.CommandContext(ctx, "bash", "-c", stopCmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		logger.Warn().
			Err(err).
			Str("output", string(output)).
			Msg("Failed to stop service (continuing anyway)")
	}

	disableCmd := fmt.Sprintf("sudo systemctl disable %s || true", serviceName)
	cmd = exec.CommandContext(ctx, "bash", "-c", disableCmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		logger.Warn().
			Err(err).
			Str("output", string(output)).
			Msg("Failed to disable service (continuing anyway)")
	}

	// 2. Remove systemd service file
	logger.Info().Msg("Removing systemd service file")
	serviceFile := fmt.Sprintf("/etc/systemd/system/%s.service", serviceName)
	removeServiceCmd := fmt.Sprintf("sudo rm -f %s && sudo systemctl daemon-reload", serviceFile)
	cmd = exec.CommandContext(ctx, "bash", "-c", removeServiceCmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		logger.Warn().
			Err(err).
			Str("output", string(output)).
			Msg("Failed to remove service file (continuing anyway)")
	}

	// 3. Destroy ZFS dataset (includes all data)
	logger.Info().Str("zfs_dataset", zfsDataset).Msg("Destroying ZFS dataset")
	destroyCmd := fmt.Sprintf("sudo zfs destroy -r %s", zfsDataset)
	cmd = exec.CommandContext(ctx, "bash", "-c", destroyCmd)
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	if err != nil {
		logger.Error().
			Err(err).
			Str("zfs_dataset", zfsDataset).
			Str("output", output).
			Msg("Failed to destroy ZFS dataset")
		return fmt.Errorf("failed to destroy ZFS dataset: %w", err)
	}

	logger.Info().
		Str("zfs_dataset", zfsDataset).
		Msg("ZFS dataset destroyed successfully")

	// 4. Delete restore record from SQLite
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
