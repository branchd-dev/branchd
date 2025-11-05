package anonymize

import (
	"strings"
	"testing"

	"github.com/branchd-dev/branchd/internal/models"
)

func TestGenerateSQL(t *testing.T) {
	tests := []struct {
		name  string
		rules []models.AnonRule
		want  string
	}{
		{
			name:  "empty rules",
			rules: []models.AnonRule{},
			want:  "",
		},
		{
			name: "single column anonymization",
			rules: []models.AnonRule{
				{Table: "users", Column: "email", Template: "user_${index}@example.com", ColumnType: "text"},
			},
			want: "UPDATE",
		},
		{
			name: "multiple columns same table",
			rules: []models.AnonRule{
				{Table: "users", Column: "email", Template: "user_${index}@example.com", ColumnType: "text"},
				{Table: "users", Column: "name", Template: "User ${index}", ColumnType: "text"},
			},
			want: "numbered_rows._row_num",
		},
		{
			name: "multiple tables",
			rules: []models.AnonRule{
				{Table: "users", Column: "email", Template: "user_${index}@example.com", ColumnType: "text"},
				{Table: "orders", Column: "reference", Template: "ORD-${index}", ColumnType: "text"},
			},
			want: "-- Anonymize table:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateSQL(tt.rules, make(map[string]string))
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Errorf("GenerateSQL() output doesn't contain expected string\nwant substring: %v\ngot: %v", tt.want, got)
			}
			if tt.want == "" && got != "" {
				t.Errorf("GenerateSQL() = %v, want empty string", got)
			}
		})
	}
}

func TestRenderTemplate(t *testing.T) {
	tests := []struct {
		name       string
		template   string
		columnType string
		want       string
	}{
		{
			name:       "simple text template with index",
			template:   "user_${index}@example.com",
			columnType: "text",
			want:       "'user_' || numbered_rows._row_num || '@example.com'",
		},
		{
			name:       "text template with multiple placeholders",
			template:   "User ${index}",
			columnType: "text",
			want:       "'User ' || numbered_rows._row_num",
		},
		{
			name:       "text template without placeholder",
			template:   "static_value",
			columnType: "text",
			want:       "'static_value'",
		},
		{
			name:       "integer template without placeholder",
			template:   "2222",
			columnType: "integer",
			want:       "2222",
		},
		{
			name:       "integer template with index",
			template:   "${index}",
			columnType: "integer",
			want:       "(numbered_rows._row_num::text)::integer",
		},
		{
			name:       "boolean true",
			template:   "true",
			columnType: "boolean",
			want:       "true",
		},
		{
			name:       "boolean false",
			template:   "false",
			columnType: "boolean",
			want:       "false",
		},
		{
			name:       "null type",
			template:   "",
			columnType: "null",
			want:       "NULL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderTemplate(tt.template, tt.columnType); got != tt.want {
				t.Errorf("renderTemplate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQuoteIdentifier(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{
			name: "simple identifier",
			id:   "users",
			want: "\"users\"",
		},
		{
			name: "identifier with quotes",
			id:   "user\"name",
			want: "\"user\"\"name\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quoteIdentifier(tt.id); got != tt.want {
				t.Errorf("quoteIdentifier() = %v, want %v", got, tt.want)
			}
		})
	}
}
