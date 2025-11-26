package commands

import (
	"fmt"

	"github.com/branchd-dev/branchd/internal/cli/client"
	"github.com/branchd-dev/branchd/internal/cli/config"
	"github.com/spf13/cobra"
)

// NewUpdateConfigCmd creates the update-config command
func NewUpdateConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-config",
		Short: "Update configuration (anon rules, post-restore SQL) on all servers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdateConfig()
		},
	}

	return cmd
}

func runUpdateConfig() error {
	// Load config from current directory
	cfg, err := config.LoadFromCurrentDir()
	if err != nil {
		return fmt.Errorf("failed to load config: %w\nRun 'branchd init' to create a configuration file", err)
	}

	if len(cfg.Servers) == 0 {
		return fmt.Errorf("no servers configured. Run 'branchd init' to add a server")
	}

	// Check if there's anything to update
	hasAnonRules := cfg.AnonRules != nil && len(cfg.AnonRules) > 0
	hasPostRestoreSQL := cfg.PostRestoreSQL != ""

	if !hasAnonRules && !hasPostRestoreSQL {
		return fmt.Errorf("no anonRules or postRestoreSQL defined in branchd.json")
	}

	// Convert config rules to client rules
	var rules []client.AnonRule
	if hasAnonRules {
		for _, rule := range cfg.AnonRules {
			rules = append(rules, client.AnonRule{
				Table:    rule.Table,
				Column:   rule.Column,
				Template: rule.Template,
				Type:     rule.Type,
			})
		}
	}

	// Update all servers
	for _, server := range cfg.Servers {
		if server.IP == "" {
			continue
		}

		fmt.Printf("Updating configuration on server '%s' (%s)... ", server.Alias, server.IP)

		// Create API client
		apiClient := client.New(server.IP)

		// Update anon rules if defined
		if hasAnonRules {
			if err := apiClient.UpdateAnonRules(server.IP, rules); err != nil {
				fmt.Printf("Failed: %v\n", err)
				continue
			}
		}

		// Update post-restore SQL if defined
		if hasPostRestoreSQL {
			postRestoreSQL := cfg.PostRestoreSQL
			if err := apiClient.UpdateConfig(server.IP, &postRestoreSQL); err != nil {
				fmt.Printf("Failed: %v\n", err)
				continue
			}
		}

		fmt.Printf("Done\n")
	}

	return nil
}
