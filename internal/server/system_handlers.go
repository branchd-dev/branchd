package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/branchd-dev/branchd/internal/models"
	"github.com/branchd-dev/branchd/internal/pgclient"
)

// SystemInfoResponse contains VM and source database information
type SystemInfoResponse struct {
	Version        string           `json:"version"`
	VM             VMMetrics        `json:"vm"`
	SourceDatabase *DatabaseMetrics `json:"source_database,omitempty"`
}

// VMMetrics contains VM resource information
type VMMetrics struct {
	CPUCount       int     `json:"cpu_count"`
	MemoryTotalGB  float64 `json:"memory_total_gb"`
	MemoryUsedGB   float64 `json:"memory_used_gb"`
	MemoryFreeGB   float64 `json:"memory_free_gb"`
	DiskTotalGB    float64 `json:"disk_total_gb"`
	DiskUsedGB     float64 `json:"disk_used_gb"`
	DiskAvailableGB float64 `json:"disk_available_gb"`
	DiskUsedPercent float64 `json:"disk_used_percent"`
}

// DatabaseMetrics contains source database information
type DatabaseMetrics struct {
	Name          string  `json:"name"`
	Version       string  `json:"version"`
	MajorVersion  int     `json:"major_version"`
	SizeGB        float64 `json:"size_gb"`
	Connected     bool    `json:"connected"`
	Error         string  `json:"error,omitempty"`
}

// @Summary Get system and source database information
// @Description Returns VM metrics (CPU, memory, disk) and source database info if configured
// @Tags system
// @Produce json
// @Success 200 {object} SystemInfoResponse
// @Failure 500 {object} map[string]interface{}
// @Router /api/system/info [get]
func (s *Server) getSystemInfo(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get VM metrics
	vmMetrics, err := s.getVMMetrics(ctx)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to get VM metrics")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get VM metrics: %v", err)})
		return
	}

	response := SystemInfoResponse{
		Version: s.version,
		VM:      *vmMetrics,
	}

	// Try to get source database metrics if config exists
	var config models.Config
	if err := s.db.First(&config).Error; err == nil && config.ConnectionString != "" {
		dbMetrics := s.getSourceDatabaseMetrics(ctx, config.ConnectionString, config.DatabaseName)
		response.SourceDatabase = dbMetrics
	}

	c.JSON(http.StatusOK, response)
}

// getVMMetrics retrieves VM resource information
func (s *Server) getVMMetrics(ctx context.Context) (*VMMetrics, error) {
	metrics := &VMMetrics{
		CPUCount: runtime.NumCPU(),
	}

	// Get memory info from /proc/meminfo (Linux)
	memInfo, err := exec.CommandContext(ctx, "bash", "-c",
		"grep -E '^(MemTotal|MemAvailable):' /proc/meminfo | awk '{print $2}'").Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(memInfo)), "\n")
		if len(lines) >= 2 {
			if total, err := strconv.ParseFloat(lines[0], 64); err == nil {
				metrics.MemoryTotalGB = total / (1024 * 1024) // KB to GB
			}
			if available, err := strconv.ParseFloat(lines[1], 64); err == nil {
				metrics.MemoryFreeGB = available / (1024 * 1024) // KB to GB
				metrics.MemoryUsedGB = metrics.MemoryTotalGB - metrics.MemoryFreeGB
			}
		}
	}

	// Get disk info from ZFS pool
	zfsInfo, err := exec.CommandContext(ctx, "bash", "-c",
		"zfs list -H -o available,used -p tank | head -1").Output()
	if err == nil {
		fields := strings.Fields(strings.TrimSpace(string(zfsInfo)))
		if len(fields) >= 2 {
			if available, err := strconv.ParseFloat(fields[0], 64); err == nil {
				metrics.DiskAvailableGB = available / (1024 * 1024 * 1024)
			}
			if used, err := strconv.ParseFloat(fields[1], 64); err == nil {
				metrics.DiskUsedGB = used / (1024 * 1024 * 1024)
			}
			metrics.DiskTotalGB = metrics.DiskAvailableGB + metrics.DiskUsedGB
			if metrics.DiskTotalGB > 0 {
				metrics.DiskUsedPercent = (metrics.DiskUsedGB / metrics.DiskTotalGB) * 100
			}
		}
	}

	return metrics, nil
}

