package commands

import (
	"fmt"

	"github.com/branchd-dev/branchd/internal/cli/client"
	"github.com/branchd-dev/branchd/internal/cli/config"
	"github.com/spf13/cobra"
)

// CheckoutClient defines the interface for branch creation operations
type CheckoutClient interface {
	CreateBranch(serverIP, branchName string) (*client.CreateBranchResponse, error)
}

// checkoutOptions allows dependency injection for testing
type checkoutOptions struct {
	apiClient CheckoutClient
	server    *config.Server
}

// CheckoutOption is a function that configures checkoutOptions
type CheckoutOption func(*checkoutOptions)

// WithCheckoutClient injects a custom API client (for testing)
func WithCheckoutClient(client CheckoutClient) CheckoutOption {
	return func(opts *checkoutOptions) {
		opts.apiClient = client
	}
}

// WithCheckoutServer injects a specific server (for testing)
func WithCheckoutServer(server *config.Server) CheckoutOption {
	return func(opts *checkoutOptions) {
		opts.server = server
	}
}

// NewCheckoutCmd creates the checkout command
func NewCheckoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checkout <branch-name>",
		Short: "Create a new database branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCheckout(args[0])
		},
	}

	return cmd
}

func runCheckout(branchName string, opts ...CheckoutOption) error {
	return runCheckoutWithOptions(branchName, opts...)
}

func runCheckoutWithOptions(branchName string, opts ...CheckoutOption) error {
	// Apply options
	options := &checkoutOptions{}
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
	var apiClient CheckoutClient
	if options.apiClient != nil {
		apiClient = options.apiClient
	} else {
		apiClient = client.New(server.IP)
	}

	// Create branch
	branch, err := apiClient.CreateBranch(server.IP, branchName)
	if err != nil {
		return err
	}

	// Print only the connection string
	fmt.Printf("postgresql://%s:%s@%s:%d/%s\n",
		branch.User,
		branch.Password,
		branch.Host,
		branch.Port,
		branch.Database,
	)

	return nil
}
