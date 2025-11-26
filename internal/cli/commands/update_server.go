package commands

import (
	"fmt"

	"github.com/branchd-dev/branchd/internal/cli/client"
	"github.com/branchd-dev/branchd/internal/cli/config"
	"github.com/spf13/cobra"
)

// NewUpdateServerCmd creates the update-server command
func NewUpdateServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-server",
		Short: "Update all branchd servers version",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdateServer()
		},
	}

	return cmd
}

func runUpdateServer() error {
	// Load config
	cfg, err := config.LoadFromCurrentDir()
	if err != nil {
		return fmt.Errorf("failed to load config: %w\nRun 'branchd init' to create a configuration file", err)
	}

	if len(cfg.Servers) == 0 {
		return fmt.Errorf("no servers configured. Run 'branchd init' to add a server")
	}

	// Update all servers
	for _, server := range cfg.Servers {
		if server.IP == "" {
			fmt.Printf("Skipping server '%s' (no IP configured)\n", server.Alias)
			continue
		}

		// Create API client
		apiClient := client.New(server.IP)

		// Trigger update
		if err := apiClient.UpdateServer(server.IP); err != nil {
			fmt.Printf("Failed to update server '%s': %v\n", server.Alias, err)
			continue
		}

		fmt.Printf("Update triggered on server '%s'\n", server.Alias)
	}

	return nil
}
