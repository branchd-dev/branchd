package logger

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Logger is the application logger instance
var Logger zerolog.Logger

// Init initializes the logger with the given configuration
func Init(level, format string) {
	// Set log level
	logLevel := parseLogLevel(level)
	zerolog.SetGlobalLevel(logLevel)

	// Configure output format
	if strings.ToLower(format) == "json" {
		Logger = zerolog.New(os.Stdout).With().
			Timestamp().
			Caller().
			Logger()
	} else {
		// Console format with colors
		output := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
			NoColor:    false,
		}
		Logger = zerolog.New(output).With().
			Timestamp().
			Caller().
			Logger()
	}

	// Set the global logger
	log.Logger = Logger
}

// parseLogLevel parses string log level to zerolog level
func parseLogLevel(level string) zerolog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}

// GetLogger returns the configured logger instance
func GetLogger() zerolog.Logger {
	return Logger
}
