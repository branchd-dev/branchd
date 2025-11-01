package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/branchd-dev/branchd/internal/cli/client"
	"github.com/branchd-dev/branchd/internal/cli/config"
)

// TestListIntegration_SingleBranch tests listing a single branch
func TestListIntegration_SingleBranch(t *testing.T) {
	// Setup
	server := &config.Server{
		Alias: "production",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockListClient{
		branches: []client.Branch{
			{
				ID:            "branch-1",
				Name:          "feature-x",
				CreatedAt:     "2025-11-01 14:30:00",
				CreatedBy:     "alice@example.com",
				RestoreID:     "restore-1",
				RestoreName:   "restore_20251101143000",
				Port:          5432,
				ConnectionURL: "postgresql://...",
			},
		},
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

	outputStr := output.String()

	// Verify header
	if !strings.Contains(outputStr, "Branches on production (192.168.1.100)") {
		t.Errorf("expected server header, got: %s", outputStr)
	}

	// Verify table headers
	if !strings.Contains(outputStr, "NAME") {
		t.Errorf("expected NAME column header, got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "CREATED BY") {
		t.Errorf("expected CREATED BY column header, got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "CREATED AT") {
		t.Errorf("expected CREATED AT column header, got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "RESTORE") {
		t.Errorf("expected RESTORE column header, got: %s", outputStr)
	}

	// Verify branch data
	if !strings.Contains(outputStr, "feature-x") {
		t.Errorf("expected branch name 'feature-x', got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "alice@example.com") {
		t.Errorf("expected creator 'alice@example.com', got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "2025-11-01 14:30:00") {
		t.Errorf("expected creation time, got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "restore_20251101143000") {
		t.Errorf("expected restore name, got: %s", outputStr)
	}
}

// TestListIntegration_MultipleBranches tests listing multiple branches
func TestListIntegration_MultipleBranches(t *testing.T) {
	// Setup
	server := &config.Server{
		Alias: "staging",
		IP:    "192.168.1.101",
	}

	mockAPI := &mockListClient{
		branches: []client.Branch{
			{
				Name:        "main",
				CreatedBy:   "alice@example.com",
				CreatedAt:   "2025-11-01 10:00:00",
				RestoreName: "restore_20251101100000",
			},
			{
				Name:        "feature-a",
				CreatedBy:   "bob@example.com",
				CreatedAt:   "2025-11-01 11:00:00",
				RestoreName: "restore_20251101110000",
			},
			{
				Name:        "feature-b",
				CreatedBy:   "charlie@example.com",
				CreatedAt:   "2025-11-01 12:00:00",
				RestoreName: "restore_20251101120000",
			},
		},
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

	outputStr := output.String()

	// Verify all branches are listed
	if !strings.Contains(outputStr, "main") {
		t.Errorf("expected 'main' branch, got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "feature-a") {
		t.Errorf("expected 'feature-a' branch, got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "feature-b") {
		t.Errorf("expected 'feature-b' branch, got: %s", outputStr)
	}

	// Verify all creators are listed
	if !strings.Contains(outputStr, "alice@example.com") {
		t.Errorf("expected alice, got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "bob@example.com") {
		t.Errorf("expected bob, got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "charlie@example.com") {
		t.Errorf("expected charlie, got: %s", outputStr)
	}
}

// TestListIntegration_EmptyList tests the empty branch list
func TestListIntegration_EmptyList(t *testing.T) {
	// Setup
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockListClient{
		branches:   []client.Branch{},
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

	outputStr := output.String()

	// Should not show table headers when no branches
	if strings.Contains(outputStr, "NAME\tCREATED BY") {
		t.Errorf("expected no table when list is empty, got: %s", outputStr)
	}

	// Should show helpful message
	if !strings.Contains(outputStr, "No branches found") {
		t.Errorf("expected 'No branches found', got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "branchd checkout <branch-name>") {
		t.Errorf("expected help text, got: %s", outputStr)
	}
}

// TestListIntegration_AuthenticationError tests authentication failure
func TestListIntegration_AuthenticationError(t *testing.T) {
	// Setup
	server := &config.Server{
		Alias: "production",
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
		t.Fatal("expected authentication error, got success")
	}

	// Verify error message
	if !strings.Contains(err.Error(), "not authenticated") {
		t.Errorf("expected authentication error, got: %s", err.Error())
	}

	// Should not output anything on error
	if output.Len() > 0 {
		t.Errorf("expected no output on error, got: %s", output.String())
	}
}

// TestListIntegration_NetworkError tests network failure handling
func TestListIntegration_NetworkError(t *testing.T) {
	// Setup
	server := &config.Server{
		Alias: "production",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockListClient{
		shouldFail: true,
		errorMsg:   "failed to send request: dial tcp 192.168.1.100:443: connection refused",
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
		t.Fatal("expected network error, got success")
	}

	// Verify error message contains network details
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("expected connection error, got: %s", err.Error())
	}
}

// TestListIntegration_DifferentServers tests listing branches from different servers
func TestListIntegration_DifferentServers(t *testing.T) {
	// Server 1
	server1 := &config.Server{
		Alias: "production",
		IP:    "192.168.1.100",
	}

	mockAPI1 := &mockListClient{
		branches: []client.Branch{
			{Name: "prod-branch", CreatedBy: "alice@example.com", CreatedAt: "2025-11-01", RestoreName: "restore-1"},
		},
	}

	var output1 bytes.Buffer
	err := runList(
		WithListClient(mockAPI1),
		WithListServer(server1),
		WithListOutput(&output1),
	)
	if err != nil {
		t.Fatalf("server1 failed: %v", err)
	}

	// Verify server1 output
	if !strings.Contains(output1.String(), "production (192.168.1.100)") {
		t.Errorf("expected production server header, got: %s", output1.String())
	}
	if !strings.Contains(output1.String(), "prod-branch") {
		t.Errorf("expected prod-branch, got: %s", output1.String())
	}

	// Server 2
	server2 := &config.Server{
		Alias: "staging",
		IP:    "192.168.1.101",
	}

	mockAPI2 := &mockListClient{
		branches: []client.Branch{
			{Name: "staging-branch", CreatedBy: "bob@example.com", CreatedAt: "2025-11-01", RestoreName: "restore-2"},
		},
	}

	var output2 bytes.Buffer
	err = runList(
		WithListClient(mockAPI2),
		WithListServer(server2),
		WithListOutput(&output2),
	)
	if err != nil {
		t.Fatalf("server2 failed: %v", err)
	}

	// Verify server2 output
	if !strings.Contains(output2.String(), "staging (192.168.1.101)") {
		t.Errorf("expected staging server header, got: %s", output2.String())
	}
	if !strings.Contains(output2.String(), "staging-branch") {
		t.Errorf("expected staging-branch, got: %s", output2.String())
	}

	// Verify isolation (server1 output shouldn't contain server2 data)
	if strings.Contains(output1.String(), "staging-branch") {
		t.Error("server1 output should not contain staging-branch")
	}
	if strings.Contains(output2.String(), "prod-branch") {
		t.Error("server2 output should not contain prod-branch")
	}
}
