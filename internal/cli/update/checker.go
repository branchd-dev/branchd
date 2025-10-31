package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	GitHubAPIURL    = "https://api.github.com/repos/branchd-dev/branchd/releases/latest"
	UserAgent       = "branchd-cli"
	DownloadBaseURL = "https://github.com/branchd-dev/branchd/releases/download"
)

// Release represents a GitHub release
type Release struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	HTMLURL string `json:"html_url"`
}

// GetLatestVersion fetches the latest version from GitHub
func GetLatestVersion() (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", GitHubAPIURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var release Release
	if err := json.Unmarshal(body, &release); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return release.TagName, nil
}

// CheckForUpdate checks if a new version is available
func CheckForUpdate(currentVersion string) (bool, string, error) {
	// Fetch latest version from GitHub
	latestVersion, err := GetLatestVersion()
	if err != nil {
		return false, "", err
	}

	// Compare versions
	updateAvailable := compareVersions(currentVersion, latestVersion)

	return updateAvailable, latestVersion, nil
}

// compareVersions returns true if latest is newer than current
func compareVersions(current, latest string) bool {
	// Remove 'v' prefix if present
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")

	// Handle 'dev' version
	if current == "dev" {
		return true // Always suggest update from dev version
	}

	// Simple string comparison (works for semver like "1.0.0")
	return current != latest
}

// PrintUpdateNotification prints a message if an update is available
func PrintUpdateNotification(currentVersion string) {
	updateAvailable, latestVersion, err := CheckForUpdate(currentVersion)
	if err != nil {
		// Silently ignore errors - update check is optional
		return
	}

	if updateAvailable {
		fmt.Fprintf(os.Stderr, "New version %s -> %s. Run: branchd update\n\n", currentVersion, latestVersion)
	}
}
