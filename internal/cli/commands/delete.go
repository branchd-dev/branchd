package commands

import (
	"fmt"

	"github.com/branchd-dev/branchd/internal/cli/client"
	"github.com/branchd-dev/branchd/internal/cli/config"
	"github.com/branchd-dev/branchd/internal/cli/serverselect"
	"github.com/spf13/cobra"
)

// NewDeleteCmd creates the delete command
func NewDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <branch-name>",
		Short: "Delete a database branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(args[0])
		},
	}

	return cmd
}

func runDelete(branchName string) error {
	// Load config
	cfg, err := config.LoadFromCurrentDir()
	if err != nil {
		return fmt.Errorf("failed to load config: %w\nRun 'branchd init' to create a configuration file", err)
	}

	// Resolve which server to use
	server, err := serverselect.ResolveServer(cfg)
	if err != nil {
		return err
	}

	if server.IP == "" {
		return fmt.Errorf("server IP is empty. Please edit branchd.json and add a valid IP address")
	}

	// Create API client
	apiClient := client.New(server.IP)

	// First, list branches to find the one with matching name
	branches, err := apiClient.ListBranches(server.IP)
	if err != nil {
		return fmt.Errorf("failed to list branches: %w", err)
	}

	// Find branch by name
	var branchID string
	for _, branch := range branches {
		if branch.Name == branchName {
			branchID = branch.ID
			break
		}
	}

	if branchID == "" {
		return fmt.Errorf("branch '%s' not found", branchName)
	}

	if err := apiClient.DeleteBranch(server.IP, branchID); err != nil {
		return err
	}

	return nil
}
