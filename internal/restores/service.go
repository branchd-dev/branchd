package restores

import (
	"context"

	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/branchd-dev/branchd/internal/models"
	"github.com/branchd-dev/branchd/internal/restore"
)

// Service handles restore-related operations
// This is a thin wrapper around the restore orchestrator for use by the API server
type Service struct {
	orchestrator *restore.Orchestrator
	logger       zerolog.Logger
}

// NewService creates a new restores service
func NewService(db *gorm.DB, logger zerolog.Logger) *Service {
	return &Service{
		orchestrator: restore.NewOrchestrator(db, logger),
		logger:       logger.With().Str("component", "restores_service").Logger(),
	}
}

// Delete removes a restore and all its resources (ZFS dataset, systemd service, etc.)
func (s *Service) Delete(ctx context.Context, restore *models.Restore) error {
	return s.orchestrator.DeleteByModel(ctx, restore)
}

// GetOrchestrator returns the underlying orchestrator for advanced operations
func (s *Service) GetOrchestrator() *restore.Orchestrator {
	return s.orchestrator
}