// getSourceDatabaseMetrics retrieves source database information
func (s *Server) getSourceDatabaseMetrics(ctx context.Context, connectionString, databaseName string) *DatabaseMetrics {
	metrics := &DatabaseMetrics{
		Name:      databaseName,
		Connected: false,
	}

	// Get database info using pgclient
	info, err := pgclient.GetDatabaseInfo(ctx, connectionString)
	if err != nil {
		metrics.Error = err.Error()
		return metrics
	}

	metrics.Connected = true
	metrics.SizeGB = info.SizeGB
	metrics.MajorVersion = info.MajorVersion
	metrics.Version = fmt.Sprintf("PostgreSQL %d", info.MajorVersion)

	// Try to get exact version string
	client, err := pgclient.NewClient(connectionString)
	if err == nil {
		defer client.Close()
		if version, err := client.GetVersion(ctx); err == nil {
			metrics.Version = version
		}
	}

	return metrics
}

// LatestVersionResponse contains the latest available version from GitHub
type LatestVersionResponse struct {
	LatestVersion  string `json:"latest_version"`
	CurrentVersion string `json:"current_version"`
	UpdateAvailable bool   `json:"update_available"`
}

// @Summary Get latest version from GitHub releases
// @Description Checks GitHub API for the latest Branchd release
// @Tags system
// @Produce json
// @Success 200 {object} LatestVersionResponse
// @Failure 500 {object} map[string]interface{}
// @Router /api/system/latest-version [get]
func (s *Server) getLatestVersion(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Fetch latest release from GitHub API
	cmd := exec.CommandContext(ctx, "curl", "-sL", "https://api.github.com/repos/branchd-dev/branchd/releases/latest")
	output, err := cmd.Output()
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to fetch latest release from GitHub")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check for updates"})
		return
	}

	// Parse JSON to extract tag_name
	cmd = exec.CommandContext(ctx, "bash", "-c", fmt.Sprintf("echo '%s' | jq -r '.tag_name'", string(output)))
	latestVersionBytes, err := cmd.Output()
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to parse GitHub release response")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse update information"})
		return
	}

	latestVersion := strings.TrimSpace(string(latestVersionBytes))
	currentVersion := s.version

	// Compare versions (simple string comparison for now)
	updateAvailable := latestVersion != "" && latestVersion != "null" && latestVersion != currentVersion

	c.JSON(http.StatusOK, LatestVersionResponse{
		LatestVersion:   latestVersion,
		CurrentVersion:  currentVersion,
		UpdateAvailable: updateAvailable,
	})
}

// @Summary Update Branchd server to latest version
// @Description Downloads and installs the latest Branchd release, then restarts services
// @Tags system
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/system/update [post]
func (s *Server) updateServer(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if already on latest version
	cmd := exec.CommandContext(ctx, "curl", "-sL", "https://api.github.com/repos/branchd-dev/branchd/releases/latest")
	output, err := cmd.Output()
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to fetch latest release from GitHub")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check for updates"})
		return
	}

	cmd = exec.CommandContext(ctx, "bash", "-c", fmt.Sprintf("echo '%s' | jq -r '.tag_name'", string(output)))
	latestVersionBytes, err := cmd.Output()
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to parse GitHub release response")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse update information"})
		return
	}

	latestVersion := strings.TrimSpace(string(latestVersionBytes))

	if latestVersion == s.version {
		c.JSON(http.StatusOK, gin.H{
			"message": "Already on latest version",
			"version": s.version,
		})
		return
	}

	// Trigger update in background (non-blocking)
	go s.performUpdate(latestVersion)

	c.JSON(http.StatusOK, gin.H{
		"message":        "Update initiated - server will restart in a few seconds",
		"current_version": s.version,
		"new_version":     latestVersion,
	})
}

