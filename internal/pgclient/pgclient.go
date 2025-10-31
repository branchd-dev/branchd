package pgclient

import (
	"context"
	"database/sql"
	"fmt"
	"os/exec"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// Client wraps a PostgreSQL connection
type Client struct {
	db               *sql.DB
	connectionString string
}

// NewClient creates a new PostgreSQL client
func NewClient(connectionString string) (*Client, error) {
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}
	return &Client{db: db, connectionString: connectionString}, nil
}

// Close closes the database connection
func (c *Client) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// Ping tests the database connection
func (c *Client) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// GetVersion retrieves the PostgreSQL version string
func (c *Client) GetVersion(ctx context.Context) (string, error) {
	var version string
	query := "SHOW server_version"
	if err := c.db.QueryRowContext(ctx, query).Scan(&version); err != nil {
		return "", fmt.Errorf("failed to query PostgreSQL version: %w", err)
	}
	return version, nil
}

// GetDatabaseSize retrieves the database size in GB
func (c *Client) GetDatabaseSize(ctx context.Context) (float64, error) {
	var sizeBytes int64
	query := "SELECT pg_database_size(current_database())"
	if err := c.db.QueryRowContext(ctx, query).Scan(&sizeBytes); err != nil {
		return 0, fmt.Errorf("failed to query database size: %w", err)
	}
	return float64(sizeBytes) / (1024 * 1024 * 1024), nil
}

// GetSchema retrieves table and column information from the public schema
func (c *Client) GetSchema(ctx context.Context) ([]TableSchema, error) {
	query := `
		SELECT
			table_name,
			column_name
		FROM information_schema.columns
		WHERE table_schema = 'public'
		ORDER BY table_name, ordinal_position
	`

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query database schema: %w", err)
	}
	defer rows.Close()

	// Build schema map
	schemaMap := make(map[string][]string)
	for rows.Next() {
		var tableName, columnName string
		if err := rows.Scan(&tableName, &columnName); err != nil {
			return nil, fmt.Errorf("failed to scan schema row: %w", err)
		}
		schemaMap[tableName] = append(schemaMap[tableName], columnName)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating schema rows: %w", err)
	}

	// Convert map to array
	var tables []TableSchema
	for tableName, columns := range schemaMap {
		tables = append(tables, TableSchema{
			Table:   tableName,
			Columns: columns,
		})
	}

	return tables, nil
}

// DatabaseInfo contains metadata about a PostgreSQL database
type DatabaseInfo struct {
	SizeGB       float64
	MajorVersion int
}

// TableSchema represents a table and its columns
type TableSchema struct {
	Table   string   `json:"table"`
	Columns []string `json:"columns"`
}

// GetDatabaseInfo retrieves size and version information from a PostgreSQL database
func GetDatabaseInfo(ctx context.Context, connectionString string) (*DatabaseInfo, error) {
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}
	defer db.Close()

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Query database size
	var dbSizeBytes int64
	query := "SELECT pg_database_size(current_database())"
	if err := db.QueryRowContext(ctx, query).Scan(&dbSizeBytes); err != nil {
		return nil, fmt.Errorf("failed to query database size: %w", err)
	}

	// Query PostgreSQL version
	var versionString string
	versionQuery := "SHOW server_version"
	if err := db.QueryRowContext(ctx, versionQuery).Scan(&versionString); err != nil {
		return nil, fmt.Errorf("failed to query PostgreSQL version: %w", err)
	}

	// Extract major version number (e.g., "16.3" -> 16, "14.10 (Ubuntu 14.10-1.pgdg22.04+1)" -> 14)
	var majorVersion int
	if _, err := fmt.Sscanf(versionString, "%d", &majorVersion); err != nil {
		// Default to 16 if parsing fails
		majorVersion = 16
	}

	// Convert to GB
	dbSizeGB := float64(dbSizeBytes) / (1024 * 1024 * 1024)

	return &DatabaseInfo{
		SizeGB:       dbSizeGB,
		MajorVersion: majorVersion,
	}, nil
}

// GetDatabaseSchema retrieves table and column information from the public schema
func GetDatabaseSchema(ctx context.Context, connectionString string) ([]TableSchema, error) {
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}
	defer db.Close()

	// Query for tables and columns in public schema
	query := `
		SELECT
			table_name,
			column_name
		FROM information_schema.columns
		WHERE table_schema = 'public'
		ORDER BY table_name, ordinal_position
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query database schema: %w", err)
	}
	defer rows.Close()

	// Build schema map
	schemaMap := make(map[string][]string)
	for rows.Next() {
		var tableName, columnName string
		if err := rows.Scan(&tableName, &columnName); err != nil {
			return nil, fmt.Errorf("failed to scan schema row: %w", err)
		}
		schemaMap[tableName] = append(schemaMap[tableName], columnName)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating schema rows: %w", err)
	}

	// Convert map to array
	var tables []TableSchema
	for tableName, columns := range schemaMap {
		tables = append(tables, TableSchema{
			Table:   tableName,
			Columns: columns,
		})
	}

	return tables, nil
}

// ValidateConnection validates that a connection string has sufficient permissions
// by running pg_dump --schema-only
func ValidateConnection(ctx context.Context, connectionString string) error {
	// Use bash to run pg_dump with proper error handling
	cmd := exec.CommandContext(ctx, "bash", "-c",
		fmt.Sprintf(`pg_dump --schema-only --file=/dev/null "%s" 2>&1`, connectionString))

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// If command succeeded, return nil
	if err == nil && !strings.Contains(outputStr, "ERROR") && !strings.Contains(outputStr, "FATAL") {
		return nil
	}

	// Return the error output for user feedback
	if outputStr != "" {
		return fmt.Errorf("%s", outputStr)
	}

	return err
}

// TestConnection tests if a connection string is valid by pinging the database
func TestConnection(ctx context.Context, connectionString string) error {
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return fmt.Errorf("invalid connection string: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	return nil
}

// WithTimeout wraps a context with a timeout for database operations
func WithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}
