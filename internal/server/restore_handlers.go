package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

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

	// Validate that connection string is set
	if config.ConnectionString == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Connection string not configured"})
		return
	}

	s.logger.Info().Str("config_id", config.ID).Msg("Manually triggering restore")

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
	restoreTask, err := tasks.NewPgDumpRestoreExecuteTask(restore.ID)
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
