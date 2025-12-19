// Package server
//
// @title Branchd API
// @version 1.0
// @description Database branching service API
// @host localhost:8080
// @BasePath /
package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-playground/validator/v10"
	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/branchd-dev/branchd/internal/auth"
	"github.com/branchd-dev/branchd/internal/branches"
	"github.com/branchd-dev/branchd/internal/caddy"
	"github.com/branchd-dev/branchd/internal/config"
	"github.com/branchd-dev/branchd/internal/models"
	"github.com/branchd-dev/branchd/internal/restores"
)

// Server represents the HTTP server
type Server struct {
	router          *gin.Engine
	db              *gorm.DB
	config          *config.Config
	logger          zerolog.Logger
	validator       *validator.Validate
	asynqClient     *asynq.Client
	branchesService *branches.Service
	restoresService *restores.Service
	caddyService    *caddy.Service
	version         string
}

// New creates a new server instance
func New(cfg *config.Config, zlog zerolog.Logger, version string) (*Server, error) {
	// Initialize database with production settings
	db, err := initDatabase(cfg, zlog)
	if err != nil {
		return nil, err
	}

	// Run database migrations
	if err := models.AutoMigrate(db); err != nil {
		return nil, err
	}

	// Initialize JWT authentication
	// Load JWT secret from database (auto-generated during first setup)
	var config models.Config
	if err := db.First(&config).Error; err == nil {
		// Config exists, use persisted JWT secret
		auth.InitializeJWT(config.JWTSecret)
		zlog.Debug().Msg("Loaded JWT secret from database")
	} else {
		// No config yet - first setup hasn't happened
		// JWT will be initialized during setupFirstAdmin
		zlog.Info().Msg("No config found - JWT will be initialized during first setup")
	}

	// Initialize validator
	validate := validator.New()

	// Register custom validators
	validate.RegisterValidation("alphanumdash", func(fl validator.FieldLevel) bool {
		// Allow alphanumeric, hyphens, and underscores only (safe for filesystem paths)
		value := fl.Field().String()
		for _, char := range value {
			if !((char >= 'a' && char <= 'z') ||
				(char >= 'A' && char <= 'Z') ||
				(char >= '0' && char <= '9') ||
				char == '-' ||
				char == '_') {
				return false
			}
		}
		return true
	})

	// Initialize Asynq client for enqueueing tasks
	asynqClient := asynq.NewClient(asynq.RedisClientOpt{
		Addr: cfg.Redis.Address,
	})

	// Initialize branches service (now runs locally, no SSH client needed)
	branchesService := branches.NewService(db, cfg, zlog)

	// Initialize restores service
	restoresService := restores.NewService(db, zlog)

	// Initialize Caddy service for TLS configuration
	caddyService, err := caddy.NewService(zlog)
	if err != nil {
		zlog.Warn().Err(err).Msg("Failed to initialize Caddy service - TLS configuration will be disabled")
		caddyService = nil
	}

	// Create server
	server := &Server{
		db:              db,
		config:          cfg,
		logger:          zlog,
		validator:       validate,
		asynqClient:     asynqClient,
		branchesService: branchesService,
		restoresService: restoresService,
		caddyService:    caddyService,
		version:         version,
	}

	// Setup router
	server.setupRouter()

	return server, nil
}

// initDatabase initializes the database connection with production settings
func initDatabase(cfg *config.Config, zlog zerolog.Logger) (*gorm.DB, error) {
	const (
		maxOpenConns      = 8         // Reduced for SQLite efficiency
		maxIdleConns      = 4         // Reduced proportionally
		connMaxLifetime   = 300       // 5 minutes
		busyTimeout       = 5000      // 5 seconds
		cacheSize         = 10000     // 10MB
		mmapSize          = 134217728 // 128MB
		walAutocheckpoint = 1000      // WAL auto-checkpoint pages
	)

	// Open database connection
	db, err := gorm.Open(sqlite.Open(cfg.Database.URL), &gorm.Config{
		Logger: logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			logger.Config{
				LogLevel:                  logger.Error,
				IgnoreRecordNotFoundError: true,
				SlowThreshold:             200 * time.Millisecond,
			},
		),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Get underlying sql.DB to configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// Configure connection pool settings
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(connMaxLifetime) * time.Second)

	// Test the connection
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Apply SQLite pragmas directly (connection string pragmas may not work with all drivers)
	// WAL mode must be set first for optimal concurrency
	pragmas := []string{
		"PRAGMA journal_mode=WAL",                                      // Enable WAL mode for better concurrency
		"PRAGMA synchronous=NORMAL",                                    // Faster than FULL, still safe with WAL
		fmt.Sprintf("PRAGMA wal_autocheckpoint=%d", walAutocheckpoint), // Auto-checkpoint WAL file
		fmt.Sprintf("PRAGMA busy_timeout=%d", busyTimeout),
		fmt.Sprintf("PRAGMA cache_size=-%d", cacheSize),
		"PRAGMA foreign_keys=1",
		"PRAGMA temp_store=2",
		fmt.Sprintf("PRAGMA mmap_size=%d", mmapSize),
	}

	for _, pragma := range pragmas {
		if err := db.Exec(pragma).Error; err != nil {
			zlog.Warn().Str("pragma", pragma).Err(err).Msg("Failed to apply pragma")
		}
	}

	// Log database pragma settings for verification
	var walMode, synchronousMode string
	var busyTimeoutVal, cacheSizeVal, foreignKeysVal, tempStoreVal, mmapSizeVal, walAutocheckpointVal int
	db.Raw("PRAGMA journal_mode").Scan(&walMode)
	db.Raw("PRAGMA synchronous").Scan(&synchronousMode)
	db.Raw("PRAGMA wal_autocheckpoint").Scan(&walAutocheckpointVal)
	db.Raw("PRAGMA busy_timeout").Scan(&busyTimeoutVal)
	db.Raw("PRAGMA cache_size").Scan(&cacheSizeVal)
	db.Raw("PRAGMA foreign_keys").Scan(&foreignKeysVal)
	db.Raw("PRAGMA temp_store").Scan(&tempStoreVal)
	db.Raw("PRAGMA mmap_size").Scan(&mmapSizeVal)

	return db, nil
}

