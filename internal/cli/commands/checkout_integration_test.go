package commands

import (
	"fmt"
	"strings"
	"testing"

	"github.com/branchd-dev/branchd/internal/cli/client"
	"github.com/branchd-dev/branchd/internal/cli/config"
)

// TestCheckoutIntegration_SuccessfulBranchCreation tests successful branch creation
func TestCheckoutIntegration_SuccessfulBranchCreation(t *testing.T) {
	// Setup
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockCheckoutClient{
		shouldFail: false,
		response: &client.CreateBranchResponse{
			ID:       "branch-123",
			User:     "branch_user",
			Password: "secret_pass",
			Host:     "192.168.1.100",
			Port:     5432,
			Database: "test_branch_db",
		},
	}

	// Capture output
	output := captureOutput(func() {
		err := runCheckout(
			"test-branch",
			WithCheckoutClient(mockAPI),
			WithCheckoutServer(server),
		)

		if err != nil {
			t.Errorf("expected successful checkout, got error: %v", err)
		}
	})

	// Verify connection string output
	expectedOutput := "postgresql://branch_user:secret_pass@192.168.1.100:5432/test_branch_db\n"
	if output != expectedOutput {
		t.Errorf("expected output:\n%s\ngot:\n%s", expectedOutput, output)
	}
}

// TestCheckoutIntegration_APIFailure tests handling of API failures
func TestCheckoutIntegration_APIFailure(t *testing.T) {
	// Setup
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	mockAPI := &mockCheckoutClient{
		shouldFail: true,
	}

	// Execute
	err := runCheckout(
		"test-branch",
		WithCheckoutClient(mockAPI),
		WithCheckoutServer(server),
	)

	// Assert failure
	if err == nil {
		t.Fatal("expected checkout to fail when API fails, but it succeeded")
	}

	expectedError := "failed to create branch (status 500): internal server error"
	if err.Error() != expectedError {
		t.Errorf("expected error '%s', got '%s'", expectedError, err.Error())
	}
}

// TestCheckoutIntegration_MultipleBranches tests creating multiple branches
func TestCheckoutIntegration_MultipleBranches(t *testing.T) {
	// Setup
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	branches := []string{"feature-auth", "feature-api", "hotfix-bug"}

	for _, branchName := range branches {
		mockAPI := &mockCheckoutClient{
			shouldFail: false,
			branchName: branchName,
			response: &client.CreateBranchResponse{
				ID:       "branch-" + branchName,
				User:     "user_" + branchName,
				Password: "pass_" + branchName,
				Host:     server.IP,
				Port:     5432,
				Database: "db_" + branchName,
			},
		}

		// Capture output
		output := captureOutput(func() {
			err := runCheckout(
				branchName,
				WithCheckoutClient(mockAPI),
				WithCheckoutServer(server),
			)

			if err != nil {
				t.Errorf("failed to create branch '%s': %v", branchName, err)
			}
		})

		// Verify output contains branch-specific info
		if !strings.Contains(output, "user_"+branchName) {
			t.Errorf("output doesn't contain expected username for branch '%s'", branchName)
		}

		if !strings.Contains(output, "db_"+branchName) {
			t.Errorf("output doesn't contain expected database for branch '%s'", branchName)
		}
	}
}

// TestCheckoutIntegration_ConnectionStringFormat tests the output format
func TestCheckoutIntegration_ConnectionStringFormat(t *testing.T) {
	// Setup
	server := &config.Server{
		Alias: "production",
		IP:    "10.0.0.50",
	}

	mockAPI := &mockCheckoutClient{
		shouldFail: false,
		response: &client.CreateBranchResponse{
			ID:       "br-001",
			User:     "pg_user",
			Password: "complex!pass@123",
			Host:     "10.0.0.50",
			Port:     5433,
			Database: "myapp_dev",
		},
	}

	// Capture output
	output := captureOutput(func() {
		err := runCheckout(
			"dev-branch",
			WithCheckoutClient(mockAPI),
			WithCheckoutServer(server),
		)

		if err != nil {
			t.Fatalf("checkout failed: %v", err)
		}
	})

	// Verify PostgreSQL connection string format
	expectedFormat := "postgresql://pg_user:complex!pass@123@10.0.0.50:5433/myapp_dev\n"
	if output != expectedFormat {
		t.Errorf("connection string format incorrect\nexpected: %s\ngot: %s", expectedFormat, output)
	}

	// Verify it starts with postgresql://
	if !strings.HasPrefix(output, "postgresql://") {
		t.Error("connection string should start with 'postgresql://'")
	}

	// Verify it contains all required components
	requiredComponents := []string{"pg_user", "complex!pass@123", "10.0.0.50", "5433", "myapp_dev"}
	for _, component := range requiredComponents {
		if !strings.Contains(output, component) {
			t.Errorf("connection string missing required component: %s", component)
		}
	}
}

// TestCheckoutIntegration_BranchNameValidation tests various branch names
func TestCheckoutIntegration_BranchNameValidation(t *testing.T) {
	// Setup
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	testCases := []struct {
		name       string
		branchName string
		shouldPass bool
	}{
		{"simple name", "main", true},
		{"with hyphens", "feature-auth", true},
		{"with underscores", "feature_auth", true},
		{"with numbers", "branch123", true},
		{"mixed", "feature-auth_v2", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockAPI := &mockCheckoutClient{
				shouldFail: false,
				branchName: tc.branchName,
				response: &client.CreateBranchResponse{
					ID:       "br-001",
					User:     "user",
					Password: "pass",
					Host:     server.IP,
					Port:     5432,
					Database: "db",
				},
			}

			err := runCheckout(
				tc.branchName,
				WithCheckoutClient(mockAPI),
				WithCheckoutServer(server),
			)

			if tc.shouldPass && err != nil {
				t.Errorf("branch name '%s' should be valid, got error: %v", tc.branchName, err)
			}
		})
	}
}

// TestCheckoutIntegration_DifferentPorts tests branches with different ports
func TestCheckoutIntegration_DifferentPorts(t *testing.T) {
	// Setup
	server := &config.Server{
		Alias: "test-server",
		IP:    "192.168.1.100",
	}

	testPorts := []int{5432, 5433, 5434, 6432}

	for _, port := range testPorts {
		mockAPI := &mockCheckoutClient{
			shouldFail: false,
			response: &client.CreateBranchResponse{
				ID:       "branch-123",
				User:     "user",
				Password: "pass",
				Host:     server.IP,
				Port:     port,
				Database: "db",
			},
		}

		output := captureOutput(func() {
			err := runCheckout(
				"test-branch",
				WithCheckoutClient(mockAPI),
				WithCheckoutServer(server),
			)

			if err != nil {
				t.Fatalf("failed with port %d: %v", port, err)
			}
		})

		// Verify port is in output
		expectedPort := fmt.Sprintf(":%d/", port)
		if !strings.Contains(output, expectedPort) {
			t.Errorf("output doesn't contain port %d, got: %s", port, output)
		}
	}
}
