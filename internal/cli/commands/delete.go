package commands

import (
	"fmt"

	"github.com/branchd-dev/branchd/internal/cli/client"
	"github.com/branchd-dev/branchd/internal/cli/config"
	"github.com/spf13/cobra"
)

// DeleteClient defines the interface for delete operations
type DeleteClient interface {
	ListBranches(serverIP string) ([]client.Branch, error)
	DeleteBranch(serverIP, branchID string) error
}

// deleteOptions allows dependency injection for testing
type deleteOptions struct {
	apiClient DeleteClient
	server    *config.Server
}

// DeleteOption is a function that configures deleteOptions
type DeleteOption func(*deleteOptions)

// WithDeleteClient injects a custom API client (for testing)
func WithDeleteClient(client DeleteClient) DeleteOption {
	return func(opts *deleteOptions) {
		opts.apiClient = client
	}
}

// WithDeleteServer injects a specific server (for testing)
func WithDeleteServer(server *config.Server) DeleteOption {
	return func(opts *deleteOptions) {
		opts.server = server
	}
}

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

func runDelete(branchName string, opts ...DeleteOption) error {
	return runDeleteWithOptions(branchName, opts...)
}

func runDeleteWithOptions(branchName string, opts ...DeleteOption) error {
	// Apply options
	options := &deleteOptions{}
	for _, opt := range opts {
		opt(options)
	}
	// Get selected server (unless injected for testing)
	var server *config.Server
	var err error
	if options.server != nil {
		server = options.server
	} else {
		server, err = getSelectedServer()
		if err != nil {
			return err
		}
	}

	// Create API client (or use injected one for testing)
	var apiClient DeleteClient
	if options.apiClient != nil {
		apiClient = options.apiClient
	} else {
		apiClient = client.New(server.IP)
	}

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

	fmt.Printf("✓ Branch '%s' deleted successfully\n", branchName)
	return nil
}
