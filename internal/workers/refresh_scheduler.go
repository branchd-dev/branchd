package workers

import (
	"time"

	"github.com/hibiken/asynq"
	"github.com/robfig/cron/v3"
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
		Msg("Config refresh due - checking if new restore can be created")

	// Check if we're already at or above max_restores limit
	var totalRestores int64
	if err := db.Model(&models.Restore{}).Count(&totalRestores).Error; err != nil {
		logger.Error().Err(err).Msg("Failed to count restores")
		return
	}

	// If we're at or above max_restores, skip creating new restore
	// Cleanup will happen after successful restores complete
	if int(totalRestores) >= config.MaxRestores {
		// Count restores with branches for logging
		var restoresWithBranches int64
		if err := db.Model(&models.Restore{}).
			Joins("JOIN branches ON branches.restore_id = restores.id").
			Distinct("restores.id").
			Count(&restoresWithBranches).Error; err != nil {
			logger.Error().Err(err).Msg("Failed to count restores with branches")
			return
		}

		logger.Warn().
			Int64("total_restores", totalRestores).
			Int64("restores_with_branches", restoresWithBranches).
			Int("max_restores", config.MaxRestores).
			Msg("Cannot create new restore - at max_restores limit")

		// Still update NextRefreshAt to prevent retrying every minute
		now := time.Now()
		nextRefresh := calculateNextRefreshTime(config.RefreshSchedule, now)
		if nextRefresh != nil {
			db.Model(&config).Update("next_refresh_at", nextRefresh)
		}
		return
	}

	// Determine schema-only flag
	// Note: Crunchy Bridge (pgBackRest) doesn't support schema-only, only logical restore (pg_dump) does
	schemaOnly := config.SchemaOnly
	if config.CrunchyBridgeAPIKey != "" {
		schemaOnly = false
	}

	// Create a new database record for the refresh
	database := models.Restore{
		Name:       models.GenerateRestoreName(),
		SchemaOnly: schemaOnly,
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

	// Enqueue restore task
	task, err := tasks.NewTriggerRestoreTask(database.ID)
	if err != nil {
		logger.Error().
			Err(err).
			Str("config_id", config.ID).
			Str("database_id", database.ID).
			Msg("Failed to create restore task")
		return
	}

	if _, err := client.Enqueue(task, asynq.Timeout(12*time.Hour)); err != nil {
		logger.Error().
			Err(err).
			Str("config_id", config.ID).
			Str("database_id", database.ID).
			Msg("Failed to enqueue restore task")
		return
	}

	// Calculate and update NextRefreshAt immediately after scheduling
	// This prevents the scheduler from creating new restores every minute
	now := time.Now()
	nextRefresh := calculateNextRefreshTime(config.RefreshSchedule, now)
	if nextRefresh != nil {
		if err := db.Model(&config).Update("next_refresh_at", nextRefresh).Error; err != nil {
			logger.Error().
				Err(err).
				Str("config_id", config.ID).
				Msg("Failed to update next_refresh_at")
		} else {
			logger.Info().
				Str("config_id", config.ID).
				Time("next_refresh_at", *nextRefresh).
				Msg("Updated next_refresh_at")
		}
	}

	logger.Info().
		Str("config_id", config.ID).
		Str("database_id", database.ID).
		Bool("schema_only", config.SchemaOnly).
		Msg("Refresh restore task enqueued successfully")
}

// calculateNextRefreshTime calculates next refresh time from cron schedule
func calculateNextRefreshTime(cronExpr string, from time.Time) *time.Time {
	if cronExpr == "" {
		return nil
	}

	// Parse cron expression (standard 5-field format: minute hour day-of-month month day-of-week)
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(cronExpr)
	if err != nil {
		return nil
	}

	next := schedule.Next(from)
	return &next
}
