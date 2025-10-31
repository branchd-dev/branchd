package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"github.com/branchd-dev/branchd/internal/caddy"
	"github.com/branchd-dev/branchd/internal/models"
	"github.com/branchd-dev/branchd/internal/pgclient"
)

// OnboardingDatabaseRequest represents the onboarding request
type OnboardingDatabaseRequest struct {
	ConnectionString string `json:"connectionString" binding:"required"`
	Domain           string `json:"domain"`           // Optional: custom domain for Let's Encrypt
	LetsEncryptEmail string `json:"letsEncryptEmail"` // Optional: email for Let's Encrypt (required if Domain is set)
}

// ConfigResponse represents the configuration response
type ConfigResponse struct {
	ID                   string     `json:"id"`
	ConnectionString     string     `json:"connection_string"`
	PostgresVersion      string     `json:"postgres_version"`
	SchemaOnly           bool       `json:"schema_only"`
	RefreshSchedule      string     `json:"refresh_schedule"`
	BranchPostgresqlConf string     `json:"branch_postgresql_conf"`
	DatabaseName         string     `json:"database_name"`
	Domain               string     `json:"domain"`
	LetsEncryptEmail     string     `json:"lets_encrypt_email"`
	MaxRestores          int        `json:"max_restores"`
	LastRefreshedAt      *time.Time `json:"last_refreshed_at"`
	NextRefreshAt        *time.Time `json:"next_refresh_at"`
	CreatedAt            time.Time  `json:"created_at"`
}

// UpdateConfigRequest represents the request to update configuration
type UpdateConfigRequest struct {
	ConnectionString string `json:"connectionString"`
	PostgresVersion  string `json:"postgresVersion"`
	SchemaOnly       *bool  `json:"schemaOnly"`
	RefreshSchedule  string `json:"refreshSchedule"`
	Domain           string `json:"domain"`
	LetsEncryptEmail string `json:"letsEncryptEmail"`
	MaxRestores      *int   `json:"maxRestores"`
}

// @Summary Get configuration
// @Description Get the current global configuration
// @Tags config
// @Produce json
// @Security BearerAuth
// @Success 200 {object} ConfigResponse
// @Failure 404 {object} map[string]interface{}
// @Router /api/config [get]
func (s *Server) getConfig(c *gin.Context) {
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

	c.JSON(http.StatusOK, ConfigResponse{
		ID:                   config.ID,
		ConnectionString:     redactConnectionString(config.ConnectionString),
		PostgresVersion:      config.PostgresVersion,
		SchemaOnly:           config.SchemaOnly,
		RefreshSchedule:      config.RefreshSchedule,
		BranchPostgresqlConf: config.BranchPostgresqlConf,
		DatabaseName:         config.DatabaseName,
		Domain:               config.Domain,
		LetsEncryptEmail:     config.LetsEncryptEmail,
		MaxRestores:          config.MaxRestores,
		LastRefreshedAt:      config.LastRefreshedAt,
		NextRefreshAt:        config.NextRefreshAt,
		CreatedAt:            config.CreatedAt,
	})
}

