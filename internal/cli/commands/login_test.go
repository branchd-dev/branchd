package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/branchd-dev/branchd/internal/cli/config"
)

// mockTokenStore is a simple in-memory token store for testing
type mockTokenStore struct {
	tokens map[string]string
}

func newMockTokenStore() *mockTokenStore {
	return &mockTokenStore{
		tokens: make(map[string]string),
	}
}

func (m *mockTokenStore) SaveToken(serverIP, token string) error {
	m.tokens[serverIP] = token
	return nil
}

func (m *mockTokenStore) LoadToken(serverIP string) (string, error) {
	token, exists := m.tokens[serverIP]
	if !exists {
		return "", fmt.Errorf("not authenticated. Please run 'branchd login' first")
	}
	return token, nil
}

func (m *mockTokenStore) DeleteToken(serverIP string) error {
	delete(m.tokens, serverIP)
	return nil
}

// setupTestEnvironment creates a temporary directory with test configs
func setupTestEnvironment(t *testing.T, servers []config.Server) (string, func()) {
	t.Helper()

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "branchd-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Create branchd.json
	cfg := config.Config{
		Servers: servers,
	}
	cfgData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	cfgPath := filepath.Join(tempDir, "branchd.json")
	if err := os.WriteFile(cfgPath, cfgData, 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Change to temp directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Return cleanup function
	cleanup := func() {
		os.Chdir(originalDir)
		os.RemoveAll(tempDir)
	}

	return tempDir, cleanup
}

// mockAPIServer creates a mock API server for testing
func mockAPIServer(t *testing.T, email, password, expectedToken string, shouldFail bool) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/login" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var loginReq struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}

		if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
			t.Errorf("failed to decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if shouldFail || loginReq.Email != email || loginReq.Password != password {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "invalid credentials"}`))
			return
		}

		// Success response
		w.WriteHeader(http.StatusOK)
		response := map[string]interface{}{
			"token": expectedToken,
			"user": map[string]interface{}{
				"id":       "user-123",
				"email":    loginReq.Email,
				"name":     "Test User",
				"is_admin": false,
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
}

func TestLoginCommand_SuccessfulLogin(t *testing.T) {
	// Setup test environment
	servers := []config.Server{
		{Alias: "test-server", IP: "127.0.0.1"},
	}
	_, cleanup := setupTestEnvironment(t, servers)
	defer cleanup()

	// Create mock API server
	mockServer := mockAPIServer(t, "test@example.com", "password123", "test-token-abc", false)
	defer mockServer.Close()

	// Create mock token store
	tokenStore := newMockTokenStore()

	// Get the server IP from the mock server (e.g., "127.0.0.1:12345")
	serverAddr := mockServer.URL[len("http://"):]

	// Update config to use mock server
	servers[0].IP = serverAddr
	_, cleanup2 := setupTestEnvironment(t, servers)
	defer cleanup2()

	// Set environment variables
	os.Setenv("BRANCHD_EMAIL", "test@example.com")
	os.Setenv("BRANCHD_PASSWORD", "password123")
	defer os.Unsetenv("BRANCHD_EMAIL")
	defer os.Unsetenv("BRANCHD_PASSWORD")

	// We need to refactor runLogin to accept dependencies
	// For now, let's test that the command structure is correct
	cmd := NewLoginCmd()

	if cmd.Use != "login" {
		t.Errorf("expected Use to be 'login', got %s", cmd.Use)
	}

	// Verify flags exist
	emailFlag := cmd.Flags().Lookup("email")
	if emailFlag == nil {
		t.Error("expected --email flag to exist")
	}

	passwordFlag := cmd.Flags().Lookup("password")
	if passwordFlag == nil {
		t.Error("expected --password flag to exist")
	}

	// Note: To properly test execution, we'd need to refactor runLogin
	// to accept injected dependencies (httpClient, tokenStore)
	// For now, this validates the command structure

	_ = tokenStore // We'll use this once we refactor for dependency injection
}

func TestLoginCommand_MissingEmail(t *testing.T) {
	// Setup test environment
	servers := []config.Server{
		{Alias: "test-server", IP: "127.0.0.1"},
	}
	_, cleanup := setupTestEnvironment(t, servers)
	defer cleanup()

	// Clear environment variables
	os.Unsetenv("BRANCHD_EMAIL")
	os.Unsetenv("BRANCHD_PASSWORD")

	// Test that runLogin fails without email
	err := runLogin("", "password123")
	if err == nil {
		t.Error("expected error when email is missing, got nil")
	}

	expectedError := "email is required (use --email flag or BRANCHD_EMAIL env var)"
	if err.Error() != expectedError {
		t.Errorf("expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestLoginCommand_NoConfigFile(t *testing.T) {
	// Create temp directory without branchd.json
	tempDir, err := os.MkdirTemp("", "branchd-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(originalDir)

	// Test that runLogin fails without config
	err = runLogin("test@example.com", "password123")
	if err == nil {
		t.Error("expected error when config file is missing, got nil")
	}

	// Should contain "failed to load config"
	if err != nil && len(err.Error()) > 0 {
		// Error message should guide user to run 'branchd init'
		if err.Error()[:22] != "failed to load config:" {
			t.Errorf("expected error to start with 'failed to load config:', got '%s'", err.Error())
		}
	}
}

func TestLoginCommand_EmptyServerIP(t *testing.T) {
	// Setup test environment with empty server IP
	servers := []config.Server{
		{Alias: "test-server", IP: ""},
	}
	_, cleanup := setupTestEnvironment(t, servers)
	defer cleanup()

	// Test that runLogin fails with empty server IP
	err := runLogin("test@example.com", "password123")
	if err == nil {
		t.Error("expected error when server IP is empty, got nil")
	}

	expectedError := "server IP is empty. Please edit branchd.json and add a valid IP address"
	if err != nil && err.Error() != expectedError {
		t.Errorf("expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestLoginCommand_EnvVarCredentials(t *testing.T) {
	// Setup test environment
	servers := []config.Server{
		{Alias: "test-server", IP: "127.0.0.1"},
	}
	_, cleanup := setupTestEnvironment(t, servers)
	defer cleanup()

	// Set environment variables
	os.Setenv("BRANCHD_EMAIL", "env@example.com")
	os.Setenv("BRANCHD_PASSWORD", "envpass")
	defer os.Unsetenv("BRANCHD_EMAIL")
	defer os.Unsetenv("BRANCHD_PASSWORD")

	// When we call runLogin with empty strings, it should use env vars
	// This will fail at the API call stage, but we can verify it reads env vars
	// by checking that it doesn't fail on the email validation
	err := runLogin("", "")

	// Should NOT fail with "email is required" since env var is set
	if err != nil && err.Error() == "email is required (use --email flag or BRANCHD_EMAIL env var)" {
		t.Error("runLogin should have read email from BRANCHD_EMAIL env var")
	}

	// It will fail later (likely at API call), but that's expected
	// The important thing is it got past the email validation
}

func TestLoginCommand_MultipleServers(t *testing.T) {
	// Setup test environment with multiple servers
	servers := []config.Server{
		{Alias: "production", IP: "192.168.1.100"},
		{Alias: "staging", IP: "192.168.1.101"},
	}
	_, cleanup := setupTestEnvironment(t, servers)
	defer cleanup()

	// Set environment variables so we don't prompt for password
	os.Setenv("BRANCHD_EMAIL", "test@example.com")
	os.Setenv("BRANCHD_PASSWORD", "password123")
	defer os.Unsetenv("BRANCHD_EMAIL")
	defer os.Unsetenv("BRANCHD_PASSWORD")

	// When multiple servers exist and none is selected,
	// the command should work (it will prompt or use the first server in non-interactive mode)
	// This will fail at API call, but validates server selection logic works
	err := runLogin("test@example.com", "password123")

	// Should not fail due to server selection issues
	// It will fail at network call (expected), but error should be about connection, not server selection
	if err != nil {
		// Verify it's a network error, not a config error
		errMsg := err.Error()
		if errMsg == "no servers configured in branchd.json" {
			t.Error("server selection failed - should have selected a server")
		}
	}
}
