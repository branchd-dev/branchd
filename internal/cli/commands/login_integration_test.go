package commands

import (
	"fmt"
	"testing"

	"github.com/branchd-dev/branchd/internal/cli/client"
	"github.com/branchd-dev/branchd/internal/cli/config"
)

// mockAPIClient simulates the API client for testing
type mockAPIClient struct {
	shouldFail bool
	email      string
	password   string
	token      string
}

func (m *mockAPIClient) Login(email, password string) (*client.LoginResponse, error) {
	if m.shouldFail || email != m.email || password != m.password {
		return nil, fmt.Errorf("login failed (status 401): {\"error\": \"invalid credentials\"}")
	}

	return &client.LoginResponse{
		Token: m.token,
		User: struct {
			ID      string `json:"id"`
			Email   string `json:"email"`
			Name    string `json:"name"`
			IsAdmin bool   `json:"is_admin"`
		}{
			ID:      "user-123",
			Email:   email,
			Name:    "Test User",
			IsAdmin: false,
		},
	}, nil
}

// mockLoginTokenStore is a simple in-memory token store for login tests
type mockLoginTokenStore struct {
	tokens map[string]string
}

func newMockLoginTokenStore() *mockLoginTokenStore {
	return &mockLoginTokenStore{
		tokens: make(map[string]string),
	}
}

func (m *mockLoginTokenStore) SaveToken(serverIP, token string) error {
	m.tokens[serverIP] = token
	return nil
}

// TestLoginIntegration_SuccessfulLogin tests the complete login flow
func TestLoginIntegration_SuccessfulLogin(t *testing.T) {
	// Setup
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockAPIClient{
		shouldFail: false,
		email:      "test@example.com",
		password:   "password123",
		token:      "jwt-token-abc",
	}

	tokenStore := newMockLoginTokenStore()

	// Execute login
	err := runLogin(
		"test@example.com",
		"password123",
		WithAPIClient(mockAPI),
		WithTokenStore(tokenStore),
		WithServer(server),
	)

	// Assert success
	if err != nil {
		t.Fatalf("expected successful login, got error: %v", err)
	}

	// Verify token was saved
	savedToken, exists := tokenStore.tokens[server.IP]
	if !exists {
		t.Fatal("expected token to be saved, but it wasn't")
	}

	if savedToken != "jwt-token-abc" {
		t.Errorf("expected token 'jwt-token-abc', got '%s'", savedToken)
	}
}

// TestLoginIntegration_FailedLogin tests login with wrong credentials
func TestLoginIntegration_FailedLogin(t *testing.T) {
	// Setup
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockAPIClient{
		shouldFail: false,
		email:      "test@example.com",
		password:   "correct-password",
		token:      "jwt-token-abc",
	}

	tokenStore := newMockLoginTokenStore()

	// Execute login with wrong password
	err := runLogin(
		"test@example.com",
		"wrong-password",
		WithAPIClient(mockAPI),
		WithTokenStore(tokenStore),
		WithServer(server),
	)

	// Assert failure
	if err == nil {
		t.Fatal("expected login to fail with wrong credentials, but it succeeded")
	}

	expectedError := "login failed: login failed (status 401): {\"error\": \"invalid credentials\"}"
	if err.Error() != expectedError {
		t.Errorf("expected error '%s', got '%s'", expectedError, err.Error())
	}

	// Verify token was NOT saved
	if len(tokenStore.tokens) > 0 {
		t.Error("expected no token to be saved after failed login")
	}
}

// TestLoginIntegration_WrongEmail tests login with incorrect email
func TestLoginIntegration_WrongEmail(t *testing.T) {
	// Setup
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockAPIClient{
		shouldFail: false,
		email:      "correct@example.com",
		password:   "password123",
		token:      "jwt-token-abc",
	}

	tokenStore := newMockLoginTokenStore()

	// Execute login with wrong email
	err := runLogin(
		"wrong@example.com",
		"password123",
		WithAPIClient(mockAPI),
		WithTokenStore(tokenStore),
		WithServer(server),
	)

	// Assert failure
	if err == nil {
		t.Fatal("expected login to fail with wrong email, but it succeeded")
	}

	// Verify token was NOT saved
	if len(tokenStore.tokens) > 0 {
		t.Error("expected no token to be saved after failed login")
	}
}

