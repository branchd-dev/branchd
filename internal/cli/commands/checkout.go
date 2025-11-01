package commands

import (
	"fmt"

	"github.com/branchd-dev/branchd/internal/cli/client"
	"github.com/spf13/cobra"
)

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

func runCheckout(branchName string) error {
	// Get selected server
	server, err := getSelectedServer()
	if err != nil {
		return err
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
