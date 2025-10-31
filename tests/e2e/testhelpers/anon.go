package testhelpers

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// CreateAnonRule creates an anonymization rule via API
func (vm *VM) CreateAnonRule(t *testing.T, table, column, template string) map[string]interface{} {
	t.Helper()

	resp := vm.APICall(t, "POST", "/api/anon-rules", map[string]interface{}{
		"table":    table,
		"column":   column,
		"template": template,
	})

	return resp
}

// DeleteAnonRule deletes an anonymization rule by ID
func (vm *VM) DeleteAnonRule(t *testing.T, ruleID string) {
	t.Helper()

	vm.APICall(t, "DELETE", "/api/anon-rules/"+ruleID, nil)
}

// ListAnonRules lists all anonymization rules
func (vm *VM) ListAnonRules(t *testing.T) []map[string]interface{} {
	t.Helper()

	return vm.APICallList(t, "GET", "/api/anon-rules", nil)
}

// SetupDefaultAnonRules creates standard anonymization rules for the test users table
// These rules anonymize all PII fields in seed.sql
func (vm *VM) SetupDefaultAnonRules(t *testing.T) []string {
	t.Helper()

	t.Log("Setting up default anonymization rules for users table...")

	rules := []struct {
		table    string
		column   string
		template string
	}{
		{"users", "name", "User ${index}"},
		{"users", "email", "user_${index}@example.com"},
		{"users", "phone", "+1555000${index}"},
		{"users", "address", "Address ${index}"},
		{"users", "ssn", "000-00-${index}"},
	}

	var ruleIDs []string
	for _, rule := range rules {
		resp := vm.CreateAnonRule(t, rule.table, rule.column, rule.template)
		ruleID, ok := resp["id"].(string)
		require.True(t, ok, "Response should contain rule ID")
		ruleIDs = append(ruleIDs, ruleID)
	}

	// Verify rules were created
	allRules := vm.ListAnonRules(t)
	require.Len(t, allRules, 5, "Should have 5 anonymization rules")

	t.Logf("Created %d anonymization rules", len(ruleIDs))

	return ruleIDs
}

// VerifyAnonymization verifies that anonymization rules were applied correctly
// It queries the database and checks that data matches expected anonymized patterns
func VerifyAnonymization(t *testing.T, db *sql.DB, expectedRows int) {
	t.Helper()

	t.Log("Verifying anonymization rules were applied...")

	// Check that names are anonymized to "User ${index}" pattern
	for i := 1; i <= expectedRows; i++ {
		expectedName := fmt.Sprintf("User %d", i)
		var actualName string
		err := db.QueryRow("SELECT name FROM users WHERE id = $1", i).Scan(&actualName)
		require.NoError(t, err, "Failed to query name for id %d", i)
		require.Equal(t, expectedName, actualName, "Name should be anonymized for id %d", i)
	}

	// Check that emails are anonymized to "user_${index}@example.com" pattern
	for i := 1; i <= expectedRows; i++ {
		expectedEmail := fmt.Sprintf("user_%d@example.com", i)
		var actualEmail string
		err := db.QueryRow("SELECT email FROM users WHERE id = $1", i).Scan(&actualEmail)
		require.NoError(t, err, "Failed to query email for id %d", i)
		require.Equal(t, expectedEmail, actualEmail, "Email should be anonymized for id %d", i)
	}

	// Check that phones are anonymized to "+1555000${index}" pattern
	for i := 1; i <= expectedRows; i++ {
		expectedPhone := fmt.Sprintf("+1555000%d", i)
		var actualPhone string
		err := db.QueryRow("SELECT phone FROM users WHERE id = $1", i).Scan(&actualPhone)
		require.NoError(t, err, "Failed to query phone for id %d", i)
		require.Equal(t, expectedPhone, actualPhone, "Phone should be anonymized for id %d", i)
	}

	// Check that addresses are anonymized to "Address ${index}" pattern
	for i := 1; i <= expectedRows; i++ {
		expectedAddress := fmt.Sprintf("Address %d", i)
		var actualAddress string
		err := db.QueryRow("SELECT address FROM users WHERE id = $1", i).Scan(&actualAddress)
		require.NoError(t, err, "Failed to query address for id %d", i)
		require.Equal(t, expectedAddress, actualAddress, "Address should be anonymized for id %d", i)
	}

	// Check that SSNs are anonymized to "000-00-${index}" pattern
	for i := 1; i <= expectedRows; i++ {
		expectedSSN := fmt.Sprintf("000-00-%d", i)
		var actualSSN string
		err := db.QueryRow("SELECT ssn FROM users WHERE id = $1", i).Scan(&actualSSN)
		require.NoError(t, err, "Failed to query SSN for id %d", i)
		require.Equal(t, expectedSSN, actualSSN, "SSN should be anonymized for id %d", i)
	}

	t.Logf("All %d rows verified for anonymization", expectedRows)
}

