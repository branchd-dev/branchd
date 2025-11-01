package commands

import (
	"fmt"

	"github.com/branchd-dev/branchd/internal/cli/client"
	"github.com/branchd-dev/branchd/internal/cli/config"
	"github.com/branchd-dev/branchd/internal/cli/serverselect"
	"github.com/spf13/cobra"
)

// NewCheckoutCmd creates the checkout command
func NewCheckoutCmd() *cobra.Command {
	var serverAlias string

	cmd := &cobra.Command{
		Use:   "checkout <branch-name>",
		Short: "Create a new database branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCheckout(args[0], serverAlias)
		},
	}

	cmd.Flags().StringVar(&serverAlias, "server", "", "Server alias (uses first server if not specified)")

	return cmd
}

func runCheckout(branchName, serverAlias string) error {
	// Load config
	cfg, err := config.LoadFromCurrentDir()
	if err != nil {
		return fmt.Errorf("failed to load config: %w\nRun 'branchd init' to create a configuration file", err)
	}

	// Resolve which server to use
	server, err := serverselect.ResolveServer(cfg, serverAlias)
	if err != nil {
		return err
	}

	if server.IP == "" {
		return fmt.Errorf("server IP is empty. Please edit branchd.json and add a valid IP address")
	}

	// Create API client
	apiClient := client.New(server.IP)

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
