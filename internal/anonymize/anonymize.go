package anonymize

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/branchd-dev/branchd/internal/models"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

// TablePrimaryKey holds the primary key column for a table
type TablePrimaryKey struct {
	Table    string
	PKColumn string // Empty string means no PK found, will use ctid
}

// GenerateSQL generates anonymization SQL from rules
// Uses PostgreSQL row_number() for deterministic anonymization
// primaryKeys maps table names to their primary key columns for consistent ordering
func GenerateSQL(rules []models.AnonRule, primaryKeys map[string]string) string {
	if len(rules) == 0 {
		return ""
	}

	// Group rules by table
	tableRules := make(map[string][]models.AnonRule)
	for _, rule := range rules {
		tableRules[rule.Table] = append(tableRules[rule.Table], rule)
	}

	var sqlStatements []string

	for table, rules := range tableRules {
		pkColumn := primaryKeys[table] // Empty string if not found
		sql := generateTableUpdateSQL(table, rules, pkColumn)
		sqlStatements = append(sqlStatements, sql)
	}

	return strings.Join(sqlStatements, "\n\n")
}

// generatePrimaryKeyQuerySQL generates SQL to query primary keys for all tables
func generatePrimaryKeyQuerySQL(tables []string) string {
	if len(tables) == 0 {
		return ""
	}

	// Build SQL to find primary key columns for all tables
	// Returns: table_name | column_name (one row per table with single-column PK)
	quotedTables := make([]string, len(tables))
	for i, table := range tables {
		quotedTables[i] = fmt.Sprintf("'%s'", strings.ReplaceAll(table, "'", "''"))
	}

	sql := fmt.Sprintf(`
SELECT
    t.tablename as table_name,
    a.attname as column_name
FROM pg_tables t
JOIN pg_class c ON c.relname = t.tablename
JOIN pg_index i ON i.indrelid = c.oid AND i.indisprimary
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = ANY(i.indkey)
WHERE t.schemaname = 'public'
  AND t.tablename IN (%s)
  AND array_length(i.indkey, 1) = 1  -- Only single-column primary keys
ORDER BY t.tablename;
`, strings.Join(quotedTables, ", "))

	return sql
}

// generateTableUpdateSQL generates UPDATE statement for a single table
// pkColumn is the primary key column name (empty string means use ctid)
func generateTableUpdateSQL(table string, rules []models.AnonRule, pkColumn string) string {
	if len(rules) == 0 {
		return ""
	}

	// Determine ordering: use primary key if available, otherwise ctid
	var orderBy string
	var orderByComment string
	if pkColumn != "" {
		orderBy = quoteIdentifier(pkColumn)
		orderByComment = fmt.Sprintf(" (ordered by PK: %s)", pkColumn)
	} else {
		orderBy = "ctid"
		orderByComment = " (ordered by ctid - no PK found)"
	}

	// Build SET clause with row_number replacement and IS DISTINCT FROM for idempotency
	var setClauses []string
	var whereConditions []string
	for _, rule := range rules {
		setValue := renderTemplate(rule.Template, rule.ColumnType)
		columnQuoted := quoteIdentifier(rule.Column)

		// Add SET clause
		setClauses = append(setClauses, fmt.Sprintf("%s = %s", columnQuoted, setValue))

		// Add condition to skip rows that already have the target value (idempotency)
		whereConditions = append(whereConditions, fmt.Sprintf("%s.%s IS DISTINCT FROM %s",
			quoteIdentifier(table), columnQuoted, setValue))
	}

	// Combine WHERE conditions with OR (update if ANY column is different)
	whereClause := strings.Join(whereConditions, " OR ")

	// Use CTE with row numbers for deterministic updates
	sql := fmt.Sprintf(`-- Anonymize table: %s%s
WITH numbered_rows AS (
  SELECT ctid, row_number() OVER (ORDER BY %s) as _row_num
  FROM %s
)
UPDATE %s
SET %s
FROM numbered_rows
WHERE %s.ctid = numbered_rows.ctid
  AND (%s);`,
		table,
		orderByComment,
		orderBy,
		quoteIdentifier(table),
		quoteIdentifier(table),
		strings.Join(setClauses, ",\n    "),
		quoteIdentifier(table),
		whereClause,
	)

	return sql
}

// renderTemplate converts template string to SQL expression
// Replaces ${index} with row number reference
// Handles different column types: text, integer, boolean, null
func renderTemplate(template string, columnType string) string {
	// Handle NULL type - ignore template and return SQL NULL
	if columnType == "null" {
		return "NULL"
	}

	// Handle boolean type
	if columnType == "boolean" {
		// Return unquoted true/false
		return template
	}

	// Handle integer type
	if columnType == "integer" {
		// Check if template contains ${index}
		if strings.Contains(template, "${index}") {
			// For integer columns with ${index}, we need to cast to text for concatenation,
			// then cast back to integer
			parts := strings.Split(template, "${index}")
			var sqlParts []string
			for i, part := range parts {
				if part != "" {
					sqlParts = append(sqlParts, "'"+part+"'")
				}
				if i < len(parts)-1 {
					sqlParts = append(sqlParts, "numbered_rows._row_num::text")
				}
			}
			// Concatenate and cast to integer
			return "(" + strings.Join(sqlParts, " || ") + ")::integer"
		}
		// No placeholder, return as unquoted integer
		return template
	}

	// Handle text type (default)
	// Check if template contains ${index}
	if !strings.Contains(template, "${index}") {
		// No placeholder, return as quoted string
		return "'" + template + "'"
	}

	// Split by ${index} to build SQL concatenation
	parts := strings.Split(template, "${index}")

	var sqlParts []string
	for i, part := range parts {
		if part != "" {
			// Add string literal part
			sqlParts = append(sqlParts, "'"+part+"'")
		}
		// Add row number between parts (except after last part)
		if i < len(parts)-1 {
			sqlParts = append(sqlParts, "numbered_rows._row_num")
		}
	}

	return strings.Join(sqlParts, " || ")
}

