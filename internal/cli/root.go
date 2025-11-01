package cli

import (
	"fmt"
	"os"

	"github.com/branchd-dev/branchd/internal/cli/commands"
	"github.com/branchd-dev/branchd/internal/cli/update"
	"github.com/spf13/cobra"
)

var version = "dev" // Will be set during build

var rootCmd = &cobra.Command{
	Use:   "branchd",
	Short: "Branchd - Database branching for PostgreSQL",
	Long: `Branchd CLI - Manage your database branches with ease.

Branchd uses ZFS copy-on-write snapshots to create instant database branches
for development, testing, and PR environments.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Skip update check for the update and version commands
		if cmd.Name() == "update" || cmd.Name() == "version" {
			return
		}

		// Check for updates (runs before every command except update/version)
		update.PrintUpdateNotification(version)
	},
}

func init() {
	// Add version command
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("branchd version %s\n", version)
		},
	})

	// Add all subcommands
	rootCmd.AddCommand(commands.NewInitCmd())
	rootCmd.AddCommand(commands.NewLoginCmd())
	rootCmd.AddCommand(commands.NewCheckoutCmd())
	rootCmd.AddCommand(commands.NewDeleteCmd())
	rootCmd.AddCommand(commands.NewListCmd())
	rootCmd.AddCommand(commands.NewDashCmd())
	rootCmd.AddCommand(commands.NewSelectServerCmd())
	rootCmd.AddCommand(commands.NewUpdateCmd(version))
	rootCmd.AddCommand(commands.NewUpdateServerCmd())
}

// Execute runs the root command
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}
	return nil
}
