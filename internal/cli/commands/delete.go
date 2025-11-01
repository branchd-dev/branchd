package commands

import (
	"fmt"

	"github.com/branchd-dev/branchd/internal/cli/client"
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
	// Get selected server
	server, err := getSelectedServer()
	if err != nil {
		return err
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
