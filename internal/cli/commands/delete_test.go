package commands

import (
	"os"
	"testing"

	"github.com/branchd-dev/branchd/internal/cli/client"
	"github.com/branchd-dev/branchd/internal/cli/config"
)

// mockDeleteClient simulates the API client for delete testing
type mockDeleteClient struct {
	branches      []client.Branch
	listError     error
	deleteError   error
	deletedBranch string // Track which branch was deleted
}

func (m *mockDeleteClient) ListBranches(serverIP string) ([]client.Branch, error) {
	if m.listError != nil {
		return nil, m.listError
	}
	return m.branches, nil
}

func (m *mockDeleteClient) DeleteBranch(serverIP, branchID string) error {
	if m.deleteError != nil {
		return m.deleteError
	}
	// Track which branch was deleted
	for _, branch := range m.branches {
		if branch.ID == branchID {
			m.deletedBranch = branch.Name
			break
		}
	}
	return nil
}

// TestDeleteCommand_CommandStructure tests the command structure
func TestDeleteCommand_CommandStructure(t *testing.T) {
	cmd := NewDeleteCmd()

	if cmd.Use != "delete <branch-name>" {
		t.Errorf("expected Use to be 'delete <branch-name>', got %s", cmd.Use)
	}

	if cmd.Short != "Delete a database branch" {
		t.Errorf("expected Short to be 'Delete a database branch', got %s", cmd.Short)
	}

	// Test that command requires exactly 1 argument
	err := cmd.Args(cmd, []string{})
	if err == nil {
		t.Error("expected error when no arguments provided, got nil")
	}

	err = cmd.Args(cmd, []string{"branch1", "branch2"})
	if err == nil {
		t.Error("expected error when multiple arguments provided, got nil")
	}

	err = cmd.Args(cmd, []string{"branch1"})
	if err != nil {
		t.Errorf("expected no error with one argument, got %v", err)
	}
}

// TestDeleteCommand_NoConfigFile tests deletion without config file
func TestDeleteCommand_NoConfigFile(t *testing.T) {
	// Create temp directory without branchd.json
	tempDir, err := os.MkdirTemp("", "branchd-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(originalDir)

	// Test that runDelete fails without config
	err = runDelete("test-branch")
	if err == nil {
		t.Error("expected error when config file is missing, got nil")
	}

	// Should contain "failed to load config"
	if err != nil && err.Error()[:22] != "failed to load config:" {
		t.Errorf("expected error to start with 'failed to load config:', got '%s'", err.Error())
	}
}

// TestDeleteCommand_EmptyBranchName tests validation of empty branch name
func TestDeleteCommand_EmptyBranchName(t *testing.T) {
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockDeleteClient{
		branches: []client.Branch{
			{ID: "branch-1", Name: "main"},
		},
	}

	// Empty string should still go through and return "not found"
	err := runDelete(
		"",
		WithDeleteClient(mockAPI),
		WithDeleteServer(server),
	)

	if err == nil {
		t.Error("expected error when branch name is empty, got nil")
	}

	expectedError := "branch '' not found"
	if err.Error() != expectedError {
		t.Errorf("expected error '%s', got '%s'", expectedError, err.Error())
	}
}
