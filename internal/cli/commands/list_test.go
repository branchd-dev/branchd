package commands

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/branchd-dev/branchd/internal/cli/client"
	"github.com/branchd-dev/branchd/internal/cli/config"
)

// mockListClient simulates the API client for listing branches
type mockListClient struct {
	branches   []client.Branch
	shouldFail bool
	errorMsg   string
}

func (m *mockListClient) ListBranches(serverIP string) ([]client.Branch, error) {
	if m.shouldFail {
		return nil, errors.New(m.errorMsg)
	}
	return m.branches, nil
}

// TestListCommand_NoBranches tests the empty branch list scenario
func TestListCommand_NoBranches(t *testing.T) {
	// Setup
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockListClient{
		branches:   []client.Branch{}, // Empty list
		shouldFail: false,
	}

	var output bytes.Buffer

	// Execute
	err := runList(
		WithListClient(mockAPI),
		WithListServer(server),
		WithListOutput(&output),
	)

	// Assert success
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	// Verify output contains "No branches found" message
	outputStr := output.String()
	if !strings.Contains(outputStr, "No branches found") {
		t.Errorf("expected 'No branches found' message, got: %s", outputStr)
	}

	// Verify helpful message is shown
	if !strings.Contains(outputStr, "branchd checkout") {
		t.Errorf("expected helpful message about creating branches, got: %s", outputStr)
	}
}

// TestListCommand_NoConfigFile tests error when config is missing
func TestListCommand_NoConfigFile(t *testing.T) {
	// Create temp directory without branchd.json
	tempDir := t.TempDir()

	originalDir := mustGetwd(t)
	mustChdir(t, tempDir)
	defer mustChdir(t, originalDir)

	// Execute
	err := runList()

	// Assert failure
	if err == nil {
		t.Fatal("expected error when config file is missing, got nil")
	}

	// Should contain "failed to load config"
	if !strings.Contains(err.Error(), "failed to load config") {
		t.Errorf("expected error about missing config, got: %s", err.Error())
	}
}

// TestListCommand_APIFailure tests handling of API failures
func TestListCommand_APIFailure(t *testing.T) {
	// Setup
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockListClient{
		shouldFail: true,
		errorMsg:   "not authenticated. Please run 'branchd login' first",
	}

	var output bytes.Buffer

	// Execute
	err := runList(
		WithListClient(mockAPI),
		WithListServer(server),
		WithListOutput(&output),
	)

	// Assert failure
	if err == nil {
		t.Fatal("expected error when API fails, but got success")
	}

	// Verify error message
	if !strings.Contains(err.Error(), "not authenticated") {
		t.Errorf("expected authentication error, got: %s", err.Error())
	}

	// Verify no output was written
	if output.Len() > 0 {
		t.Errorf("expected no output on error, got: %s", output.String())
	}
}

// Helper functions
func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	return wd
}

func mustChdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
}
