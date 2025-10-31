package update

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// SelfUpdate downloads and installs the latest version
func SelfUpdate(currentVersion string) error {
	// Get latest version
	latestVersion, err := GetLatestVersion()
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	// Check if already up to date
	if !compareVersions(currentVersion, latestVersion) {
		fmt.Printf("Already up to date (version %s)\n", currentVersion)
		return nil
	}

	fmt.Printf("Updating from %s to %s...\n", currentVersion, latestVersion)

	// Detect platform
	binaryName, err := getBinaryName()
	if err != nil {
		return err
	}

	// Download new binary
	fmt.Println("Downloading new version...")
	downloadURL := fmt.Sprintf("%s/%s/%s", DownloadBaseURL, latestVersion, binaryName)

	tmpFile, err := downloadFile(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}
	defer os.Remove(tmpFile)

	// Download and verify checksum
	fmt.Println("Verifying checksum...")
	checksumURL := fmt.Sprintf("%s.sha256", downloadURL)
	if err := verifyChecksum(tmpFile, checksumURL); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	// Get current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Replace binary
	fmt.Println("Installing new version...")
	if err := replaceBinary(tmpFile, execPath); err != nil {
		return fmt.Errorf("failed to install update: %w", err)
	}

	fmt.Printf("\n✓ Successfully updated to version %s!\n", latestVersion)

	return nil
}

// getBinaryName returns the binary name for the current platform
func getBinaryName() (string, error) {
	var binaryName string

	switch runtime.GOOS {
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			binaryName = "branchd-linux-amd64"
		case "arm64":
			binaryName = "branchd-linux-arm64"
		default:
			return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
		}
	case "darwin":
		switch runtime.GOARCH {
		case "amd64":
			binaryName = "branchd-darwin-amd64"
		case "arm64":
			binaryName = "branchd-darwin-arm64"
		default:
			return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
		}
	case "windows":
		switch runtime.GOARCH {
		case "amd64":
			binaryName = "branchd-windows-amd64.exe"
		default:
			return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
		}
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	return binaryName, nil
}

// downloadFile downloads a file to a temporary location
func downloadFile(url string) (string, error) {
	client := &http.Client{
		Timeout: 5 * time.Minute, // Binary download can take time
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Create temp file
	tmpFile, err := os.CreateTemp("", "branchd-update-*")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	// Copy data
	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

// verifyChecksum downloads and verifies the SHA256 checksum
func verifyChecksum(filePath, checksumURL string) error {
	// Download checksum file
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest("GET", checksumURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download checksum (status %d)", resp.StatusCode)
	}

	checksumData, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Parse checksum (format: "hash  filename")
	parts := strings.Fields(string(checksumData))
	if len(parts) < 1 {
		return fmt.Errorf("invalid checksum format")
	}
	expectedHash := parts[0]

	// Calculate actual hash
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actualHash := fmt.Sprintf("%x", h.Sum(nil))

	if actualHash != expectedHash {
		return fmt.Errorf("checksum mismatch (expected: %s, got: %s)", expectedHash, actualHash)
	}

	return nil
}

// replaceBinary replaces the current binary with the new one
func replaceBinary(newBinaryPath, currentBinaryPath string) error {
	// Make new binary executable
	if err := os.Chmod(newBinaryPath, 0755); err != nil {
		return err
	}

	// On Windows, we can't replace a running executable
	// Instead, rename the old one and move the new one in place
	if runtime.GOOS == "windows" {
		backupPath := currentBinaryPath + ".old"

		// Remove old backup if it exists
		os.Remove(backupPath)

		// Rename current binary
		if err := os.Rename(currentBinaryPath, backupPath); err != nil {
			return fmt.Errorf("failed to backup current binary: %w", err)
		}

		// Move new binary into place
		if err := os.Rename(newBinaryPath, currentBinaryPath); err != nil {
			// Try to restore backup
			os.Rename(backupPath, currentBinaryPath)
			return fmt.Errorf("failed to install new binary: %w", err)
		}

		fmt.Println("\nNote: Old binary saved as .old - you can delete it manually")
		return nil
	}

	// On Unix-like systems, we can replace the file directly
	// Create a backup first
	backupPath := currentBinaryPath + ".backup"
	if err := copyFile(currentBinaryPath, backupPath); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Replace the binary
	if err := copyFile(newBinaryPath, currentBinaryPath); err != nil {
		// Restore backup on failure
		copyFile(backupPath, currentBinaryPath)
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	// Remove backup
	os.Remove(backupPath)

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	// Copy permissions
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, sourceInfo.Mode())
}
