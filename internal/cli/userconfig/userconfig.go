package userconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	configDirName  = "branchd"
	configFileName = "config.json"
)

// UserConfig represents the user's local configuration stored in ~/.config/branchd/config.json
type UserConfig struct {
	SelectedServerIP string `json:"selected_server_ip"`
}

// GetConfigPath returns the path to the user config file
func GetConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", configDirName)
	return filepath.Join(configDir, configFileName), nil
}

// Load reads the user configuration file
func Load() (*UserConfig, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	// If config doesn't exist, return empty config
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return &UserConfig{}, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read user config file: %w", err)
	}

	var cfg UserConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse user config file: %w", err)
	}

	return &cfg, nil
}

// Save writes the user configuration to a file
func Save(cfg *UserConfig) error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	// Create config directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal user config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write user config file: %w", err)
	}

	return nil
}

// SetSelectedServer updates the selected server IP and saves the config
func SetSelectedServer(serverIP string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	cfg.SelectedServerIP = serverIP
	return Save(cfg)
}

// GetSelectedServer returns the selected server IP, or empty string if not set
func GetSelectedServer() (string, error) {
	cfg, err := Load()
	if err != nil {
		return "", err
	}

	return cfg.SelectedServerIP, nil
}
