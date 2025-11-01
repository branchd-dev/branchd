package commands

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/branchd-dev/branchd/internal/cli/client"
	"github.com/spf13/cobra"
)

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

func runList() error {
	// Get selected server
	server, err := getSelectedServer()
	if err != nil {
		return err
	}

	// Create API client
	apiClient := client.New(server.IP)

	// List branches
	branches, err := apiClient.ListBranches(server.IP)
	if err != nil {
		return err
	}

	if len(branches) == 0 {
		fmt.Println("No branches found.")
		fmt.Println("\nCreate a branch with: branchd checkout <branch-name>")
		return nil
	}

	// Display branches in a table
	fmt.Printf("Branches on %s (%s):\n\n", server.Alias, server.IP)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
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
