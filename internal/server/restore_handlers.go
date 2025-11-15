package server

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/branchd-dev/branchd/internal/anonymize"
	"github.com/branchd-dev/branchd/internal/models"
	"github.com/branchd-dev/branchd/internal/tasks"
)

// @Summary List restores
// @Description List all database restores
// @Tags restores
// @Produce json
// @Security BearerAuth
// @Success 200 {array} models.Restore
// @Failure 401 {object} map[string]interface{}
// @Router /api/restores [get]
func (s *Server) listRestores(c *gin.Context) {
	var restores []models.Restore
	if err := s.db.Preload("Branches").Order("created_at ASC").Find(&restores).Error; err != nil {
		s.logger.Error().Err(err).Msg("Failed to list restores")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list restores"})
		return
	}

	c.JSON(http.StatusOK, restores)
}

// @Summary Get restore
// @Description Get a specific restore by ID
// @Tags restores
// @Produce json
// @Security BearerAuth
// @Param id path string true "Restore ID"
// @Success 200 {object} models.Restore
// @Failure 404 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /api/restores/{id} [get]
func (s *Server) getRestore(c *gin.Context) {
	restoreID := c.Param("id")

	var restore models.Restore
	if err := s.db.Preload("Branches").Where("id = ?", restoreID).First(&restore).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Restore not found"})
			return
		}
		s.logger.Error().Err(err).Str("restore_id", restoreID).Msg("Failed to find restore")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	c.JSON(http.StatusOK, restore)
}

// @Summary Delete restore
// @Description Delete a restore (only allowed if no branches exist)
// @Tags restores
// @Produce json
// @Security BearerAuth
// @Param id path string true "Restore ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/restores/{id} [delete]
func (s *Server) deleteRestore(c *gin.Context) {
	restoreID := c.Param("id")

	// Load restore with branches
	var restore models.Restore
	if err := s.db.Preload("Branches").Where("id = ?", restoreID).First(&restore).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Restore not found"})
			return
		}
		s.logger.Error().Err(err).Str("restore_id", restoreID).Msg("Failed to find restore")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Check if restore has active branches
	if len(restore.Branches) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "Cannot delete restore with active branches",
			"branches": len(restore.Branches),
		})
		return
	}

	// Delete restore using branches service
	if err := s.branchesService.DeleteRestore(c.Request.Context(), &restore); err != nil {
		s.logger.Error().Err(err).Str("restore_id", restoreID).Msg("Failed to delete restore")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete restore"})
		return
	}

	s.logger.Info().Str("restore_id", restoreID).Str("restore_name", restore.Name).Msg("Restore deleted successfully")
	c.JSON(http.StatusOK, gin.H{"message": "Restore deleted successfully"})
}

// @Summary Trigger database restore
// @Description Manually trigger a database restore from the configured source
// @Tags restores
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/restores/trigger-restore [post]
func (s *Server) triggerRestore(c *gin.Context) {
	var config models.Config
	if err := s.db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Configuration not found"})
			return
		}
		s.logger.Error().Err(err).Msg("Failed to get config")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Validate that a restore source is configured (either connection string or Crunchy Bridge)
	hasConnectionString := config.ConnectionString != ""
	hasCrunchyBridge := config.CrunchyBridgeAPIKey != ""

	if !hasConnectionString && !hasCrunchyBridge {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No restore source configured (need either connection string or Crunchy Bridge credentials)"})
		return
	}

	s.logger.Info().
		Str("config_id", config.ID).
		Bool("has_connection_string", hasConnectionString).
		Bool("has_crunchy_bridge", hasCrunchyBridge).
		Msg("Manually triggering restore")

	// Create a new restore record with UTC datetime-based name (e.g., restore_20251017143202)
	restore := models.Restore{
		Name:       models.GenerateRestoreName(),
		SchemaOnly: config.SchemaOnly,
		Port:       5432,
	}

	if err := s.db.Create(&restore).Error; err != nil {
		s.logger.Error().Err(err).Msg("Failed to create restore record")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create restore"})
		return
	}

	// Enqueue restore task
	restoreTask, err := tasks.NewTriggerRestoreTask(restore.ID)
	if err != nil {
		s.logger.Error().Err(err).Str("restore_id", restore.ID).Msg("Failed to create restore task")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start restore"})
		return
	}

	taskInfo, err := s.asynqClient.Enqueue(restoreTask, asynq.Timeout(12*time.Hour))
	if err != nil {
		s.logger.Error().Err(err).Str("restore_id", restore.ID).Msg("Failed to enqueue restore task")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start restore"})
		return
	}

	s.logger.Info().
		Str("config_id", config.ID).
		Str("restore_id", restore.ID).
		Str("task_id", taskInfo.ID).
		Msg("Restore task enqueued successfully")

	c.JSON(http.StatusOK, gin.H{
		"message":    "Restore triggered successfully",
		"restore_id": restore.ID,
		"task_id":    taskInfo.ID,
	})
}

