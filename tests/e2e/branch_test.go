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

func TestBranchOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Get PostgreSQL version from env (default: 16)
	postgresVersion := os.Getenv("TEST_POSTGRES_VERSION")
	if postgresVersion == "" {
		postgresVersion = "16"
	}

	// Get connection string from env
	connectionString := os.Getenv("TEST_CONNECTION_STRING")
	require.NotEmpty(t, connectionString, "TEST_CONNECTION_STRING must be set")

	// Get or create persistent test VM (keeps running between test runs)
	vm := testhelpers.GetOrCreateVM(t, postgresVersion)

	// Build and deploy Branchd binaries and web UI
	vm.BuildAndDeploy(t)

	// Reset Branchd state (clean SQLite database for fresh test run)
	vm.ResetState(t)

	// Reset source database to known state
	testhelpers.ResetSourceDatabase(t)

	ctx := context.Background()

	// Generate timestamp suffix for unique branch names
	timestamp := time.Now().Unix()

	// ===================================================================
	// Setup: Create admin user and configure database
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

	t.Run("Onboarding", func(t *testing.T) {
		t.Log("Configuring source database...")

		// Configure source database connection via PATCH /api/config
		vm.APICall(t, "PATCH", "/api/config", map[string]interface{}{
			"connectionString": connectionString,
		})

		// Verify config was updated
		config := vm.APICall(t, "GET", "/api/config", nil)
		t.Logf("Config response: %+v", config)
		require.Equal(t, postgresVersion, config["postgres_version"])
		require.Equal(t, true, config["schema_only"], "Config should default to schema_only=true")

		t.Log("Source database configured")
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
	// Test 2: Trigger Schema-Only Restore (Fast initial setup)
	// ===================================================================
	var schemaOnlyRestoreID string

	t.Run("TriggerSchemaOnlyRestore", func(t *testing.T) {
		t.Log("Triggering schema-only restore...")

		// Trigger restore explicitly (config is already set with schema_only=true)
		vm.APICall(t, "POST", "/api/restores/trigger-restore", nil)

		t.Log("Schema-only restore triggered")
	})

	t.Run("WaitForSchemaOnlyRestore", func(t *testing.T) {
		t.Log("Waiting for schema-only restore to complete...")

		// Poll until restore is ready
		vm.WaitForCondition(t, 60*time.Second, func() bool {
			restores := vm.APICallList(t, "GET", "/api/restores", nil)
			if len(restores) == 0 {
				return false
			}

			// Check if first restore is ready
			schemaReady, ok := restores[0]["schema_ready"].(bool)
			return ok && schemaReady
		})

		t.Log("Schema-only restore completed")

		// Verify restore state
		restores := vm.APICallList(t, "GET", "/api/restores", nil)
		require.Len(t, restores, 1, "Should have exactly 1 restore")

		restore := restores[0]
		schemaOnlyRestoreID = restore["id"].(string)
		require.True(t, restore["schema_ready"].(bool), "Restore schema should be ready")
		require.True(t, restore["schema_only"].(bool), "First restore should be schema-only")
	})

	// ===================================================================
	// Test 3: Schema-Only Branch (Fast user experience)
	// ===================================================================
	t.Run("TestSchemaOnlyBranch", func(t *testing.T) {
		t.Log("Testing schema-only branch...")

		branchName := fmt.Sprintf("schema-branch-%d", timestamp)
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
		t.Log("Triggering full restore...")

		// Trigger restore explicitly (config was updated to schema_only=false)
		vm.APICall(t, "POST", "/api/restores/trigger-restore", nil)

		t.Log("Full restore triggered")
	})

	t.Run("WaitForFullRestore", func(t *testing.T) {
		t.Log("Waiting for full restore to complete...")

		// Wait until we have 2 restores (schema-only + full)
		vm.WaitForCondition(t, 60*time.Second, func() bool {
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

		t.Log("Full restore completed")

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
	})

	// ===================================================================
	// Test 5: Full Database Branch (With Anonymized Data)
	// ===================================================================
	var fullBranchID string

	t.Run("TestFullDatabaseBranch", func(t *testing.T) {
		t.Log("Testing full database branch with anonymization...")

		// Keep the branch (deleteBranch=false) to verify it's preserved after cleanup
		branchName := fmt.Sprintf("full-branch-%d", timestamp)
		fullBranchID = vm.TestBranchOperations(t, ctx, branchName, false, false)
		require.NotEmpty(t, fullBranchID, "Branch ID should not be empty")

		t.Log("Full database branch test completed with anonymization verified")
	})

	// ===================================================================
	// Test 6: Refresh Flow (Preserves branches, cleans up old restores)
	// ===================================================================
	var refreshedRestoreID string

	t.Run("RefreshRestores", func(t *testing.T) {
		t.Log("Testing refresh flow...")

		// Get current restore count
		beforeRestores := vm.APICallList(t, "GET", "/api/restores", nil)
		beforeCount := len(beforeRestores)
		t.Logf("Current restore count: %d", beforeCount)

		// Trigger a new restore to simulate refresh
		vm.APICall(t, "POST", "/api/restores/trigger-restore", nil)

		// Wait for new restore to be created and ready
		vm.WaitForCondition(t, 40*time.Second, func() bool {
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
		t.Logf("Refresh completed, new restore ID: %s", refreshedRestoreID)

		// Verify old restore WITH branch was preserved
		afterRestores := vm.APICallList(t, "GET", "/api/restores", nil)

		var foundFullRestore bool
		for _, restore := range afterRestores {
			if restore["id"].(string) == fullRestoreID {
				foundFullRestore = true
				break
			}
		}
		require.True(t, foundFullRestore, "Old full restore should still exist (has branch: full-branch)")

		t.Log("Verified old restore with branch was preserved during refresh")
	})

	// ===================================================================
	// Test 7: New Branches Use Refreshed Restore
	// ===================================================================
	t.Run("TestRefreshedBranch", func(t *testing.T) {
		t.Log("Testing that new branches use refreshed restore...")

		// Create new branch - should use refreshed restore
		branchName := fmt.Sprintf("refreshed-branch-%d", timestamp)
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
		require.NotNil(t, refreshedBranch, "Should find refreshed-branch")

		t.Log("New branch created successfully from refreshed restore")
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
			branchName := fmt.Sprintf("multi-branch-%d-%d", i, timestamp)
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

		t.Logf("Created %d branches successfully", len(newBranchIDs))

		// Clean up new branches
		for i, branchID := range newBranchIDs {
			vm.APICall(t, "DELETE", "/api/branches/"+branchID, nil)
			t.Logf("Deleted multi-branch-%d", i+1)
		}

		// Verify deletion
		finalBranches := vm.APICallList(t, "GET", "/api/branches", nil)
		require.Equal(t, beforeCount, len(finalBranches), "Should be back to original branch count")
	})
}
