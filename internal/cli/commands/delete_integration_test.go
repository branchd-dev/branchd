package commands

import (
	"fmt"
	"testing"

	"github.com/branchd-dev/branchd/internal/cli/client"
	"github.com/branchd-dev/branchd/internal/cli/config"
)

// TestDeleteIntegration_SuccessfulDeletion tests successful branch deletion
func TestDeleteIntegration_SuccessfulDeletion(t *testing.T) {
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockDeleteClient{
		branches: []client.Branch{
			{ID: "branch-1", Name: "feature-auth", CreatedBy: "user@example.com"},
			{ID: "branch-2", Name: "feature-api", CreatedBy: "user@example.com"},
			{ID: "branch-3", Name: "bug-fix", CreatedBy: "user@example.com"},
		},
	}

	// Delete existing branch
	err := runDelete(
		"feature-api",
		WithDeleteClient(mockAPI),
		WithDeleteServer(server),
	)

	if err != nil {
		t.Fatalf("expected successful deletion, got error: %v", err)
	}

	// Verify the correct branch was deleted
	if mockAPI.deletedBranch != "feature-api" {
		t.Errorf("expected to delete 'feature-api', but deleted '%s'", mockAPI.deletedBranch)
	}
}

// TestDeleteIntegration_BranchNotFound tests deletion of non-existent branch
func TestDeleteIntegration_BranchNotFound(t *testing.T) {
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockDeleteClient{
		branches: []client.Branch{
			{ID: "branch-1", Name: "main"},
			{ID: "branch-2", Name: "develop"},
		},
	}

	// Try to delete non-existent branch
	err := runDelete(
		"non-existent",
		WithDeleteClient(mockAPI),
		WithDeleteServer(server),
	)

	if err == nil {
		t.Fatal("expected error when branch doesn't exist, got nil")
	}

	expectedError := "branch 'non-existent' not found"
	if err.Error() != expectedError {
		t.Errorf("expected error '%s', got '%s'", expectedError, err.Error())
	}

	// Verify nothing was deleted
	if mockAPI.deletedBranch != "" {
		t.Errorf("expected no branch to be deleted, but '%s' was deleted", mockAPI.deletedBranch)
	}
}

// TestDeleteIntegration_NoBranches tests deletion when no branches exist
func TestDeleteIntegration_NoBranches(t *testing.T) {
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockDeleteClient{
		branches: []client.Branch{}, // Empty list
	}

	err := runDelete(
		"any-branch",
		WithDeleteClient(mockAPI),
		WithDeleteServer(server),
	)

	if err == nil {
		t.Fatal("expected error when no branches exist, got nil")
	}

	expectedError := "branch 'any-branch' not found"
	if err.Error() != expectedError {
		t.Errorf("expected error '%s', got '%s'", expectedError, err.Error())
	}
}

// TestDeleteIntegration_ListBranchesFailure tests handling of list API failure
func TestDeleteIntegration_ListBranchesFailure(t *testing.T) {
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockDeleteClient{
		listError: fmt.Errorf("failed to list branches (status 500): internal server error"),
	}

	err := runDelete(
		"any-branch",
		WithDeleteClient(mockAPI),
		WithDeleteServer(server),
	)

	if err == nil {
		t.Fatal("expected error when list API fails, got nil")
	}

	expectedError := "failed to list branches: failed to list branches (status 500): internal server error"
	if err.Error() != expectedError {
		t.Errorf("expected error '%s', got '%s'", expectedError, err.Error())
	}

	// Verify nothing was deleted
	if mockAPI.deletedBranch != "" {
		t.Errorf("expected no branch to be deleted after list failure, but '%s' was deleted", mockAPI.deletedBranch)
	}
}

// TestDeleteIntegration_DeleteAPIFailure tests handling of delete API failure
func TestDeleteIntegration_DeleteAPIFailure(t *testing.T) {
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockDeleteClient{
		branches: []client.Branch{
			{ID: "branch-1", Name: "main"},
		},
		deleteError: fmt.Errorf("failed to delete branch (status 403): permission denied"),
	}

	err := runDelete(
		"main",
		WithDeleteClient(mockAPI),
		WithDeleteServer(server),
	)

	if err == nil {
		t.Fatal("expected error when delete API fails, got nil")
	}

	expectedError := "failed to delete branch (status 403): permission denied"
	if err.Error() != expectedError {
		t.Errorf("expected error '%s', got '%s'", expectedError, err.Error())
	}
}

