package commands

import (
	"fmt"
	"os"

	"syscall"

	"github.com/branchd-dev/branchd/internal/cli/client"
	"github.com/branchd-dev/branchd/internal/cli/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// LoginClient defines the interface for API login operations
type LoginClient interface {
	Login(email, password string) (*client.LoginResponse, error)
}

// LoginTokenStore defines the interface for token storage
type LoginTokenStore interface {
	SaveToken(serverIP, token string) error
}

// loginOptions allows dependency injection for testing
type loginOptions struct {
	apiClient  LoginClient
	tokenStore LoginTokenStore
	server     *config.Server
}

// LoginOption is a function that configures loginOptions
type LoginOption func(*loginOptions)

// WithAPIClient injects a custom API client (for testing)
func WithAPIClient(client LoginClient) LoginOption {
	return func(opts *loginOptions) {
		opts.apiClient = client
	}
}

// WithTokenStore injects a custom token store (for testing)
func WithTokenStore(store LoginTokenStore) LoginOption {
	return func(opts *loginOptions) {
		opts.tokenStore = store
	}
}

// WithServer injects a specific server (for testing)
func WithServer(server *config.Server) LoginOption {
	return func(opts *loginOptions) {
		opts.server = server
	}
}

// NewLoginCmd creates the login command
func NewLoginCmd() *cobra.Command {
	var email, password string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login to a branchd server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogin(email, password)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Email address (or set BRANCHD_EMAIL)")
	cmd.Flags().StringVar(&password, "password", "", "Password (or set BRANCHD_PASSWORD, will prompt if not provided)")

	return cmd
}

func runLogin(email, password string, opts ...LoginOption) error {
	return runLoginWithOptions(email, password, opts...)
}

func runLoginWithOptions(email, password string, opts ...LoginOption) error {
	// Apply options
	options := &loginOptions{}
	for _, opt := range opts {
		opt(options)
	}
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

	// Get selected server (unless injected for testing)
	var server *config.Server
	var err error
	if options.server != nil {
		server = options.server
	} else {
		server, err = getSelectedServer()
		if err != nil {
			return err
		}
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

	// Create API client (or use injected one for testing)
	var apiClient LoginClient
	if options.apiClient != nil {
		apiClient = options.apiClient
	} else {
		apiClient = client.New(server.IP)
	}

	// Create token store (or use injected one for testing)
	var tokenStore LoginTokenStore
	if options.tokenStore != nil {
		tokenStore = options.tokenStore
	} else {
		tokenStore = &defaultTokenStore{}
	}

	// Attempt login
	fmt.Printf("Logging in to %s (%s)...\n", server.Alias, server.IP)

	loginResp, err := apiClient.Login(email, password)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Save token
	if err := tokenStore.SaveToken(server.IP, loginResp.Token); err != nil {
		return fmt.Errorf("failed to save authentication token: %w", err)
	}

	fmt.Println("✓ Login successful!")
	fmt.Printf("  User: %s (%s)\n", loginResp.User.Name, loginResp.User.Email)
	if loginResp.User.IsAdmin {
		fmt.Println("  Role: Admin")
	}

	return nil
}
