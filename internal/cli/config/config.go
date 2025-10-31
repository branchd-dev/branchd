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

// Config represents the CLI configuration file
type Config struct {
	Servers []Server `json:"servers"`
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
