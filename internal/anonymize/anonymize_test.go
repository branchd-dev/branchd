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
				{Table: "users", Column: "email", Template: "user_${index}@example.com"},
			},
			want: "UPDATE",
		},
		{
			name: "multiple columns same table",
			rules: []models.AnonRule{
				{Table: "users", Column: "email", Template: "user_${index}@example.com"},
				{Table: "users", Column: "name", Template: "User ${index}"},
			},
			want: "numbered_rows._row_num",
		},
		{
			name: "multiple tables",
			rules: []models.AnonRule{
				{Table: "users", Column: "email", Template: "user_${index}@example.com"},
				{Table: "orders", Column: "reference", Template: "ORD-${index}"},
			},
			want: "-- Anonymize table:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateSQL(tt.rules)
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
		name     string
		template string
		want     string
	}{
		{
			name:     "simple template with index",
			template: "user_${index}@example.com",
			want:     "'user_' || numbered_rows._row_num || '@example.com'",
		},
		{
			name:     "template with multiple placeholders",
			template: "User ${index}",
			want:     "'User ' || numbered_rows._row_num",
		},
		{
			name:     "template without placeholder",
			template: "static_value",
			want:     "'static_value'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderTemplate(tt.template); got != tt.want {
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
