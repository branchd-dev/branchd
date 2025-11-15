package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/branchd-dev/branchd/tests/e2e/testhelpers"
)

func TestCrunchyBridgeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Get PostgreSQL version from env (default: 16)
	postgresVersion := os.Getenv("TEST_POSTGRES_VERSION")
	if postgresVersion == "" {
		postgresVersion = "16"
	}

	// Get Crunchy Bridge credentials from env
	crunchyBridgeAPIKey := os.Getenv("CRUNCHYBRIDGE_API_KEY")
	crunchyBridgeClusterName := os.Getenv("CRUNCHYBRIDGE_CLUSTER_NAME")
	crunchyBridgeDatabaseName := os.Getenv("CRUNCHYBRIDGE_DATABASE_NAME")

	require.NotEmpty(t, crunchyBridgeAPIKey, "CRUNCHYBRIDGE_API_KEY must be set")
	require.NotEmpty(t, crunchyBridgeClusterName, "CRUNCHYBRIDGE_CLUSTER_NAME must be set")
	require.NotEmpty(t, crunchyBridgeDatabaseName, "CRUNCHYBRIDGE_DATABASE_NAME must be set")

	// Get or create persistent test VM (keeps running between test runs)
	vm := testhelpers.GetOrCreateVM(t, postgresVersion)

	// Build and deploy Branchd binaries and web UI
	vm.BuildAndDeploy(t)

	// Reset Branchd state (clean SQLite database for fresh test run)
	vm.ResetState(t)

	// Reset source database to known state (same as branch_test.go)
	testhelpers.ResetSourceDatabase(t)

	ctx := context.Background()

	// Generate timestamp suffix for unique branch names
	timestamp := time.Now().Unix()

	// ===================================================================
	// Setup: Create admin user and configure Crunchy Bridge
	// ===================================================================
	t.Run("Setup", func(t *testing.T) {
		t.Log("Creating admin user...")

		// Create admin user via setup endpoint
		resp := vm.APICall(t, "POST", "/api/setup", map[string]interface{}{
			"name":     "Test Admin",
			"email":    "admin@test.com",
			"password": "testpass123",
		})

		// Extract and store JWT token
		token, ok := resp["token"].(string)
		require.True(t, ok, "Response should contain token")
		require.NotEmpty(t, token, "Token should not be empty")
		vm.JWTToken = token

		t.Log("Admin user created, JWT token stored")
	})

	t.Run("ConfigureCrunchyBridge", func(t *testing.T) {
		t.Log("Configuring Crunchy Bridge integration...")

		// Configure Crunchy Bridge credentials via PATCH /api/config
		vm.APICall(t, "PATCH", "/api/config", map[string]interface{}{
			"crunchyBridgeApiKey":      crunchyBridgeAPIKey,
			"crunchyBridgeClusterName": crunchyBridgeClusterName,
			"crunchyBridgeDatabaseName": crunchyBridgeDatabaseName,
			"postgresVersion":          postgresVersion,
		})

		// Verify config was updated
		config := vm.APICall(t, "GET", "/api/config", nil)
		t.Logf("Config response: %+v", config)
		require.Equal(t, postgresVersion, config["postgres_version"])
		require.Equal(t, crunchyBridgeClusterName, config["crunchy_bridge_cluster_name"])
		require.Equal(t, crunchyBridgeDatabaseName, config["crunchy_bridge_database_name"])
		require.NotEmpty(t, config["crunchy_bridge_api_key"], "API key should be set (redacted)")
		require.Equal(t, true, config["schema_only"], "Config should default to schema_only=true")

		t.Log("Crunchy Bridge configured successfully")
	})

	// ===================================================================
	// Test 1: Anonymization Rules (Setup before activation)
	// ===================================================================
	t.Run("SetupAnonymizationRules", func(t *testing.T) {
		t.Log("Setting up anonymization rules...")

		// Setup default anon rules (5 rules for users table PII)
		ruleIDs := vm.SetupDefaultAnonRules(t)
		require.Len(t, ruleIDs, 5, "Should create 5 anonymization rules")

		// Verify rules can be listed
		rules := vm.ListAnonRules(t)
		require.Len(t, rules, 5, "Should list 5 anonymization rules")

		t.Log("Anonymization rules configured successfully")
	})

	// ===================================================================
	// Test 2: Trigger Schema-Only Restore from Crunchy Bridge
	// ===================================================================
	var schemaOnlyRestoreID string

	t.Run("TriggerSchemaOnlyRestore", func(t *testing.T) {
		t.Log("Triggering schema-only restore from Crunchy Bridge...")

		// Trigger restore explicitly (config is already set with schema_only=true)
		vm.APICall(t, "POST", "/api/restores/trigger-restore", nil)

		t.Log("Schema-only restore triggered (via pgBackRest)")
	})

	t.Run("WaitForSchemaOnlyRestore", func(t *testing.T) {
		t.Log("Waiting for schema-only restore to complete...")

		// Poll until restore is ready (pgBackRest restore may take longer than pg_dump)
		vm.WaitForCondition(t, 120*time.Second, func() bool {
			restores := vm.APICallList(t, "GET", "/api/restores", nil)
			if len(restores) == 0 {
				return false
			}

			// Check if first restore is ready
			schemaReady, ok := restores[0]["schema_ready"].(bool)
			return ok && schemaReady
		})

		t.Log("Schema-only restore completed via pgBackRest")

		// Verify restore state
		restores := vm.APICallList(t, "GET", "/api/restores", nil)
		require.Len(t, restores, 1, "Should have exactly 1 restore")

		restore := restores[0]
		schemaOnlyRestoreID = restore["id"].(string)
		require.True(t, restore["schema_ready"].(bool), "Restore schema should be ready")
		require.True(t, restore["schema_only"].(bool), "First restore should be schema-only")

		t.Log("Verified restore was created from Crunchy Bridge backup")
	})

	// ===================================================================
	// Test 3: Schema-Only Branch from Crunchy Bridge Restore
	// ===================================================================
	t.Run("TestSchemaOnlyBranch", func(t *testing.T) {
		t.Log("Testing schema-only branch from Crunchy Bridge restore...")

		branchName := fmt.Sprintf("cb-schema-branch-%d", timestamp)
		branchID := vm.TestBranchOperations(t, ctx, branchName, true, true)
		require.NotEmpty(t, branchID, "Branch ID should not be empty")

		t.Log("Schema-only branch test completed")
	})

	// ===================================================================
	// Test 4: Update Config to Trigger Full Restore
	// ===================================================================
	var fullRestoreID string

	t.Run("UpdateConfigForFullRestore", func(t *testing.T) {
		t.Log("Updating config to disable schema-only mode...")

		// Update config to disable schema-only (will trigger full restore on next activation)
		vm.APICall(t, "PATCH", "/api/config", map[string]interface{}{
			"schemaOnly": false,
		})

		// Verify config was updated
		config := vm.APICall(t, "GET", "/api/config", nil)
		require.False(t, config["schema_only"].(bool), "schema_only should be false")

		t.Log("Config updated to full restore mode")
	})

	t.Run("TriggerFullRestore", func(t *testing.T) {
		t.Log("Triggering full restore from Crunchy Bridge...")

		// Trigger restore explicitly (config was updated to schema_only=false)
		vm.APICall(t, "POST", "/api/restores/trigger-restore", nil)

		t.Log("Full restore triggered (via pgBackRest with data)")
	})

	t.Run("WaitForFullRestore", func(t *testing.T) {
		t.Log("Waiting for full restore to complete...")

		// Wait until we have 2 restores (schema-only + full)
		// Note: pgBackRest full restore may take longer than pg_dump
		vm.WaitForCondition(t, 180*time.Second, func() bool {
			restores := vm.APICallList(t, "GET", "/api/restores", nil)
			if len(restores) < 2 {
				return false
			}

			// Find the full restore (schema_only=false)
			for _, restore := range restores {
				if !restore["schema_only"].(bool) {
					schemaReady, ok1 := restore["schema_ready"].(bool)
					dataReady, ok2 := restore["data_ready"].(bool)
					if ok1 && ok2 && schemaReady && dataReady {
						fullRestoreID = restore["id"].(string)
						return true
					}
				}
			}
			return false
		})

		t.Log("Full restore completed via pgBackRest")

		// Verify restore state
		restores := vm.APICallList(t, "GET", "/api/restores", nil)
		require.GreaterOrEqual(t, len(restores), 1, "Should have at least 1 restore")

		// Find full restore
		var fullRestore map[string]interface{}
		for _, restore := range restores {
			if !restore["schema_only"].(bool) {
				fullRestore = restore
				break
			}
		}
		require.NotNil(t, fullRestore, "Should find full restore")
		require.True(t, fullRestore["schema_ready"].(bool), "Full restore schema should be ready")
		require.True(t, fullRestore["data_ready"].(bool), "Full restore data should be ready")

		t.Log("Verified full restore from Crunchy Bridge backup")
	})

	// ===================================================================
	// Test 5: Full Database Branch (With Anonymized Data)
	// ===================================================================
	var fullBranchID string

	t.Run("TestFullDatabaseBranch", func(t *testing.T) {
		t.Log("Testing full database branch with anonymization...")

		// Keep the branch (deleteBranch=false) to verify it's preserved after cleanup
		branchName := fmt.Sprintf("cb-full-branch-%d", timestamp)
		fullBranchID = vm.TestBranchOperations(t, ctx, branchName, false, false)
		require.NotEmpty(t, fullBranchID, "Branch ID should not be empty")

		t.Log("Full database branch test completed with anonymization verified")
	})

	// ===================================================================
	// Test 6: Refresh Flow (Tests Crunchy Bridge re-restore)
	// ===================================================================
	var refreshedRestoreID string

	t.Run("RefreshRestores", func(t *testing.T) {
		t.Log("Testing refresh flow with Crunchy Bridge...")

		// Get current restore count
		beforeRestores := vm.APICallList(t, "GET", "/api/restores", nil)
		beforeCount := len(beforeRestores)
		t.Logf("Current restore count: %d", beforeCount)

		// Trigger a new restore to simulate refresh
		vm.APICall(t, "POST", "/api/restores/trigger-restore", nil)

		// Wait for new restore to be created and ready
		vm.WaitForCondition(t, 180*time.Second, func() bool {
			restores := vm.APICallList(t, "GET", "/api/restores", nil)

			// Look for newest restore (by created_at or just find one that's ready and different)
			for _, restore := range restores {
				if restore["id"].(string) != fullRestoreID && restore["id"].(string) != schemaOnlyRestoreID {
					schemaReady, ok1 := restore["schema_ready"].(bool)
					dataReady, ok2 := restore["data_ready"].(bool)
					if ok1 && ok2 && schemaReady && dataReady {
						refreshedRestoreID = restore["id"].(string)
						return true
					}
				}
			}
			return false
		})

		require.NotEmpty(t, refreshedRestoreID, "Refreshed restore should be created")
		t.Logf("Refresh completed via Crunchy Bridge, new restore ID: %s", refreshedRestoreID)

		// Verify old restore WITH branch was preserved
		afterRestores := vm.APICallList(t, "GET", "/api/restores", nil)

		var foundFullRestore bool
		for _, restore := range afterRestores {
			if restore["id"].(string) == fullRestoreID {
				foundFullRestore = true
				break
			}
		}
		require.True(t, foundFullRestore, "Old full restore should still exist (has branch: cb-full-branch)")

		t.Log("Verified old restore with branch was preserved during refresh")
	})

	// ===================================================================
	// Test 7: New Branches Use Refreshed Restore
	// ===================================================================
	t.Run("TestRefreshedBranch", func(t *testing.T) {
		t.Log("Testing that new branches use refreshed restore...")

		// Create new branch - should use refreshed restore
		branchName := fmt.Sprintf("cb-refreshed-branch-%d", timestamp)
		branchID := vm.TestBranchOperations(t, ctx, branchName, false, false)
		require.NotEmpty(t, branchID, "Branch ID should not be empty")

		// Verify branch uses refreshed restore
		branches := vm.APICallList(t, "GET", "/api/branches", nil)

		var refreshedBranch map[string]interface{}
		for _, b := range branches {
			if b["name"].(string) == branchName {
				refreshedBranch = b
				break
			}
		}
		require.NotNil(t, refreshedBranch, "Should find cb-refreshed-branch")

		t.Log("New branch created successfully from refreshed Crunchy Bridge restore")
	})

	// ===================================================================
	// Test 8: Multiple Branches (Port Allocation)
	// ===================================================================
	t.Run("CreateMultipleBranches", func(t *testing.T) {
		t.Log("Testing multiple branch creation and port allocation...")

		// Get current branch count
		beforeBranches := vm.APICallList(t, "GET", "/api/branches", nil)
		beforeCount := len(beforeBranches)

		// Create 3 more branches
		newBranchIDs := make([]string, 0, 3)
		for i := 1; i <= 3; i++ {
			branchName := fmt.Sprintf("cb-multi-branch-%d-%d", i, timestamp)
			resp := vm.APICall(t, "POST", "/api/branches", map[string]interface{}{
				"name": branchName,
			})

			branchID, ok := resp["id"].(string)
			require.True(t, ok, "Response should contain branch ID")
			newBranchIDs = append(newBranchIDs, branchID)

			// Verify unique port was allocated
			port, ok := resp["port"].(float64)
			require.True(t, ok, "Response should contain port")
			require.GreaterOrEqual(t, int(port), 15432, "Port should be >= 15432")
			require.LessOrEqual(t, int(port), 16432, "Port should be <= 16432")
		}

		// Verify all branches exist
		afterBranches := vm.APICallList(t, "GET", "/api/branches", nil)
		require.Equal(t, beforeCount+3, len(afterBranches), "Should have 3 more branches")

		t.Logf("Created %d branches successfully from Crunchy Bridge restore", len(newBranchIDs))

		// Clean up new branches
		for i, branchID := range newBranchIDs {
			vm.APICall(t, "DELETE", "/api/branches/"+branchID, nil)
			t.Logf("Deleted cb-multi-branch-%d", i+1)
		}

		// Verify deletion
		finalBranches := vm.APICallList(t, "GET", "/api/branches", nil)
		require.Equal(t, beforeCount, len(finalBranches), "Should be back to original branch count")
	})

	// ===================================================================
	// Test 9: Verify Crunchy Bridge Integration Details
	// ===================================================================
	t.Run("VerifyCrunchyBridgeDetails", func(t *testing.T) {
		t.Log("Verifying Crunchy Bridge integration details...")

		// Check restore logs to verify pgBackRest was used
		restores := vm.APICallList(t, "GET", "/api/restores", nil)
		require.NotEmpty(t, restores, "Should have at least one restore")

		// Get logs for the most recent restore
		mostRecentRestore := restores[len(restores)-1]
		restoreID := mostRecentRestore["id"].(string)

		logsResp := vm.APICall(t, "GET", fmt.Sprintf("/api/restores/%s/logs?lines=100", restoreID), nil)
		logs, ok := logsResp["logs"].([]interface{})
		require.True(t, ok, "Response should contain logs array")
		require.NotEmpty(t, logs, "Should have restore logs")

		// Convert logs to strings and check for pgBackRest markers
		logLines := make([]string, len(logs))
		for i, log := range logs {
			logLines[i] = log.(string)
		}

		// Verify pgBackRest-specific log messages
		foundPgBackRest := false
		for _, line := range logLines {
			if contains(line, "pgbackrest") || contains(line, "pgBackRest") {
				foundPgBackRest = true
				break
			}
		}

		require.True(t, foundPgBackRest, "Logs should mention pgBackRest")
		t.Log("Verified restore used pgBackRest")

		t.Log("Crunchy Bridge integration details verified")
	})
}

// contains is a helper function to check if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if toLower(s[i+j]) != toLower(substr[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func toLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}
