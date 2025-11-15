package workers

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/branchd-dev/branchd/internal/models"
)

// RestoreProvider defines the interface that all restore methods must implement
// Each provider (logical, Crunchy Bridge, etc.) handles the restore process differently
type RestoreProvider interface {
	// ValidateConfig validates provider-specific configuration
	ValidateConfig(config *models.Config) error

	// StartRestore performs setup and starts the restore process
	// The restore runs asynchronously, and completion is monitored via the wait handler
	StartRestore(ctx context.Context, params RestoreParams) error

	// GetProviderType returns the provider type identifier for logging
	GetProviderType() string
}

// RestoreParams contains common parameters needed by all restore providers
type RestoreParams struct {
	Restore         *models.Restore
	Config          *models.Config
	Port            int    // Allocated PostgreSQL port for this restore
	RestoreDataPath string // ZFS dataset path (e.g., /opt/branchd/restore_20250920143000)
	Logger          zerolog.Logger
}
