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

// deleteStaleRestores removes all restores that have no branches.
// A "stale" restore is one that has no branches attached to it.
// This is called after every successful restore to clean up unused restores.
// The excludeRestoreID parameter prevents deleting the just-completed restore.
func deleteStaleRestores(ctx context.Context, db *gorm.DB, excludeRestoreID string, logger zerolog.Logger) error {
	// Find all restores with branches preloaded
	var allRestores []models.Restore
	if err := db.Preload("Branches").Find(&allRestores).Error; err != nil {
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
		logger.Debug().Msg("No stale restores to clean up")
		return nil
	}

	logger.Info().
		Int("count", len(staleRestores)).
		Msg("Deleting stale restores (restores without branches)")

	// Delete each stale restore
	for _, restore := range staleRestores {
		logger.Info().
			Str("restore_id", restore.ID).
			Str("restore_name", restore.Name).
			Msg("Deleting stale restore")

		if err := deleteRestore(ctx, db, &restore, logger); err != nil {
			logger.Error().
				Err(err).
				Str("restore_id", restore.ID).
				Msg("Failed to delete stale restore")
			// Continue with other restores even if one fails
			continue
		}
	}

	return nil
}