// TestLoginIntegration_MultipleServers tests that tokens are stored per-server
func TestLoginIntegration_MultipleServers(t *testing.T) {
	// Setup two servers
	server1 := &config.Server{
		Alias: "production",
		IP:    "192.168.1.100",
	}

	server2 := &config.Server{
		Alias: "staging",
		IP:    "192.168.1.101",
	}

	mockAPI := &mockAPIClient{
		shouldFail: false,
		email:      "test@example.com",
		password:   "password123",
		token:      "jwt-token-server1",
	}

	tokenStore := newMockLoginTokenStore()

	// Login to server1
	err := runLogin(
		"test@example.com",
		"password123",
		WithAPIClient(mockAPI),
		WithTokenStore(tokenStore),
		WithServer(server1),
	)
	if err != nil {
		t.Fatalf("failed to login to server1: %v", err)
	}

	// Login to server2 with different token
	mockAPI.token = "jwt-token-server2"
	err = runLogin(
		"test@example.com",
		"password123",
		WithAPIClient(mockAPI),
		WithTokenStore(tokenStore),
		WithServer(server2),
	)
	if err != nil {
		t.Fatalf("failed to login to server2: %v", err)
	}

	// Verify both tokens are stored separately
	if len(tokenStore.tokens) != 2 {
		t.Errorf("expected 2 tokens to be stored, got %d", len(tokenStore.tokens))
	}

	token1 := tokenStore.tokens[server1.IP]
	if token1 != "jwt-token-server1" {
		t.Errorf("expected server1 token 'jwt-token-server1', got '%s'", token1)
	}

	token2 := tokenStore.tokens[server2.IP]
	if token2 != "jwt-token-server2" {
		t.Errorf("expected server2 token 'jwt-token-server2', got '%s'", token2)
	}
}

// TestLoginIntegration_APIFailure tests handling of API failures
func TestLoginIntegration_APIFailure(t *testing.T) {
	// Setup
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockAPIClient{
		shouldFail: true, // Force API failure
	}

	tokenStore := newMockLoginTokenStore()

	// Execute login
	err := runLogin(
		"test@example.com",
		"password123",
		WithAPIClient(mockAPI),
		WithTokenStore(tokenStore),
		WithServer(server),
	)

	// Assert failure
	if err == nil {
		t.Fatal("expected login to fail when API fails, but it succeeded")
	}

	// Error should indicate login failure
	if err.Error()[:12] != "login failed" {
		t.Errorf("expected error to start with 'login failed', got '%s'", err.Error())
	}

	// Verify token was NOT saved
	if len(tokenStore.tokens) > 0 {
		t.Error("expected no token to be saved after API failure")
	}
}

// TestLoginIntegration_TokenOverwrite tests that re-login overwrites old token
func TestLoginIntegration_TokenOverwrite(t *testing.T) {
	// Setup
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockAPIClient{
		shouldFail: false,
		email:      "test@example.com",
		password:   "password123",
		token:      "old-token",
	}

	tokenStore := newMockLoginTokenStore()

	// First login
	err := runLogin(
		"test@example.com",
		"password123",
		WithAPIClient(mockAPI),
		WithTokenStore(tokenStore),
		WithServer(server),
	)
	if err != nil {
		t.Fatalf("first login failed: %v", err)
	}

	// Verify old token
	if tokenStore.tokens[server.IP] != "old-token" {
		t.Errorf("expected old token 'old-token', got '%s'", tokenStore.tokens[server.IP])
	}

	// Login again with new token
	mockAPI.token = "new-token"
	err = runLogin(
		"test@example.com",
		"password123",
		WithAPIClient(mockAPI),
		WithTokenStore(tokenStore),
		WithServer(server),
	)
	if err != nil {
		t.Fatalf("second login failed: %v", err)
	}

	// Verify new token overwrote old one
	if tokenStore.tokens[server.IP] != "new-token" {
		t.Errorf("expected new token 'new-token', got '%s'", tokenStore.tokens[server.IP])
	}

	// Verify only one token exists (not both)
	if len(tokenStore.tokens) != 1 {
		t.Errorf("expected 1 token after re-login, got %d", len(tokenStore.tokens))
	}
}