// quoteIdentifier quotes a PostgreSQL identifier
func quoteIdentifier(name string) string {
	return fmt.Sprintf("\"%s\"", strings.ReplaceAll(name, "\"", "\"\""))
}

// ApplyParams contains parameters needed to apply anonymization rules
type ApplyParams struct {
	DatabaseName    string
	PostgresVersion string
	PostgresPort    int
}

// Apply loads and applies anonymization rules to a database
// Returns the number of rules applied and any error
func Apply(ctx context.Context, db *gorm.DB, params ApplyParams, logger zerolog.Logger) (int, error) {
	// Load all anonymization rules
	var rules []models.AnonRule
	if err := db.Find(&rules).Error; err != nil {
		return 0, fmt.Errorf("failed to load anon rules: %w", err)
	}

	if len(rules) == 0 {
		logger.Info().
			Str("database_name", params.DatabaseName).
			Msg("No anonymization rules configured, skipping")
		return 0, nil
	}

	logger.Info().
		Str("database_name", params.DatabaseName).
		Int("rule_count", len(rules)).
		Msg("Applying anonymization rules")

	// Extract unique table names from rules
	tableMap := make(map[string]bool)
	for _, rule := range rules {
		tableMap[rule.Table] = true
	}
	var tables []string
	for table := range tableMap {
		tables = append(tables, table)
	}

	// Query for primary keys
	primaryKeys := make(map[string]string)
	if len(tables) > 0 {
		pkQuerySQL := generatePrimaryKeyQuerySQL(tables)
		pkScript := fmt.Sprintf(`#!/bin/bash
set -euo pipefail
DATABASE_NAME="%s"
PG_VERSION="%s"
PG_PORT="%d"
PG_BIN="/usr/lib/postgresql/${PG_VERSION}/bin"

sudo -u postgres ${PG_BIN}/psql -p ${PG_PORT} -d "${DATABASE_NAME}" -t -A -F'|' <<'PK_QUERY'
%s
PK_QUERY
`, params.DatabaseName, params.PostgresVersion, params.PostgresPort, pkQuerySQL)

		cmd := exec.CommandContext(ctx, "bash", "-c", pkScript)
		outputBytes, err := cmd.CombinedOutput()
		if err != nil {
			// Log warning but continue - we'll use ctid as fallback
			logger.Warn().
				Err(err).
				Str("output", string(outputBytes)).
				Msg("Failed to query primary keys, will use ctid for ordering")
		} else {
			// Parse output: table_name|column_name (one per line)
			output := strings.TrimSpace(string(outputBytes))
			if output != "" {
				for _, line := range strings.Split(output, "\n") {
					parts := strings.Split(line, "|")
					if len(parts) == 2 {
						tableName := strings.TrimSpace(parts[0])
						columnName := strings.TrimSpace(parts[1])
						primaryKeys[tableName] = columnName
						logger.Debug().
							Str("table", tableName).
							Str("pk_column", columnName).
							Msg("Detected primary key")
					}
				}
			}
		}
	}

	// Generate SQL from rules with primary key information
	sql := GenerateSQL(rules, primaryKeys)
	if sql == "" {
		logger.Warn().Msg("Generated empty SQL from rules")
		return 0, nil
	}

	// Execute anonymization SQL on the database
	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

DATABASE_NAME="%s"
PG_VERSION="%s"
PG_PORT="%d"
PG_BIN="/usr/lib/postgresql/${PG_VERSION}/bin"

echo "Applying anonymization rules to database ${DATABASE_NAME}"

# Execute anonymization SQL with correct port
sudo -u postgres ${PG_BIN}/psql -p ${PG_PORT} -d "${DATABASE_NAME}" <<'ANONYMIZE_SQL'
%s
ANONYMIZE_SQL

echo "Anonymization completed successfully"
`, params.DatabaseName, params.PostgresVersion, params.PostgresPort, sql)

	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	if err != nil {
		logger.Error().
			Err(err).
			Str("output", output).
			Str("database_name", params.DatabaseName).
			Msg("Failed to execute anonymization script")
		return 0, fmt.Errorf("anonymization script execution failed: %w", err)
	}

	logger.Info().
		Str("database_name", params.DatabaseName).
		Int("rule_count", len(rules)).
		Str("output", output).
		Msg("Anonymization rules applied successfully")

	return len(rules), nil
}
