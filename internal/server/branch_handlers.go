package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/branchd-dev/branchd/internal/branches"
	"github.com/branchd-dev/branchd/internal/models"
)

type CreateBranchRequest struct {
	Name string `json:"name" binding:"required" validate:"required,min=1,max=50,alphanumdash"`
}

type CreateBranchResponse struct {
	ID       string `json:"id"`       // Branch ID (ULID)
	User     string `json:"user"`     // 16-chars random string
	Password string `json:"password"` // 32-chars random string
	Host     string `json:"host"`     // localhost or VM IP
	Port     int    `json:"port"`     // assigned port for this branch
	Database string `json:"database"` // parsed from Config.ConnectionString
}

// @Router /api/branches [post]
// @Param body body CreateBranchRequest true "Branch creation request"
// @Success 201 {object} CreateBranchResponse
func (s *Server) createBranch(c *gin.Context) {
	sessionData, exists := GetSessionData(c)
	if !exists {
		s.logger.Error().Msg("Session data not found in context")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Parse request body
	var req CreateBranchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.logger.Warn().Err(err).Msg("Invalid request body")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	// Validate request
	if err := s.validator.Struct(&req); err != nil {
		s.logger.Warn().Err(err).Msg("Request validation failed")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed", "details": err.Error()})
		return
	}

	// Normalize branch name to lowercase for consistency
	req.Name = strings.ToLower(req.Name)

	// Get config (singleton)
	var config models.Config
	if err := s.db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Configuration not found. Please complete onboarding first."})
			return
		}
		s.logger.Error().Err(err).Msg("Failed to find config")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Create branch using the service
	branchParams := branches.CreateBranchParams{
		BranchName:  req.Name,
		CreatedByID: sessionData.UserID,
	}

	branch, err := s.branchesService.CreateBranch(c.Request.Context(), branchParams)
	if err != nil {
		s.logger.Error().Err(err).Msg("Error creating branch")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Load the restore that this branch is associated with to get correct restore name
	var restore models.Restore
	if err := s.db.First(&restore, "id = ?", branch.RestoreID).Error; err != nil {
		s.logger.Error().Err(err).Msg("Failed to load restore for branch")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load restore information"})
		return
	}

	// Determine host for connection string
	// Priority: 1. Config.Domain, 2. Request Host, 3. localhost
	host := config.Domain
	if host == "" {
		host = c.Request.Host
		if host == "" {
			host = "localhost"
		}
		// Remove port from host if present (e.g., "example.com:8080" -> "example.com")
		if colonIdx := strings.Index(host, ":"); colonIdx != -1 {
			host = host[:colonIdx]
		}
	}

	// Return connection details with correct restore name from the Restore record
	response := CreateBranchResponse{
		ID:       branch.ID,
		User:     branch.User,
		Password: branch.Password,
		Host:     host,
		Port:     branch.Port,
		Database: restore.Name, // Use restore name from Restore record, not config
	}

	c.JSON(http.StatusCreated, response)
}

// @Router /api/branches/:id [delete]
// @Param id path string true "Branch ID"
// @Success 200 {object} map[string]interface{}
func (s *Server) deleteBranch(c *gin.Context) {
	branchID := c.Param("id")

	// Find branch
	var branch models.Branch
	if err := s.db.Where("id = ?", branchID).First(&branch).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Branch not found"})
			return
		}
		s.logger.Error().Err(err).Str("branch_id", branchID).Msg("Failed to find branch")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Delete branch using service
	deleteParams := branches.DeleteBranchParams{
		BranchName: branch.Name,
	}
	if err := s.branchesService.DeleteBranch(c.Request.Context(), deleteParams); err != nil {
		s.logger.Error().Err(err).Msg("Error deleting branch")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Branch deleted successfully",
	})
}

// BranchListResponse represents a branch in the list view
type BranchListResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	CreatedAt     string `json:"created_at"`
	CreatedBy     string `json:"created_by"`
	RestoreID     string `json:"restore_id"`
	RestoreName   string `json:"restore_name"`
	Port          int    `json:"port"`
	ConnectionURL string `json:"connection_url"`
}

// @Router /api/branches [get]
// @Success 200 {array} BranchListResponse
func (s *Server) listBranches(c *gin.Context) {
	// Get all branches with preloaded relationships
	var branches []models.Branch
	if err := s.db.Preload("Restore").
		Preload("CreatedBy").
		Order("created_at ASC").
		Find(&branches).Error; err != nil {
		s.logger.Error().Err(err).Msg("Failed to load branches")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load branches"})
		return
	}

	// Get config to determine host for connection strings
	var config models.Config
	if err := s.db.First(&config).Error; err != nil {
		s.logger.Error().Err(err).Msg("Failed to load config")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load configuration"})
		return
	}

	// Determine host for connection strings
	// Priority: 1. Config.Domain, 2. Request Host, 3. localhost
	host := config.Domain
	if host == "" {
		host = c.Request.Host
		if host == "" {
			host = "localhost"
		}
		// Remove port from host if present (e.g., "example.com:8080" -> "example.com")
		if colonIdx := strings.Index(host, ":"); colonIdx != -1 {
			host = host[:colonIdx]
		}
	}

	response := make([]BranchListResponse, 0, len(branches))
	for _, branch := range branches {
		// Determine created by
		createdBy := "Unknown"
		if branch.CreatedBy != nil {
			createdBy = branch.CreatedBy.Email
		}

		// Build connection URL using the restore name and determined host
		connectionURL := fmt.Sprintf("postgresql://%s:%s@%s:%d/%s",
			branch.User,
			branch.Password,
			host,
			branch.Port,
			branch.Restore.Name,
		)

		response = append(response, BranchListResponse{
			ID:            branch.ID,
			Name:          branch.Name,
			CreatedAt:     branch.CreatedAt.Format("2006-01-02 15:04:05"),
			CreatedBy:     createdBy,
			RestoreID:     branch.RestoreID,
			RestoreName:   branch.Restore.Name,
			Port:          branch.Port,
			ConnectionURL: connectionURL,
		})
	}

	c.JSON(http.StatusOK, response)
}
