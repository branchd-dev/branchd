package config

import (
	"encoding/json"
	"testing"
)

func TestAnonRule_Parse_AutoDetection(t *testing.T) {
	tests := []struct {
		name             string
		template         string
		expectedTemplate string
		expectedType     string
		shouldError      bool
	}{
		{
			name:             "string template",
			template:         `"user${index}@email.com"`,
			expectedTemplate: "user${index}@email.com",
			expectedType:     "text",
			shouldError:      false,
		},
		{
			name:             "integer template",
			template:         `2222`,
			expectedTemplate: "2222",
			expectedType:     "integer",
			shouldError:      false,
		},
		{
			name:             "float template",
			template:         `3.14`,
			expectedTemplate: "3.140000",
			expectedType:     "integer",
			shouldError:      false,
		},
		{
			name:             "boolean true",
			template:         `true`,
			expectedTemplate: "true",
			expectedType:     "boolean",
			shouldError:      false,
		},
		{
			name:             "boolean false",
			template:         `false`,
			expectedTemplate: "false",
			expectedType:     "boolean",
			shouldError:      false,
		},
		{
			name:             "null value",
			template:         `null`,
			expectedTemplate: "",
			expectedType:     "null",
			shouldError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := AnonRule{
				Table:    "users",
				Column:   "test_column",
				Template: json.RawMessage(tt.template),
			}

			parsed, err := rule.Parse()

			if tt.shouldError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if parsed.Template != tt.expectedTemplate {
				t.Errorf("template = %q, want %q", parsed.Template, tt.expectedTemplate)
			}

			if parsed.ColumnType != tt.expectedType {
				t.Errorf("columnType = %q, want %q", parsed.ColumnType, tt.expectedType)
			}

			if parsed.Table != "users" {
				t.Errorf("table = %q, want %q", parsed.Table, "users")
			}

			if parsed.Column != "test_column" {
				t.Errorf("column = %q, want %q", parsed.Column, "test_column")
			}
		})
	}
}

func TestAnonRule_Parse_ExplicitType(t *testing.T) {
	tests := []struct {
		name             string
		template         string
		explicitType     string
		expectedTemplate string
		expectedType     string
		shouldError      bool
		errorContains    string
	}{
		{
			name:             "string with explicit integer type",
			template:         `"123${index}"`,
			explicitType:     "integer",
			expectedTemplate: "123${index}",
			expectedType:     "integer",
			shouldError:      false,
		},
		{
			name:             "number with explicit text type",
			template:         `2222`,
			explicitType:     "text",
			expectedTemplate: "2222",
			expectedType:     "text",
			shouldError:      false,
		},
		{
			name:             "string with explicit boolean type",
			template:         `"true"`,
			explicitType:     "boolean",
			expectedTemplate: "true",
			expectedType:     "boolean",
			shouldError:      false,
		},
		{
			name:             "boolean with explicit text type",
			template:         `false`,
			explicitType:     "text",
			expectedTemplate: "false",
			expectedType:     "text",
			shouldError:      false,
		},
		{
			name:             "explicit null type ignores template",
			template:         `"anything"`,
			explicitType:     "null",
			expectedTemplate: "",
			expectedType:     "null",
			shouldError:      false,
		},
		{
			name:          "invalid explicit type",
			template:      `"test"`,
			explicitType:  "invalid",
			shouldError:   true,
			errorContains: "invalid type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := AnonRule{
				Table:    "users",
				Column:   "test_column",
				Template: json.RawMessage(tt.template),
				Type:     tt.explicitType,
			}

			parsed, err := rule.Parse()

			if tt.shouldError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("error = %q, should contain %q", err.Error(), tt.errorContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if parsed.Template != tt.expectedTemplate {
				t.Errorf("template = %q, want %q", parsed.Template, tt.expectedTemplate)
			}

			if parsed.ColumnType != tt.expectedType {
				t.Errorf("columnType = %q, want %q", parsed.ColumnType, tt.expectedType)
			}
		})
	}
}

func TestAnonRule_Parse_EdgeCases(t *testing.T) {
	t.Run("empty template", func(t *testing.T) {
		rule := AnonRule{
			Table:    "users",
			Column:   "test",
			Template: json.RawMessage(``),
		}

		_, err := rule.Parse()
		if err == nil {
			t.Error("expected error for empty template")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		rule := AnonRule{
			Table:    "users",
			Column:   "test",
			Template: json.RawMessage(`{invalid`),
		}

		_, err := rule.Parse()
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("complex object not supported", func(t *testing.T) {
		rule := AnonRule{
			Table:    "users",
			Column:   "test",
			Template: json.RawMessage(`{"key": "value"}`),
		}

		_, err := rule.Parse()
		if err == nil {
			t.Error("expected error for unsupported type (object)")
		}
	})

	t.Run("array not supported", func(t *testing.T) {
		rule := AnonRule{
			Table:    "users",
			Column:   "test",
			Template: json.RawMessage(`[1, 2, 3]`),
		}

		_, err := rule.Parse()
		if err == nil {
			t.Error("expected error for unsupported type (array)")
		}
	})
}

func TestAnonRule_Parse_RealWorldExamples(t *testing.T) {
	tests := []struct {
		name             string
		rule             AnonRule
		expectedTemplate string
		expectedType     string
	}{
		{
			name: "email anonymization",
			rule: AnonRule{
				Table:    "users",
				Column:   "email",
				Template: json.RawMessage(`"user${index}@email.com"`),
			},
			expectedTemplate: "user${index}@email.com",
			expectedType:     "text",
		},
		{
			name: "port number",
			rule: AnonRule{
				Table:    "servers",
				Column:   "port",
				Template: json.RawMessage(`5432`),
			},
			expectedTemplate: "5432",
			expectedType:     "integer",
		},
		{
			name: "string port with index and explicit integer type",
			rule: AnonRule{
				Table:    "servers",
				Column:   "sftp_port",
				Template: json.RawMessage(`"123${index}"`),
				Type:     "integer",
			},
			expectedTemplate: "123${index}",
			expectedType:     "integer",
		},
		{
			name: "boolean active flag",
			rule: AnonRule{
				Table:    "users",
				Column:   "active",
				Template: json.RawMessage(`false`),
			},
			expectedTemplate: "false",
			expectedType:     "boolean",
		},
		{
			name: "null deleted_at",
			rule: AnonRule{
				Table:    "users",
				Column:   "deleted_at",
				Template: json.RawMessage(`null`),
			},
			expectedTemplate: "",
			expectedType:     "null",
		},
		{
			name: "static SSN",
			rule: AnonRule{
				Table:    "users",
				Column:   "ssn",
				Template: json.RawMessage(`"123-45-6789"`),
			},
			expectedTemplate: "123-45-6789",
			expectedType:     "text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := tt.rule.Parse()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if parsed.Template != tt.expectedTemplate {
				t.Errorf("template = %q, want %q", parsed.Template, tt.expectedTemplate)
			}

			if parsed.ColumnType != tt.expectedType {
				t.Errorf("columnType = %q, want %q", parsed.ColumnType, tt.expectedType)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
