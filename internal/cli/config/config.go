package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const ConfigFileName = "branchd.json"

// Server represents a Branchd server configuration
type Server struct {
	IP    string `json:"ip"`
	Alias string `json:"alias"`
}

// AnonRule represents an anonymization rule
type AnonRule struct {
	Table    string          `json:"table"`
	Column   string          `json:"column"`
	Template json.RawMessage `json:"template"`
	Type     string          `json:"type,omitempty"` // Optional: "text", "integer", "boolean", "null" - overrides auto-detection
}

// ParsedAnonRule represents a parsed anonymization rule with type information
type ParsedAnonRule struct {
	Table      string
	Column     string
	Template   string // String representation of the template value
	ColumnType string // "text", "integer", "boolean", "null"
}

// Parse parses the JSON template and returns type information
func (r *AnonRule) Parse() (ParsedAnonRule, error) {
	parsed := ParsedAnonRule{
		Table:  r.Table,
		Column: r.Column,
	}

	// Try to unmarshal as different types to detect the JSON type
	if len(r.Template) == 0 {
		return parsed, fmt.Errorf("template is empty")
	}

	// If type is explicitly specified, use it and extract the template value
	if r.Type != "" {
		// Validate the type
		validTypes := map[string]bool{"text": true, "integer": true, "boolean": true, "null": true}
		if !validTypes[r.Type] {
			return parsed, fmt.Errorf("invalid type '%s', must be one of: text, integer, boolean, null", r.Type)
		}

		parsed.ColumnType = r.Type

		// For null type, template is ignored
		if r.Type == "null" {
			parsed.Template = ""
			return parsed, nil
		}

		// Extract template value as string
		var strVal string
		if err := json.Unmarshal(r.Template, &strVal); err == nil {
			parsed.Template = strVal
			return parsed, nil
		}

		// If string unmarshal fails, try other JSON types and convert to string
		// This handles cases like: template is number but type is "text"

		// Try boolean
		var boolVal bool
		if err := json.Unmarshal(r.Template, &boolVal); err == nil {
			if boolVal {
				parsed.Template = "true"
			} else {
				parsed.Template = "false"
			}
			return parsed, nil
		}

		// Try number
		var numVal float64
		if err := json.Unmarshal(r.Template, &numVal); err == nil {
			if numVal == float64(int64(numVal)) {
				parsed.Template = fmt.Sprintf("%d", int64(numVal))
			} else {
				parsed.Template = fmt.Sprintf("%f", numVal)
			}
			return parsed, nil
		}

		return parsed, fmt.Errorf("failed to parse template with explicit type '%s'", r.Type)
	}

	// Auto-detect type from JSON

	// Check for null
	if string(r.Template) == "null" {
		parsed.ColumnType = "null"
		parsed.Template = ""
		return parsed, nil
	}

	// Try boolean
	var boolVal bool
	if err := json.Unmarshal(r.Template, &boolVal); err == nil {
		parsed.ColumnType = "boolean"
		if boolVal {
			parsed.Template = "true"
		} else {
			parsed.Template = "false"
		}
		return parsed, nil
	}

	// Try number (integer or float)
	var numVal float64
	if err := json.Unmarshal(r.Template, &numVal); err == nil {
		parsed.ColumnType = "integer"
		// Convert to string, handle both int and float
		if numVal == float64(int64(numVal)) {
			parsed.Template = fmt.Sprintf("%d", int64(numVal))
		} else {
			parsed.Template = fmt.Sprintf("%f", numVal)
		}
		return parsed, nil
	}

	// Try string (must be last, as it's the most permissive)
	var strVal string
	if err := json.Unmarshal(r.Template, &strVal); err == nil {
		parsed.ColumnType = "text"
		parsed.Template = strVal
		return parsed, nil
	}

	return parsed, fmt.Errorf("unsupported template type: %s", string(r.Template))
}

// Config represents the CLI configuration file
type Config struct {
	Servers   []Server   `json:"servers"`
	AnonRules []AnonRule `json:"anonRules,omitempty"`
}

// DefaultConfig returns a default configuration with example servers
func DefaultConfig() *Config {
	return &Config{
		Servers: []Server{
			{
				IP:    "",
				Alias: "e.g. us-east-1 server",
			},
		},
	}
}

// FindConfigFile searches for branchd.json in current directory and parent directories
func FindConfigFile() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	// Search upwards until we find branchd.json or reach root
	dir := currentDir
	for {
		configPath := filepath.Join(dir, ConfigFileName)
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("branchd.json not found in %s or any parent directory", currentDir)
}

// Load reads the configuration file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

// LoadFromCurrentDir loads config from current directory or parent directories
func LoadFromCurrentDir() (*Config, error) {
	configPath, err := FindConfigFile()
	if err != nil {
		return nil, err
	}

	return Load(configPath)
}

// Save writes the configuration to a file
func Save(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetServerByAlias returns a server by its alias
func (c *Config) GetServerByAlias(alias string) (*Server, error) {
	for _, server := range c.Servers {
		if server.Alias == alias {
			return &server, nil
		}
	}
	return nil, fmt.Errorf("server with alias '%s' not found", alias)
}

// GetDefaultServer returns the first server in the list
func (c *Config) GetDefaultServer() (*Server, error) {
	if len(c.Servers) == 0 {
		return nil, fmt.Errorf("no servers configured in branchd.json")
	}
	return &c.Servers[0], nil
}
