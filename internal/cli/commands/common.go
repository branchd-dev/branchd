package commands

import (
	"fmt"

	"github.com/branchd-dev/branchd/internal/cli/config"
	"github.com/branchd-dev/branchd/internal/cli/serverselect"
)

// getSelectedServer loads the config and returns the selected server.
// This is common logic used by most commands.
// If you need the config object itself, call config.LoadFromCurrentDir() separately.
func getSelectedServer() (*config.Server, error) {
	// Load config
	cfg, err := config.LoadFromCurrentDir()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w\nRun 'branchd init' to create a configuration file", err)
	}

	// Resolve which server to use
	server, err := serverselect.ResolveServer(cfg)
	if err != nil {
		return nil, err
	}

	if server.IP == "" {
		return nil, fmt.Errorf("server IP is empty. Please edit branchd.json and add a valid IP address")
	}

	return server, nil
}
