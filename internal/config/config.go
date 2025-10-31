package config

import (
	"os"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application
type Config struct {
	// Database Configuration
	Database DatabaseConfig

	// Redis Configuration
	Redis RedisConfig

	// Logging Configuration
	Logging LoggingConfig
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	URL string
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	Address string // Redis address (host:port)
}

// LoggingConfig holds logging-related configuration
type LoggingConfig struct {
	Level  string
	Format string // json, console
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	// Load .env files (fails silently if files don't exist)
	_ = godotenv.Load(".env")
	_ = godotenv.Load(".env.local")

	// Database URL - default to /data/branchd.sqlite, allow override for dev
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "branchd.sqlite"
	}

	// Redis address - default to localhost:6379, allow override for dev/docker
	redisAddr := os.Getenv("REDIS_ADDRESS")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	// Logging configuration - defaults suitable for production
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	logFormat := os.Getenv("LOG_FORMAT")
	if logFormat == "" {
		logFormat = "json"
	}

	return &Config{
		Database: DatabaseConfig{
			URL: dbURL,
		},
		Redis: RedisConfig{
			Address: redisAddr,
		},
		Logging: LoggingConfig{
			Level:  logLevel,
			Format: logFormat,
		},
	}, nil
}
