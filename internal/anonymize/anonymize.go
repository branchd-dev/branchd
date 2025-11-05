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

// GenerateSQL generates anonymization SQL from rules
// Uses PostgreSQL row_number() for deterministic anonymization
func GenerateSQL(rules []models.AnonRule) string {
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
		sql := generateTableUpdateSQL(table, rules)
		sqlStatements = append(sqlStatements, sql)
	}

	return strings.Join(sqlStatements, "\n\n")
}

// generateTableUpdateSQL generates UPDATE statement for a single table
func generateTableUpdateSQL(table string, rules []models.AnonRule) string {
	if len(rules) == 0 {
		return ""
	}

	// Build SET clause with row_number replacement
	var setClauses []string
	for _, rule := range rules {
		// Replace ${index} with row number in the template
		// Use row_number() OVER (ORDER BY primary key or ctid for deterministic ordering)
		setValue := renderTemplate(rule.Template)
		setClauses = append(setClauses, fmt.Sprintf("%s = %s", quoteIdentifier(rule.Column), setValue))
	}

	// Use CTE with row numbers for deterministic updates
	sql := fmt.Sprintf(`-- Anonymize table: %s
WITH numbered_rows AS (
  SELECT ctid, row_number() OVER (ORDER BY ctid) as _row_num
  FROM %s
)
UPDATE %s
SET %s
FROM numbered_rows
WHERE %s.ctid = numbered_rows.ctid;`,
		table,
		quoteIdentifier(table),
		quoteIdentifier(table),
		strings.Join(setClauses, ",\n    "),
		quoteIdentifier(table),
	)

	return sql
}

// renderTemplate converts template string to SQL expression
// Replaces ${index} with row number reference
func renderTemplate(template string) string {
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
	DatabaseName   string
	PostgresVersion string
	PostgresPort   int
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

	// Generate SQL from rules
	sql := GenerateSQL(rules)
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
