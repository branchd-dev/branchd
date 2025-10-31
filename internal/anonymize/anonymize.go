package anonymize

import (
	"fmt"
	"strings"

	"github.com/branchd-dev/branchd/internal/models"
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
