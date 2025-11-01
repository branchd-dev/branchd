package commands

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/branchd-dev/branchd/internal/cli/client"
	"github.com/branchd-dev/branchd/internal/cli/config"
	"github.com/spf13/cobra"
)

// ListClient defines the interface for listing branches
type ListClient interface {
	ListBranches(serverIP string) ([]client.Branch, error)
}

// listOptions allows dependency injection for testing
type listOptions struct {
	apiClient ListClient
	server    *config.Server
	output    io.Writer
}

// ListOption is a function that configures listOptions
type ListOption func(*listOptions)

// WithListClient injects a custom API client (for testing)
func WithListClient(client ListClient) ListOption {
	return func(opts *listOptions) {
		opts.apiClient = client
	}
}

// WithListServer injects a specific server (for testing)
func WithListServer(server *config.Server) ListOption {
	return func(opts *listOptions) {
		opts.server = server
	}
}

// WithListOutput injects a custom output writer (for testing)
func WithListOutput(w io.Writer) ListOption {
	return func(opts *listOptions) {
		opts.output = w
	}
}

// NewListCmd creates the list command
func NewListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List all branches",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList()
		},
	}

	return cmd
}

func runList(opts ...ListOption) error {
	return runListWithOptions(opts...)
}

func runListWithOptions(opts ...ListOption) error {
	// Apply options
	options := &listOptions{
		output: os.Stdout, // Default to stdout
	}
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
	var apiClient ListClient
	if options.apiClient != nil {
		apiClient = options.apiClient
	} else {
		apiClient = client.New(server.IP)
	}

	// List branches
	branches, err := apiClient.ListBranches(server.IP)
	if err != nil {
		return err
	}

	if len(branches) == 0 {
		fmt.Fprintln(options.output, "No branches found.")
		fmt.Fprintln(options.output, "\nCreate a branch with: branchd checkout <branch-name>")
		return nil
	}

	// Display branches in a table
	fmt.Fprintf(options.output, "Branches on %s (%s):\n\n", server.Alias, server.IP)

	w := tabwriter.NewWriter(options.output, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tCREATED BY\tCREATED AT\tRESTORE")
	fmt.Fprintln(w, "────\t──────────\t──────────\t───────")

	for _, branch := range branches {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			branch.Name,
			branch.CreatedBy,
			branch.CreatedAt,
			branch.RestoreName,
		)
	}

	w.Flush()

	return nil
}
