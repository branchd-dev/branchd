package commands

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

// NewDashCmd creates the dash command
func NewDashCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dash",
		Short: "Open the web dashboard in browser",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDash()
		},
	}

	return cmd
}

func runDash() error {
	// Get selected server
	server, err := getSelectedServer()
	if err != nil {
		return err
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
