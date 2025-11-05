package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/branchd-dev/branchd/internal/models"
)

type CreateAnonRuleRequest struct {
	Table    string          `json:"table" binding:"required"`
	Column   string          `json:"column" binding:"required"`
	Template json.RawMessage `json:"template" binding:"required"`
	Type     string          `json:"type"` // Optional: "text", "integer", "boolean", "null" - overrides auto-detection
}

// Parse parses the template and detects its type
func (r *CreateAnonRuleRequest) Parse() (template string, columnType string, err error) {
	// If type is explicitly specified, use it
	if r.Type != "" {
		// Validate the type
		validTypes := map[string]bool{"text": true, "integer": true, "boolean": true, "null": true}
		if !validTypes[r.Type] {
			return "", "", fmt.Errorf("invalid type '%s', must be one of: text, integer, boolean, null", r.Type)
		}

		columnType = r.Type

		// For null type, template is ignored
		if r.Type == "null" {
			return "", "null", nil
		}

		// Extract template value as string
		var strVal string
		if err := json.Unmarshal(r.Template, &strVal); err == nil {
			return strVal, columnType, nil
		}

		// If string unmarshal fails, try other JSON types and convert to string
		// This handles cases like: template is number but type is "text"

		// Try boolean
		var boolVal bool
		if err := json.Unmarshal(r.Template, &boolVal); err == nil {
			if boolVal {
				return "true", columnType, nil
			}
			return "false", columnType, nil
		}

		// Try number
		var numVal float64
		if err := json.Unmarshal(r.Template, &numVal); err == nil {
			if numVal == float64(int64(numVal)) {
				return fmt.Sprintf("%d", int64(numVal)), columnType, nil
			}
			return fmt.Sprintf("%f", numVal), columnType, nil
		}

		return "", "", fmt.Errorf("failed to parse template with explicit type '%s'", r.Type)
	}

	// Auto-detect type from JSON

	// Check for null
	if string(r.Template) == "null" {
		return "", "null", nil
	}

	// Try boolean
	var boolVal bool
	if err := json.Unmarshal(r.Template, &boolVal); err == nil {
		if boolVal {
			return "true", "boolean", nil
		}
		return "false", "boolean", nil
	}

	// Try number (integer or float)
	var numVal float64
	if err := json.Unmarshal(r.Template, &numVal); err == nil {
		// Convert to string, handle both int and float
		if numVal == float64(int64(numVal)) {
			return fmt.Sprintf("%d", int64(numVal)), "integer", nil
		}
		return fmt.Sprintf("%f", numVal), "integer", nil
	}

	// Try string (must be last, as it's the most permissive)
	var strVal string
	if err := json.Unmarshal(r.Template, &strVal); err == nil {
		return strVal, "text", nil
	}

	return "", "", fmt.Errorf("unsupported template type: %s", string(r.Template))
}

type UpdateAnonRulesRequest struct {
	Rules []CreateAnonRuleRequest `json:"rules" binding:"required"`
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

	// Parse template to detect type
	template, columnType, err := req.Parse()
	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to parse template")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template", "details": err.Error()})
		return
	}

	// Create anon rule (global, applies to all database restores)
	rule := models.AnonRule{
		Table:      req.Table,
		Column:     req.Column,
		Template:   template,
		ColumnType: columnType,
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
		Str("column_type", rule.ColumnType).
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

// @Router /api/anon-rules [put]
// @Param request body UpdateAnonRulesRequest true "Update anon rules request"
// @Success 200 {object} []models.AnonRule
func (s *Server) updateAnonRules(c *gin.Context) {
	var req UpdateAnonRulesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.logger.Warn().Err(err).Msg("Invalid request body")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	// Parse all rules first to validate
	var parsedRules []models.AnonRule
	for _, rule := range req.Rules {
		template, columnType, err := rule.Parse()
		if err != nil {
			s.logger.Warn().Err(err).Str("table", rule.Table).Str("column", rule.Column).Msg("Failed to parse template")
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid template for %s.%s", rule.Table, rule.Column), "details": err.Error()})
			return
		}
		parsedRules = append(parsedRules, models.AnonRule{
			Table:      rule.Table,
			Column:     rule.Column,
			Template:   template,
			ColumnType: columnType,
		})
	}

	// Use transaction to ensure atomicity (delete all + insert all)
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Delete all existing rules
		if err := tx.Where("1=1").Delete(&models.AnonRule{}).Error; err != nil {
			return err
		}

		// Insert new rules
		if len(parsedRules) > 0 {
			if err := tx.Create(&parsedRules).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to update anon rules")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update anonymization rules"})
		return
	}

	// Load and return the new rules
	var rules []models.AnonRule
	if err := s.db.Order("created_at DESC").Find(&rules).Error; err != nil {
		s.logger.Error().Err(err).Msg("Failed to load anon rules")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	s.logger.Info().
		Int("count", len(rules)).
		Msg("Updated anonymization rules")

	c.JSON(http.StatusOK, rules)
}
