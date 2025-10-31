package main

import (
	"fmt"
	"os"

	"github.com/branchd-dev/branchd/internal/config"
	"github.com/branchd-dev/branchd/internal/logger"
	"github.com/branchd-dev/branchd/internal/server"
)

var version = "dev" // Will be set during build with -ldflags

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger.Init(cfg.Logging.Level, cfg.Logging.Format)
	log := logger.GetLogger()

	// Create server
	srv, err := server.New(cfg, log, version)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create server")
	}

	log.Info().Str("version", version).Msg("Starting Branchd server...")

	// Start HTTP server (this blocks)
	if err := srv.Start(); err != nil {
		log.Fatal().Err(err).Msg("Server failed to start")
	}
}
