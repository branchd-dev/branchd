package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/branchd-dev/branchd/internal/cli/config"
	"github.com/spf13/cobra"
)

// NewInitCmd creates the init command
func NewInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init <ip-address>",
		Short: "Init a new branchd server",
		Args:  cobra.ExactArgs(1),
		RunE:  runInit,
	}
}

func runInit(cmd *cobra.Command, args []string) error {
	ipAddress := args[0]

	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	configPath := filepath.Join(currentDir, config.ConfigFileName)

	var cfg *config.Config
	isNewConfig := false

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		// Load existing config
		cfg, err = config.Load(configPath)
		if err != nil {
			return fmt.Errorf("failed to load existing config: %w", err)
		}
		fmt.Println("Found existing branchd.json")
	} else {
		// Create new config
		cfg = &config.Config{
			Servers: []config.Server{},
		}
		isNewConfig = true
	}

	// Check if server already exists
	serverExists := false
	for _, server := range cfg.Servers {
		if server.IP == ipAddress {
			serverExists = true
			break
		}
	}

	if serverExists {
		fmt.Printf("Server with IP %s already exists in branchd.json\n", ipAddress)
	} else {
		// Add new server
		alias := ipAddress
		if len(cfg.Servers) == 0 {
			alias = "production"
		} else {
			alias = fmt.Sprintf("server-%d", len(cfg.Servers)+1)
		}

		cfg.Servers = append(cfg.Servers, config.Server{
			IP:    ipAddress,
			Alias: alias,
		})

		// Save to file
		if err := config.Save(configPath, cfg); err != nil {
			return err
		}

		if isNewConfig {
			fmt.Printf("✓ Created ./branchd.json with server %s (%s)\n", ipAddress, alias)
		} else {
			fmt.Printf("✓ Added server %s (%s) to ./branchd.json\n", ipAddress, alias)
		}
	}

	// Open browser to setup page
	setupURL := fmt.Sprintf("https://%s/setup", ipAddress)
	fmt.Printf("\nOpening setup page at %s...\n", setupURL)

	if err := openBrowser(setupURL); err != nil {
		fmt.Printf("⚠ Could not open browser automatically: %v\n", err)
		fmt.Printf("Please visit: %s\n", setupURL)
	}

	fmt.Println("\nNext steps:")
	fmt.Println("  1. Complete the setup wizard in your browser")
	fmt.Println("  2. Run 'branchd login' to authenticate")

	return nil
}
