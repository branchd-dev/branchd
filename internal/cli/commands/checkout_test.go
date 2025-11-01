package commands

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/branchd-dev/branchd/internal/cli/client"
	"github.com/branchd-dev/branchd/internal/cli/config"
)

// mockCheckoutClient simulates the API client for checkout tests
type mockCheckoutClient struct {
	shouldFail bool
	branchName string
	response   *client.CreateBranchResponse
}

func (m *mockCheckoutClient) CreateBranch(serverIP, branchName string) (*client.CreateBranchResponse, error) {
	if m.shouldFail {
		return nil, fmt.Errorf("failed to create branch (status 500): internal server error")
	}

	if m.branchName != "" && branchName != m.branchName {
		return nil, fmt.Errorf("failed to create branch (status 400): branch name mismatch")
	}

	return m.response, nil
}

// TestCheckoutCommand_CommandStructure tests the command is properly configured
func TestCheckoutCommand_CommandStructure(t *testing.T) {
	cmd := NewCheckoutCmd()

	if cmd.Use != "checkout <branch-name>" {
		t.Errorf("expected Use to be 'checkout <branch-name>', got %s", cmd.Use)
	}

	if cmd.Short != "Create a new database branch" {
		t.Errorf("expected Short description, got %s", cmd.Short)
	}

	// Test that it requires exactly 1 argument
	err := cmd.Args(cmd, []string{})
	if err == nil {
		t.Error("expected error when no branch name provided")
	}

	err = cmd.Args(cmd, []string{"branch1", "branch2"})
	if err == nil {
		t.Error("expected error when multiple branch names provided")
	}

	err = cmd.Args(cmd, []string{"branch1"})
	if err != nil {
		t.Errorf("expected no error with one branch name, got: %v", err)
	}
}

// TestCheckoutCommand_NoConfigFile tests error when config doesn't exist
func TestCheckoutCommand_NoConfigFile(t *testing.T) {
	// Create temp directory without branchd.json
	tempDir, err := os.MkdirTemp("", "branchd-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(originalDir)

	// Test that runCheckout fails without config
	err = runCheckout("test-branch")
	if err == nil {
		t.Error("expected error when config file is missing, got nil")
	}

	// Should contain "failed to load config"
	if err != nil && !strings.Contains(err.Error(), "failed to load config") {
		t.Errorf("expected error to contain 'failed to load config', got '%s'", err.Error())
	}
}

// TestCheckoutCommand_EmptyServerIP tests error with empty server IP
func TestCheckoutCommand_EmptyServerIP(t *testing.T) {
	// Setup test environment with empty server IP
	servers := []config.Server{
		{Alias: "test-server", IP: ""},
	}
	_, cleanup := setupTestEnvironment(t, servers)
	defer cleanup()

	// Test that runCheckout fails with empty server IP
	err := runCheckout("test-branch")
	if err == nil {
		t.Error("expected error when server IP is empty, got nil")
	}

	expectedError := "server IP is empty. Please edit branchd.json and add a valid IP address"
	if err != nil && err.Error() != expectedError {
		t.Errorf("expected error '%s', got '%s'", expectedError, err.Error())
	}
}

// captureOutput captures stdout during function execution
func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}