// performUpdate downloads and installs the latest release
func (s *Server) performUpdate(newVersion string) {
	s.logger.Info().Str("current_version", s.version).Str("new_version", newVersion).Msg("Starting server update")

	// Create update script
	updateScript := `#!/bin/bash
set -euo pipefail

# Log everything to a file
exec > >(tee /var/log/branchd-update.log) 2>&1

echo "=== Branchd Update Script Started at $(date) ==="

GITHUB_REPO="branchd-dev/branchd"
BRANCHD_ARCH="arm64"
BUNDLE_NAME="branchd-linux-${BRANCHD_ARCH}.tar.gz"
RELEASE_TAG="%s"
DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${RELEASE_TAG}/${BUNDLE_NAME}"
CHECKSUM_URL="https://github.com/${GITHUB_REPO}/releases/download/${RELEASE_TAG}/${BUNDLE_NAME}.sha256"

echo "Downloading Branchd ${RELEASE_TAG}..."
cd /tmp
curl -fsSL -o "${BUNDLE_NAME}" "${DOWNLOAD_URL}"
curl -fsSL -o "${BUNDLE_NAME}.sha256" "${CHECKSUM_URL}"

echo "Verifying checksum..."
if ! sha256sum -c "${BUNDLE_NAME}.sha256"; then
    echo "ERROR: Checksum verification failed!"
    rm -f "${BUNDLE_NAME}" "${BUNDLE_NAME}.sha256"
    exit 1
fi

echo "Extracting bundle..."
tar -xzf "${BUNDLE_NAME}"

# The bundle always extracts to branchd-{arch} format
BUNDLE_DIR="branchd-${BRANCHD_ARCH}"
echo "Using bundle directory: ${BUNDLE_DIR}"

if [ ! -d "${BUNDLE_DIR}" ]; then
    echo "ERROR: Bundle directory ${BUNDLE_DIR} not found!"
    echo "Contents of /tmp:"
    ls -la /tmp/ | grep branchd
    exit 1
fi

echo "Stopping services..."
systemctl stop branchd-server branchd-worker

echo "Installing binaries..."
install -m 755 "${BUNDLE_DIR}/server" /usr/local/bin/branchd-server
install -m 755 "${BUNDLE_DIR}/worker" /usr/local/bin/branchd-worker

echo "Installing web UI..."
rm -rf /var/www/branchd/*
cp -r "${BUNDLE_DIR}/web"/* /var/www/branchd/
chown -R caddy:caddy /var/www/branchd

echo "Restarting services..."
systemctl daemon-reload
systemctl start branchd-server branchd-worker
systemctl restart caddy

echo "Cleanup..."
cd /
rm -rf /tmp/branchd-* /tmp/"${BUNDLE_NAME}" /tmp/"${BUNDLE_NAME}.sha256"

echo "✓ Update complete to ${RELEASE_TAG} at $(date)"
`

	// Write script to /run directory
	// Cannot use /tmp or /var/tmp because the service has PrivateTmp=true
	// which creates private namespaces for both, making files inaccessible to systemd-run
	// /run is not affected by PrivateTmp and is the standard location for runtime files
	scriptContent := fmt.Sprintf(updateScript, newVersion)
	scriptPath := "/run/branchd-update.sh"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		s.logger.Error().Err(err).Msg("Failed to create update script")
		return
	}

	// Execute update script detached from this process so it survives server shutdown
	s.logger.Info().Msg("Executing update script...")
	// Use systemd-run to run the update script as a separate transient unit
	// This ensures the script continues running even after branchd-server is stopped
	// Use timestamp to create unique unit name to avoid conflicts
	unitName := fmt.Sprintf("branchd-update-%d", time.Now().Unix())
	cmd := exec.Command("systemd-run", "--unit="+unitName, "--no-block", "bash", scriptPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		s.logger.Error().Err(err).Str("output", string(output)).Msg("Failed to start update process")
	} else {
		s.logger.Info().Str("output", string(output)).Msg("Update process started successfully")
	}
}
