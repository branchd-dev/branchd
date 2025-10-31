package auth

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const (
	service = "branchd-cli"
)

// getKeyringKey returns a unique key for storing JWT tokens per server
func getKeyringKey(serverIP string) string {
	return fmt.Sprintf("jwt-%s", serverIP)
}

// SaveToken persists the JWT token securely in the OS keychain/credential manager
func SaveToken(serverIP, token string) error {
	key := getKeyringKey(serverIP)
	if err := keyring.Set(service, key, token); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}
	return nil
}

// LoadToken retrieves the JWT token from the OS keychain/credential manager
func LoadToken(serverIP string) (string, error) {
	key := getKeyringKey(serverIP)
	token, err := keyring.Get(service, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", fmt.Errorf("not authenticated. Please run 'branchd login' first")
		}
		return "", fmt.Errorf("failed to load token: %w", err)
	}
	return token, nil
}

// DeleteToken removes the JWT token from the OS keychain/credential manager
func DeleteToken(serverIP string) error {
	key := getKeyringKey(serverIP)
	if err := keyring.Delete(service, key); err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to delete token: %w", err)
	}
	return nil
}

// ClearAllTokens removes all stored tokens (useful for logout)
func ClearAllTokens() error {
	// Note: go-keyring doesn't provide a way to list all keys,
	// so we can only delete tokens if we know the server IPs.
	// For now, this is a placeholder that could be extended
	// to read from config and delete all known servers.
	return nil
}
