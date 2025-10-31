package testhelpers

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// ResetSourceDatabase resets the test data in the source PostgreSQL database
func ResetSourceDatabase(t *testing.T) {
	t.Helper()

	connStr := os.Getenv("TEST_CONNECTION_STRING")
	require.NotEmpty(t, connStr, "TEST_CONNECTION_STRING must be set")

	t.Log("Resetting source database...")

	db, err := sql.Open("postgres", connStr)
	require.NoError(t, err, "Failed to connect to source database")
	defer db.Close()

	// Drop and recreate users table
	_, err = db.Exec(`
		DROP TABLE IF EXISTS users CASCADE;

		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name VARCHAR(100),
			email VARCHAR(100),
			phone VARCHAR(20),
			address TEXT,
			ssn VARCHAR(11)
		);
	`)
	require.NoError(t, err, "Failed to recreate users table")

	// Insert test data
	_, err = db.Exec(`
		INSERT INTO users (name, email, phone, address, ssn) VALUES
			('Alice Johnson', 'alice.johnson@company.com', '+1-555-0101', '123 Main St, New York, NY 10001', '123-45-6789'),
			('Bob Smith', 'bob.smith@company.com', '+1-555-0102', '456 Oak Ave, Los Angeles, CA 90001', '234-56-7890'),
			('Carol Williams', 'carol.williams@company.com', '+1-555-0103', '789 Pine Rd, Chicago, IL 60601', '345-67-8901');
	`)
	require.NoError(t, err, "Failed to insert test data")

	t.Log("Source database reset complete")
}

// ConnectPostgres connects to a PostgreSQL database
func ConnectPostgres(t *testing.T, connStr string) *sql.DB {
	t.Helper()

	db, err := sql.Open("postgres", connStr)
	require.NoError(t, err, "Failed to connect to database: %s", connStr)

	err = db.Ping()
	require.NoError(t, err, "Failed to ping database: %s", connStr)

	return db
}

// QueryRow executes a query and returns a single value
func QueryRow(t *testing.T, db *sql.DB, query string, args ...interface{}) string {
	t.Helper()

	var result string
	err := db.QueryRow(query, args...).Scan(&result)
	require.NoError(t, err, "Query failed: %s", query)

	return result
}

// QueryRows executes a query and returns all rows
func QueryRows(t *testing.T, db *sql.DB, query string, args ...interface{}) []map[string]interface{} {
	t.Helper()

	rows, err := db.Query(query, args...)
	require.NoError(t, err, "Query failed: %s", query)
	defer rows.Close()

	cols, err := rows.Columns()
	require.NoError(t, err, "Failed to get columns")

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		err := rows.Scan(valuePtrs...)
		require.NoError(t, err, "Failed to scan row")

		row := make(map[string]interface{})
		for i, col := range cols {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	require.NoError(t, rows.Err(), "Error iterating rows")

	return results
}

// GetConnectionStringFromResponse extracts connection string from API response
func GetConnectionStringFromResponse(resp map[string]interface{}) string {
	user := resp["user"].(string)
	password := resp["password"].(string)
	host := resp["host"].(string)
	port := int(resp["port"].(float64))
	database := resp["database"].(string)

	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s",
		user, password, host, port, database)
}
