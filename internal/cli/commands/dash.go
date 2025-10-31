package commands

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/branchd-dev/branchd/internal/cli/config"
	"github.com/spf13/cobra"
)

// NewDashCmd creates the dash command
func NewDashCmd() *cobra.Command {
	var serverAlias string

	cmd := &cobra.Command{
		Use:   "dash",
		Short: "Open the web dashboard in browser",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDash(serverAlias)
		},
	}

	cmd.Flags().StringVar(&serverAlias, "server", "", "Server alias (uses first server if not specified)")

	return cmd
}

func runDash(serverAlias string) error {
	// Load config
	cfg, err := config.LoadFromCurrentDir()
	if err != nil {
		return fmt.Errorf("failed to load config: %w\nRun 'branchd init' to create a configuration file", err)
	}

	// Get server
	var server *config.Server
	if serverAlias != "" {
		server, err = cfg.GetServerByAlias(serverAlias)
		if err != nil {
			return err
		}
	} else {
		server, err = cfg.GetDefaultServer()
		if err != nil {
			return err
		}
	}

	if server.IP == "" {
		return fmt.Errorf("server IP is empty. Please edit branchd.json and add a valid IP address")
	}

	// Build dashboard URL (Caddy serves HTTPS on port 443)
	dashboardURL := fmt.Sprintf("https://%s", server.IP)

	fmt.Printf("Opening dashboard for %s (%s)...\n", server.Alias, server.IP)
	fmt.Printf("URL: %s\n", dashboardURL)

	// Open browser based on OS
	if err := openBrowser(dashboardURL); err != nil {
		return fmt.Errorf("failed to open browser: %w\nPlease visit: %s", err, dashboardURL)
	}

	return nil
}

// openBrowser opens the URL in the default browser
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Start()
}
