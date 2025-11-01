package commands

import (
	"fmt"

	"github.com/branchd-dev/branchd/internal/cli/config"
	"github.com/branchd-dev/branchd/internal/cli/serverselect"
	"github.com/branchd-dev/branchd/internal/cli/userconfig"
	"github.com/spf13/cobra"
)

// NewSelectServerCmd creates the select-server command
func NewSelectServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "select-server [ip-or-alias]",
		Short: "Select the server to use for commands",
		Long: `Select the server to use for commands.

If no param is provided, an interactive prompt will be shown.

Examples:
  $ branchd select-server              # Interactive selection
  $ branchd select-server 192.168.1.1  # Select by IP
  $ branchd select-server production   # Select by alias`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var ipOrAlias string
			if len(args) > 0 {
				ipOrAlias = args[0]
			}
			return runSelectServer(ipOrAlias)
		},
	}

	return cmd
}

func runSelectServer(ipOrAlias string) error {
	// Load project config
	cfg, err := config.LoadFromCurrentDir()
	if err != nil {
		return fmt.Errorf("failed to load config: %w\nRun 'branchd init' to create a configuration file", err)
	}

	var server *config.Server

	if ipOrAlias != "" {
		// User provided an IP or alias, find it
		server, err = serverselect.GetServerByIPOrAlias(cfg, ipOrAlias)
		if err != nil {
			return err
		}
	} else {
		// Show interactive selection
		server, err = serverselect.PromptServerSelection(cfg)
		if err != nil {
			return err
		}
	}

	// Save the selected server
	if err := userconfig.SetSelectedServer(server.IP); err != nil {
		return fmt.Errorf("failed to save selected server: %w", err)
	}

	fmt.Printf("Selected server: %s (%s)\n", server.Alias, server.IP)
	return nil
}