// @Summary Get restore logs
// @Description Get logs for a specific restore
// @Tags restores
// @Produce json
// @Security BearerAuth
// @Param id path string true "Restore ID"
// @Param lines query int false "Number of lines to fetch (default: 50)"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/restores/{id}/logs [get]
func (s *Server) getRestoreLogs(c *gin.Context) {
	restoreID := c.Param("id")

	// Get lines parameter (default to 50)
	lines := 50
	if linesStr := c.Query("lines"); linesStr != "" {
		if l, err := strconv.Atoi(linesStr); err == nil && l > 0 && l <= 1000 {
			lines = l
		}
	}

	// Find restore
	var restore models.Restore
	if err := s.db.Where("id = ?", restoreID).First(&restore).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Restore not found"})
			return
		}
		s.logger.Error().Err(err).Str("restore_id", restoreID).Msg("Failed to find restore")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Construct log file path
	logPath := fmt.Sprintf("/var/log/branchd/restore-%s.log", restore.Name)

	// Check if log file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		c.JSON(http.StatusOK, gin.H{
			"logs":        []string{},
			"total_lines": 0,
			"exists":      false,
		})
		return
	}

	// Read log file
	file, err := os.Open(logPath)
	if err != nil {
		s.logger.Error().Err(err).Str("log_path", logPath).Msg("Failed to open log file")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read log file"})
		return
	}
	defer file.Close()

	// Read all lines into a slice
	var allLines []string
	scanner := bufio.NewScanner(file)
	// Increase buffer size for long log lines
	const maxCapacity = 1024 * 1024 // 1MB
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		s.logger.Error().Err(err).Str("log_path", logPath).Msg("Failed to read log file")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read log file"})
		return
	}

	totalLines := len(allLines)

	// Get last N lines
	var logLines []string
	if totalLines <= lines {
		logLines = allLines
	} else {
		logLines = allLines[totalLines-lines:]
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":        logLines,
		"total_lines": totalLines,
		"exists":      true,
	})
}

// postgresVersionToPort maps PostgreSQL major version to its port
func postgresVersionToPort(version string) int {
	switch version {
	case "14":
		return 5414
	case "15":
		return 5415
	case "16":
		return 5416
	case "17":
		return 5417
	default:
		// Default to 5432 for unknown versions
		return 5432
	}
}

// @Summary Apply anonymization rules to restore
// @Description Manually trigger anonymization rules on a specific restore
// @Tags restores
// @Produce json
// @Security BearerAuth
// @Param id path string true "Restore ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/restores/{id}/anonymize [post]
func (s *Server) applyAnonymization(c *gin.Context) {
	restoreID := c.Param("id")

	// Find restore
	var restore models.Restore
	if err := s.db.Where("id = ?", restoreID).First(&restore).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Restore not found"})
			return
		}
		s.logger.Error().Err(err).Str("restore_id", restoreID).Msg("Failed to find restore")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Load config to get PG version
	var config models.Config
	if err := s.db.First(&config).Error; err != nil {
		s.logger.Error().Err(err).Msg("Failed to load config")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	s.logger.Info().
		Str("restore_id", restoreID).
		Str("restore_name", restore.Name).
		Msg("Manually triggering anonymization")

	// Apply anonymization rules
	pgPort := postgresVersionToPort(config.PostgresVersion)
	rulesApplied, err := anonymize.Apply(c.Request.Context(), s.db, anonymize.ApplyParams{
		DatabaseName:    restore.Name,
		PostgresVersion: config.PostgresVersion,
		PostgresPort:    pgPort,
	}, s.logger)
	if err != nil {
		s.logger.Error().Err(err).Str("restore_id", restoreID).Msg("Failed to apply anonymization")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to apply anonymization: %v", err)})
		return
	}

	s.logger.Info().
		Str("restore_id", restoreID).
		Int("rules_applied", rulesApplied).
		Msg("Anonymization completed successfully")

	c.JSON(http.StatusOK, gin.H{
		"message":       "Anonymization completed successfully",
		"rules_applied": rulesApplied,
	})
}