// TestDeleteIntegration_CaseSensitive tests that branch name matching is case-sensitive
func TestDeleteIntegration_CaseSensitive(t *testing.T) {
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockDeleteClient{
		branches: []client.Branch{
			{ID: "branch-1", Name: "Main"},
		},
	}

	// Try to delete with different case
	err := runDelete(
		"main",
		WithDeleteClient(mockAPI),
		WithDeleteServer(server),
	)

	// Should not find "main" when "Main" exists (case-sensitive)
	if err == nil {
		t.Fatal("expected error due to case-sensitive matching, got nil")
	}

	expectedError := "branch 'main' not found"
	if err.Error() != expectedError {
		t.Errorf("expected error '%s', got '%s'", expectedError, err.Error())
	}

	// Verify nothing was deleted
	if mockAPI.deletedBranch != "" {
		t.Errorf("expected no branch to be deleted, but '%s' was deleted", mockAPI.deletedBranch)
	}
}

// TestDeleteIntegration_MultipleServers tests deletion from different servers
func TestDeleteIntegration_MultipleServers(t *testing.T) {
	server1 := &config.Server{
		Alias: "production",
		IP:    "192.168.1.100",
	}

	server2 := &config.Server{
		Alias: "staging",
		IP:    "192.168.1.101",
	}

	mockAPI1 := &mockDeleteClient{
		branches: []client.Branch{
			{ID: "branch-1", Name: "prod-feature"},
		},
	}

	mockAPI2 := &mockDeleteClient{
		branches: []client.Branch{
			{ID: "branch-2", Name: "staging-feature"},
		},
	}

	// Delete from server1
	err := runDelete(
		"prod-feature",
		WithDeleteClient(mockAPI1),
		WithDeleteServer(server1),
	)
	if err != nil {
		t.Fatalf("failed to delete from server1: %v", err)
	}

	if mockAPI1.deletedBranch != "prod-feature" {
		t.Errorf("expected to delete 'prod-feature' from server1, got '%s'", mockAPI1.deletedBranch)
	}

	// Delete from server2
	err = runDelete(
		"staging-feature",
		WithDeleteClient(mockAPI2),
		WithDeleteServer(server2),
	)
	if err != nil {
		t.Fatalf("failed to delete from server2: %v", err)
	}

	if mockAPI2.deletedBranch != "staging-feature" {
		t.Errorf("expected to delete 'staging-feature' from server2, got '%s'", mockAPI2.deletedBranch)
	}
}

// TestDeleteIntegration_FirstMatchingBranch tests that first matching branch is deleted
func TestDeleteIntegration_FirstMatchingBranch(t *testing.T) {
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockDeleteClient{
		branches: []client.Branch{
			{ID: "branch-1", Name: "test"},
			{ID: "branch-2", Name: "feature"},
			{ID: "branch-3", Name: "test"}, // Duplicate name (shouldn't happen in real system)
		},
	}

	err := runDelete(
		"test",
		WithDeleteClient(mockAPI),
		WithDeleteServer(server),
	)

	if err != nil {
		t.Fatalf("expected successful deletion, got error: %v", err)
	}

	// Should delete the first matching branch
	if mockAPI.deletedBranch != "test" {
		t.Errorf("expected to delete 'test', got '%s'", mockAPI.deletedBranch)
	}
}

// TestDeleteIntegration_SpecialCharactersInName tests branch names with special characters
func TestDeleteIntegration_SpecialCharactersInName(t *testing.T) {
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	specialNames := []string{
		"feature/auth-123",
		"bug-fix_2024",
		"release-1.0.0",
		"test@branch",
	}

	for _, branchName := range specialNames {
		mockAPI := &mockDeleteClient{
			branches: []client.Branch{
				{ID: "branch-1", Name: branchName},
			},
		}

		err := runDelete(
			branchName,
			WithDeleteClient(mockAPI),
			WithDeleteServer(server),
		)

		if err != nil {
			t.Errorf("failed to delete branch '%s': %v", branchName, err)
		}

		if mockAPI.deletedBranch != branchName {
			t.Errorf("expected to delete '%s', got '%s'", branchName, mockAPI.deletedBranch)
		}
	}
}
