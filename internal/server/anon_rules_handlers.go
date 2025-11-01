package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/branchd-dev/branchd/internal/models"
)

type CreateAnonRuleRequest struct {
	Table    string `json:"table" binding:"required"`
	Column   string `json:"column" binding:"required"`
	Template string `json:"template" binding:"required"`
}

// @Router /api/anon-rules [get]
// @Success 200 {object} []models.AnonRule
func (s *Server) listAnonRules(c *gin.Context) {
	// Load all anon rules (global, not per-instance)
	var rules []models.AnonRule
	if err := s.db.Order("created_at DESC").Find(&rules).Error; err != nil {
		s.logger.Error().Err(err).Msg("Failed to load anon rules")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	c.JSON(http.StatusOK, rules)
}

// @Router /api/anon-rules [post]
// @Param request body CreateAnonRuleRequest true "Create anon rule request"
// @Success 201 {object} models.AnonRule
func (s *Server) createAnonRule(c *gin.Context) {
	var req CreateAnonRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.logger.Warn().Err(err).Msg("Invalid request body")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	// Create anon rule (global, applies to all database restores)
	rule := models.AnonRule{
		Table:    req.Table,
		Column:   req.Column,
		Template: req.Template,
	}

	if err := s.db.Create(&rule).Error; err != nil {
		s.logger.Error().Err(err).Msg("Failed to create anon rule")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create anonymization rule"})
		return
	}

	s.logger.Info().
		Str("rule_id", rule.ID).
		Str("table", rule.Table).
		Str("column", rule.Column).
		Msg("Created anonymization rule")

	c.JSON(http.StatusCreated, rule)
}

// @Router /api/anon-rules/{id} [delete]
// @Param id path string true "Rule ID"
// @Success 204
func (s *Server) deleteAnonRule(c *gin.Context) {
	ruleID := c.Param("id")

	// Find rule
	var rule models.AnonRule
	if err := s.db.Where("id = ?", ruleID).First(&rule).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
			return
		}
		s.logger.Error().Err(err).Str("rule_id", ruleID).Msg("Failed to find anon rule")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Delete rule
	if err := s.db.Delete(&rule).Error; err != nil {
		s.logger.Error().Err(err).Str("rule_id", ruleID).Msg("Failed to delete anon rule")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete anonymization rule"})
		return
	}

	s.logger.Info().
		Str("rule_id", ruleID).
		Msg("Deleted anonymization rule")

	c.Status(http.StatusNoContent)
}
