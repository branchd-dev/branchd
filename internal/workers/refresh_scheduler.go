package workers

import (
	"time"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/branchd-dev/branchd/internal/models"
	"github.com/branchd-dev/branchd/internal/tasks"
)

// StartRefreshScheduler runs a periodic check (every minute) for config refresh
func StartRefreshScheduler(client *asynq.Client, db *gorm.DB, logger zerolog.Logger) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Run immediately on startup, then every minute
	checkAndEnqueueRefreshTasks(client, db, logger)

	for range ticker.C {
		checkAndEnqueueRefreshTasks(client, db, logger)
	}
}

func checkAndEnqueueRefreshTasks(client *asynq.Client, db *gorm.DB, logger zerolog.Logger) {
	// Load the singleton config
	var config models.Config
	err := db.First(&config).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			logger.Debug().Msg("No config found - skipping refresh check")
			return
		}
		logger.Error().Err(err).Msg("Failed to query config for refresh")
		return
	}

	// Check if refresh schedule is configured
	if config.RefreshSchedule == "" {
		logger.Debug().Msg("No refresh schedule configured")
		return
	}

	if config.NextRefreshAt != nil && config.NextRefreshAt.After(time.Now()) {
		logger.Debug().
			Time("next_refresh_at", *config.NextRefreshAt).
			Msg("Refresh not due yet")
		return
	}

	logger.Info().
		Str("config_id", config.ID).
		Str("refresh_schedule", config.RefreshSchedule).
		Time("next_refresh_at", func() time.Time {
			if config.NextRefreshAt != nil {
				return *config.NextRefreshAt
			}
			return time.Time{}
		}()).
		Msg("Config refresh due - creating new database and enqueueing restore task")

	// Create a new database record for the refresh
	database := models.Restore{
		Name:       models.GenerateRestoreName(),
		SchemaOnly: config.SchemaOnly,
		Port:       5432, // Main PostgreSQL cluster port
	}

	if err := db.Create(&database).Error; err != nil {
		logger.Error().
			Err(err).
			Str("config_id", config.ID).
			Msg("Failed to create database record for refresh")
		return
	}

	logger.Info().
		Str("config_id", config.ID).
		Str("database_id", database.ID).
		Str("database_name", database.Name).
		Msg("Created new database record for refresh")

	// Enqueue pg_dump/restore task - it will restore the database
	task, err := tasks.NewTriggerLogicalRestoreTask(database.ID)
	if err != nil {
		logger.Error().
			Err(err).
			Str("config_id", config.ID).
			Str("database_id", database.ID).
			Msg("Failed to create pg_dump restore task")
		return
	}

	if _, err := client.Enqueue(task, asynq.Timeout(12*time.Hour)); err != nil {
		logger.Error().
			Err(err).
			Str("config_id", config.ID).
			Str("database_id", database.ID).
			Msg("Failed to enqueue pg_dump restore task")
		return
	}

	logger.Info().
		Str("config_id", config.ID).
		Str("database_id", database.ID).
		Bool("schema_only", config.SchemaOnly).
		Msg("Refresh restore task enqueued successfully")
}
