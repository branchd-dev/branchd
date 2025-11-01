package commands

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"

	"syscall"

	"github.com/branchd-dev/branchd/internal/cli/auth"
	"github.com/branchd-dev/branchd/internal/cli/client"
	"github.com/branchd-dev/branchd/internal/cli/config"
	"github.com/branchd-dev/branchd/internal/cli/serverselect"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// NewLoginCmd creates the login command
func NewLoginCmd() *cobra.Command {
	var email, password string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with a Branchd server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogin(email, password)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Email address (or set BRANCHD_EMAIL)")
	cmd.Flags().StringVar(&password, "password", "", "Password (or set BRANCHD_PASSWORD, will prompt if not provided)")

	return cmd
}

func runLogin(email, password string) error {
	// Check for environment variables (useful for CI/CD)
	if email == "" {
		email = os.Getenv("BRANCHD_EMAIL")
	}
	if password == "" {
		password = os.Getenv("BRANCHD_PASSWORD")
	}

	// Validate email
	if email == "" {
		return fmt.Errorf("email is required (use --email flag or BRANCHD_EMAIL env var)")
	}

	// Load config
	cfg, err := config.LoadFromCurrentDir()
	if err != nil {
		return fmt.Errorf("failed to load config: %w\nRun 'branchd init' to create a configuration file", err)
	}

	// Resolve which server to use (respects selected server from select-server command)
	server, err := serverselect.ResolveServer(cfg)
	if err != nil {
		return err
	}

	if server.IP == "" {
		return fmt.Errorf("server IP is empty. Please edit branchd.json and add a valid IP address")
	}

	// Prompt for password if not provided via flag or env var
	if password == "" {
		// Check if stdin is a terminal (not piped)
		if term.IsTerminal(int(syscall.Stdin)) {
			fmt.Print("Password: ")
			bytePassword, err := term.ReadPassword(int(syscall.Stdin))
			if err != nil {
				return fmt.Errorf("failed to read password: %w", err)
			}
			password = string(bytePassword)
			fmt.Println() // New line after password input
		} else {
			return fmt.Errorf("password is required in non-interactive mode (use --password flag or BRANCHD_PASSWORD env var)")
		}
	}

	// Create HTTP client that accepts self-signed certificates
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // Accept self-signed certificates
			},
		},
	}

	// Create API client with custom HTTP client
	apiClient := client.New(server.IP)
	apiClient.SetHTTPClient(httpClient)

	// Attempt login
	fmt.Printf("Logging in to %s (%s)...\n", server.Alias, server.IP)

	loginResp, err := apiClient.Login(email, password)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Save token
	if err := auth.SaveToken(server.IP, loginResp.Token); err != nil {
		return fmt.Errorf("failed to save authentication token: %w", err)
	}

	fmt.Println("✓ Login successful!")
	fmt.Printf("  User: %s (%s)\n", loginResp.User.Name, loginResp.User.Email)
	if loginResp.User.IsAdmin {
		fmt.Println("  Role: Admin")
	}

	return nil
}
