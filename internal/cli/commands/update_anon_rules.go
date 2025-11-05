package commands

import (
	"fmt"

	"github.com/branchd-dev/branchd/internal/cli/client"
	"github.com/branchd-dev/branchd/internal/cli/config"
	"github.com/spf13/cobra"
)

// NewUpdateAnonRulesCmd creates the update-anon-rules command
func NewUpdateAnonRulesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-anon-rules",
		Short: "Update anonymization rules on all servers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdateAnonRules()
		},
	}

	return cmd
}

func runUpdateAnonRules() error {
	// Load config from current directory
	cfg, err := config.LoadFromCurrentDir()
	if err != nil {
		return fmt.Errorf("failed to load config: %w\nRun 'branchd init' to create a configuration file", err)
	}

	// Check if anonRules are defined
	if cfg.AnonRules == nil {
		return fmt.Errorf("no anonRules defined in branchd.json")
	}

	if len(cfg.Servers) == 0 {
		return fmt.Errorf("no servers configured. Run 'branchd init' to add a server")
	}

	// Convert config rules to client rules
	var rules []client.AnonRule
	for _, rule := range cfg.AnonRules {
		rules = append(rules, client.AnonRule{
			Table:    rule.Table,
			Column:   rule.Column,
			Template: rule.Template,
		})
	}

	// Update all servers
	for _, server := range cfg.Servers {
		if server.IP == "" {
			continue
		}

		fmt.Printf("Updating server '%s' (%s)... ", server.Alias, server.IP)

		// Create API client
		apiClient := client.New(server.IP)

		// Update rules on server
		if err := apiClient.UpdateAnonRules(server.IP, rules); err != nil {
			fmt.Printf("Failed to update server '%s': %v\n", server.Alias, err)
			continue
		}

		fmt.Printf("Done\n")
	}

	return nil
}