// TestBranchOperations is a comprehensive test helper that creates a branch and performs various SQL operations
func (vm *VM) TestBranchOperations(
	t *testing.T,
	ctx context.Context,
	branchName string,
	schemaOnly bool,
	deleteBranch bool,
) string {
	t.Helper()

	t.Logf("Creating branch: %s (schema_only=%v)", branchName, schemaOnly)

	// Create branch
	resp := vm.APICall(t, "POST", "/api/branches", map[string]interface{}{
		"name": branchName,
	})

	branchID, ok := resp["id"].(string)
	require.True(t, ok, "Response should contain branch ID")
	require.NotEmpty(t, branchID, "Branch ID should not be empty")

	// Extract connection string
	connStr := GetConnectionStringFromResponse(resp)
	require.NotEmpty(t, connStr, "Branch should return connection string")

	t.Logf("Branch created with ID: %s", branchID)

	// Connect to branch database
	db := ConnectPostgres(t, connStr)
	defer db.Close()

	// Check initial user count
	var userCount int
	err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	require.NoError(t, err, "Failed to count users")

	if schemaOnly {
		require.Equal(t, 0, userCount, "Schema-only branch should have no data initially")
		t.Log("Verified schema-only branch has empty tables")
	} else {
		require.Equal(t, 3, userCount, "Full database should have 3 users from seed data")
		t.Log("Verified full database has seed data")

		// Verify anonymization was applied
		VerifyAnonymization(t, db, 3)
		t.Log("Anonymization verified successfully")
	}

	// Insert new user
	var newUserID int
	err = db.QueryRow(`
		INSERT INTO users (name, email, phone, address, ssn)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, "New User", "new_user@example.com", "+15559990001", "Test Address", "999-99-9999").Scan(&newUserID)
	require.NoError(t, err, "Failed to insert new user")
	t.Logf("Inserted new user with ID: %d", newUserID)

	// Query back inserted user
	var insertedName string
	err = db.QueryRow("SELECT name FROM users WHERE id = $1", newUserID).Scan(&insertedName)
	require.NoError(t, err, "Failed to query inserted user")
	require.Equal(t, "New User", insertedName, "Should query back inserted user")

	// Update user
	_, err = db.Exec("UPDATE users SET name = $1 WHERE id = $2", "Updated User", newUserID)
	require.NoError(t, err, "Failed to update user")

	// Verify update
	var updatedName string
	err = db.QueryRow("SELECT name FROM users WHERE id = $1", newUserID).Scan(&updatedName)
	require.NoError(t, err, "Failed to query updated user")
	require.Equal(t, "Updated User", updatedName, "Should query back updated user")

	// Create new table
	_, err = db.Exec("CREATE TABLE test_table (id SERIAL PRIMARY KEY, data TEXT)")
	require.NoError(t, err, "Failed to create test table")

	// Insert into new table
	var testID int
	err = db.QueryRow("INSERT INTO test_table (data) VALUES ($1) RETURNING id", "test data").Scan(&testID)
	require.NoError(t, err, "Failed to insert into test table")

	// Verify final user count
	err = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	require.NoError(t, err, "Failed to count users after operations")

	expectedFinalCount := 1
	if !schemaOnly {
		expectedFinalCount = 4 // 3 seed users + 1 new user
	}
	require.Equal(t, expectedFinalCount, userCount, "Final user count should match expected")

	t.Log("All SQL operations completed successfully")

	// Delete branch if requested
	if deleteBranch {
		vm.APICall(t, "DELETE", "/api/branches/"+branchID, nil)
		t.Logf("Branch %s deleted", branchName)

		// Verify connection is closed
		err = db.Ping()
		require.Error(t, err, "Connection to deleted branch should fail")
	}

	return branchID
}