// @Summary Update configuration
// @Description Update the global configuration
// @Tags config
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body UpdateConfigRequest true "Configuration updates"
// @Success 200 {object} ConfigResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/config [patch]
func (s *Server) updateConfig(c *gin.Context) {
	var req UpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

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

	// Update connection string if provided
	if req.ConnectionString != "" {
		// Validate new connection string
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		// Log parsed URL components (without password) for debugging
		if parsedURL, err := url.Parse(req.ConnectionString); err == nil {
			s.logger.Info().
				Str("scheme", parsedURL.Scheme).
				Str("host", parsedURL.Host).
				Str("path", parsedURL.Path).
				Str("username", parsedURL.User.Username()).
				Bool("has_password", parsedURL.User.Username() != "").
				Msg("Attempting to connect to PostgreSQL")
		}

		client, err := pgclient.NewClient(req.ConnectionString)
		if err != nil {
			s.logger.Warn().Err(err).Msg("Failed to create PostgreSQL client")
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Failed to parse connection string",
				"details": err.Error(),
			})
			return
		}
		defer client.Close()

		// Test connection
		if err := client.Ping(ctx); err != nil {
			s.logger.Warn().
				Err(err).
				Str("error_type", "connection_failed").
				Msg("Failed to connect to PostgreSQL - check password encoding and network access")
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Failed to connect to database",
				"details": err.Error(),
			})
			return
		}

		// Get PostgreSQL version
		version, err := client.GetVersion(ctx)
		if err != nil {
			s.logger.Error().Err(err).Msg("Failed to get PostgreSQL version")
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Failed to get database version",
				"details": err.Error(),
			})
			return
		}

		majorVersion := extractMajorVersion(version)

		config.ConnectionString = req.ConnectionString
		config.PostgresVersion = majorVersion
	} else if req.PostgresVersion != "" {
		// Allow manual PostgreSQL version update if connection string not provided
		config.PostgresVersion = req.PostgresVersion
	}

	// Update schema-only flag if provided
	if req.SchemaOnly != nil {
		config.SchemaOnly = *req.SchemaOnly
	}

	// Update max restores if provided
	if req.MaxRestores != nil {
		if *req.MaxRestores < 1 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "max_restores must be at least 1",
			})
			return
		}
		config.MaxRestores = *req.MaxRestores
	}

	// Update refresh schedule (allow empty string to clear)
	config.RefreshSchedule = req.RefreshSchedule
	if req.RefreshSchedule != "" {
		// Calculate next refresh time for non-empty schedule
		nextRefreshAt := calculateNextRefresh(req.RefreshSchedule, time.Now())
		config.NextRefreshAt = nextRefreshAt
	} else {
		// Clear next refresh time when schedule is empty
		config.NextRefreshAt = nil
	}

	// Update domain and Let's Encrypt email if provided
	if req.Domain != "" && req.LetsEncryptEmail == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "lets_encrypt_email is required when domain is set",
		})
		return
	}

	config.Domain = req.Domain
	config.LetsEncryptEmail = req.LetsEncryptEmail

	// If domain is set, configure Caddy with Let's Encrypt
	if req.Domain != "" {
		if err := s.configureCaddy(req.Domain, req.LetsEncryptEmail); err != nil {
			s.logger.Error().Err(err).Msg("Failed to configure Caddy")
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to configure TLS certificate",
				"details": err.Error(),
			})
			return
		}
	}

	// Save updates
	if err := s.db.Save(&config).Error; err != nil {
		s.logger.Error().Err(err).Msg("Failed to update config")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update configuration"})
		return
	}

	s.logger.Info().Str("config_id", config.ID).Msg("Configuration updated")

	c.JSON(http.StatusOK, ConfigResponse{
		ID:                   config.ID,
		ConnectionString:     redactConnectionString(config.ConnectionString),
		PostgresVersion:      config.PostgresVersion,
		SchemaOnly:           config.SchemaOnly,
		RefreshSchedule:      config.RefreshSchedule,
		BranchPostgresqlConf: config.BranchPostgresqlConf,
		DatabaseName:         config.DatabaseName,
		Domain:               config.Domain,
		LetsEncryptEmail:     config.LetsEncryptEmail,
		MaxRestores:          config.MaxRestores,
		LastRefreshedAt:      config.LastRefreshedAt,
		NextRefreshAt:        config.NextRefreshAt,
		CreatedAt:            config.CreatedAt,
	})
}

// redactConnectionString replaces the password with *** in a PostgreSQL connection string
func redactConnectionString(connStr string) string {
	if connStr == "" {
		return ""
	}

	// Try parsing as URL first (postgresql://user:pass@host:port/dbname)
	if strings.HasPrefix(connStr, "postgresql://") || strings.HasPrefix(connStr, "postgres://") {
		u, err := url.Parse(connStr)
		if err == nil && u.User != nil {
			// Manually reconstruct the URL with redacted password to avoid URL encoding
			scheme := u.Scheme
			username := u.User.Username()
			host := u.Host
			path := u.Path
			query := u.RawQuery

			// Build the redacted URL manually
			redacted := scheme + "://" + username + ":***@" + host + path
			if query != "" {
				redacted += "?" + query
			}
			return redacted
		}
	}

	// Fallback: look for password= in key-value format
	parts := strings.Split(connStr, " ")
	var redacted []string
	for _, part := range parts {
		if strings.HasPrefix(part, "password=") {
			redacted = append(redacted, "password=***")
		} else {
			redacted = append(redacted, part)
		}
	}
	return strings.Join(redacted, " ")
}

// Helper function to extract major version from PostgreSQL version string
// e.g., "PostgreSQL 16.3 on x86_64-pc-linux-gnu" -> "16"
func extractMajorVersion(version string) string {
	// Handle two formats:
	// 1. "PostgreSQL 16.10" -> parts[1] = "16.10"
	// 2. "16.10" -> parts[0] = "16.10"

	parts := strings.Fields(version)
	var versionNum string

	if len(parts) >= 2 {
		// Format: "PostgreSQL 16.10"
		versionNum = parts[1]
	} else if len(parts) == 1 {
		// Format: "16.10"
		versionNum = parts[0]
	} else {
		return version
	}

	// Split by dot and take major version
	dotParts := strings.Split(versionNum, ".")
	if len(dotParts) > 0 {
		return dotParts[0]
	}

	return versionNum
}

// calculateNextRefresh calculates the next refresh time from a cron expression
func calculateNextRefresh(cronExpr string, from time.Time) *time.Time {
	if cronExpr == "" {
		return nil
	}

	// Parse cron expression
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(cronExpr)
	if err != nil {
		return nil
	}

	next := schedule.Next(from)
	return &next
}

// configureCaddy configures Caddy with the provided domain and Let's Encrypt email
// If domain is empty, Caddy will use self-signed certificates (default)
func (s *Server) configureCaddy(domain, email string) error {
	if s.caddyService == nil {
		return nil // Caddy service not initialized (e.g., in tests)
	}

	return s.caddyService.GenerateAndReload(caddy.Config{
		Domain:           domain,
		LetsEncryptEmail: email,
	})
}