// setupRouter configures the Gin router with routes and middleware
func (s *Server) setupRouter() {
	// Set Gin mode based on environment
	gin.SetMode(gin.ReleaseMode)

	s.router = gin.New()

	// Add middleware
	s.router.Use(gin.Recovery())
	s.router.Use(s.loggingMiddleware())

	// CORS middleware
	s.router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Health check endpoint (no auth required)
	s.router.GET("/health", s.healthCheck)

	// Public auth endpoints (no auth required)
	s.router.POST("/api/setup", s.setupFirstAdmin)
	s.router.POST("/api/auth/login", s.login)

	// Authenticated API routes (JWT required)
	api := s.router.Group("/api")
	api.Use(JWTAuthMiddleware(s.db, s.logger))
	{
		// Auth endpoints
		api.GET("/auth/me", s.getCurrentUser)

		// System information
		api.GET("/system/info", s.getSystemInfo)
		api.GET("/system/latest-version", s.getLatestVersion)
		api.POST("/system/update", s.updateServer)

		// User management (admin only)
		userRoutes := api.Group("/users")
		userRoutes.Use(AdminOnlyMiddleware(s.logger))
		{
			userRoutes.GET("", s.listUsers)
			userRoutes.POST("", s.createUser)
			userRoutes.DELETE("/:id", s.deleteUser)
		}

		// Onboarding & Configuration
		api.GET("/config", s.getConfig)
		api.PATCH("/config", s.updateConfig)

		// Database management
		api.GET("/restores", s.listRestores)
		api.GET("/restores/:id", s.getRestore)
		api.GET("/restores/:id/logs", s.getRestoreLogs)
		api.DELETE("/restores/:id", s.deleteRestore)
		api.POST("/restores/trigger-restore", s.triggerRestore)
		api.POST("/restores/:id/anonymize", s.applyAnonymization)

		// Anonymization rules (global)
		api.GET("/anon-rules", s.listAnonRules)
		api.POST("/anon-rules", s.createAnonRule)
		api.PUT("/anon-rules", s.updateAnonRules)
		api.DELETE("/anon-rules/:id", s.deleteAnonRule)

		// Branches
		api.GET("/branches", s.listBranches)
		api.POST("/branches", s.createBranch)
		api.DELETE("/branches/:id", s.deleteBranch)
	}
}

// loggingMiddleware creates a custom logging middleware using zerolog
func (s *Server) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		duration := time.Since(start)

		s.logger.Info().
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status", c.Writer.Status()).
			Dur("duration", duration).
			Str("client_ip", c.ClientIP()).
			Msg("HTTP request")
	}
}

// @Router /health [get]
// @Success 200 {object} map[string]interface{}
func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "online",
		"timestamp": time.Now().UTC(),
		"service":   "branchd-api",
	})
}

// GetDB returns the database connection for use by workers
func (s *Server) GetDB() *gorm.DB {
	return s.db
}

// Start starts the HTTP server
func (s *Server) Start() error {
	port := ":8080"

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create HTTP server with production timeouts
	srv := &http.Server{
		Addr:    port,
		Handler: s.router,
		// Timeouts for long-running operations like branch creation
		ReadTimeout:       180 * time.Second, // 3 minutes
		WriteTimeout:      180 * time.Second, // 3 minutes
		ReadHeaderTimeout: 30 * time.Second,  // 30 seconds
		IdleTimeout:       300 * time.Second, // 5 minutes
	}

	// Start server in goroutine
	go func() {
		s.logger.Info().Str("port", port).Msg("Starting HTTP server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error().Err(err).Msg("HTTP server error")
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	s.logger.Info().Msg("Received shutdown signal, shutting down gracefully...")

	// Close Asynq client
	if err := s.asynqClient.Close(); err != nil {
		s.logger.Warn().Err(err).Msg("Error closing Asynq client")
	}
	s.logger.Info().Msg("Asynq client closed successfully")

	// Shutdown HTTP server with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	s.logger.Info().Msg("Shutting down HTTP server...")
	if err := srv.Shutdown(shutdownCtx); err != nil {
		s.logger.Error().Err(err).Msg("Error shutting down HTTP server")
		return err
	}

	s.logger.Info().Msg("Server shutdown complete")

	// Close database connection to flush WAL writes
	if sqlDB, err := s.db.DB(); err == nil {
		s.logger.Info().Msg("Closing database connection...")
		if err := sqlDB.Close(); err != nil {
			s.logger.Error().Err(err).Msg("Error closing database")
		} else {
			s.logger.Info().Msg("Database closed successfully")
		}
	}

	return nil
}
