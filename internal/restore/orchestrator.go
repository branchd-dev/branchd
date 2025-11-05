package restore

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
)

const (
	RestoreLogDir = "/var/log/branchd"
)

// RestoreOrchestrator handles the setup and orchestration of restore operations
type RestoreOrchestrator struct {
	logger zerolog.Logger
}

// NewOrchestrator creates a new restore orchestrator
func NewOrchestrator(logger zerolog.Logger) *RestoreOrchestrator {
	return &RestoreOrchestrator{
		logger: logger,
	}
}

// ValidateInputs validates all required parameters for a restore
func (o *RestoreOrchestrator) ValidateInputs(
	connectionString string,
	pgVersion string,
	pgPort int,
	databaseName string,
) error {
	if connectionString == "" {
		return fmt.Errorf("connection string is required")
	}
	if pgVersion == "" {
		return fmt.Errorf("PostgreSQL version is required")
	}
	if pgPort <= 0 || pgPort > 65535 {
		return fmt.Errorf("invalid PostgreSQL port: %d (must be between 1-65535)", pgPort)
	}
	if databaseName == "" {
		return fmt.Errorf("database name is required")
	}

	// Validate database name doesn't contain dangerous characters
	if strings.ContainsAny(databaseName, "\"';\\") {
		return fmt.Errorf("database name contains invalid characters")
	}

	return nil
}

// CreateLogDirectory creates the log directory with proper permissions
func (o *RestoreOrchestrator) CreateLogDirectory(ctx context.Context) error {
	o.logger.Info().Msg("Creating log directory")

	// Create directory
	if err := os.MkdirAll(RestoreLogDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Change ownership to current user
	// Note: This requires the binary to run with appropriate permissions
	cmd := exec.CommandContext(ctx, "sudo", "chown", "-R",
		fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		RestoreLogDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to change log directory ownership: %w (output: %s)", err, string(output))
	}

	return nil
}

// CheckIfRestoreInProgress checks if a restore is already running for the given database
// Returns (isRunning, pid, error)
func (o *RestoreOrchestrator) CheckIfRestoreInProgress(ctx context.Context, databaseName string) (bool, int, error) {
	pidFile := o.GetPIDFilePath(databaseName)

	o.logger.Debug().
		Str("database_name", databaseName).
		Str("pid_file", pidFile).
		Msg("Checking if restore is already in progress")

	// Check if PID file exists
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		return false, 0, nil
	}

	// Read PID from file
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		return false, 0, fmt.Errorf("failed to read PID file: %w", err)
	}

	pidStr := strings.TrimSpace(string(pidBytes))
	if pidStr == "" {
		o.logger.Warn().Msg("PID file is empty, will clean up")
		_ = os.Remove(pidFile)
		return false, 0, nil
	}

	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		o.logger.Warn().Err(err).Msg("Invalid PID in file, will clean up")
		_ = os.Remove(pidFile)
		return false, 0, nil
	}

	// Check if process is running using kill -0
	checkCmd := exec.CommandContext(ctx, "kill", "-0", strconv.Itoa(pid))
	if err := checkCmd.Run(); err != nil {
		// Process is not running
		o.logger.Info().
			Int("pid", pid).
			Msg("Found stale PID file, will clean up")
		_ = os.Remove(pidFile)
		return false, 0, nil
	}

	// Process is running
	o.logger.Info().
		Int("pid", pid).
		Str("database_name", databaseName).
		Msg("Restore process is already running")

	return true, pid, nil
}

// GetLogFilePath returns the path to the restore log file
func (o *RestoreOrchestrator) GetLogFilePath(databaseName string) string {
	return fmt.Sprintf("%s/restore-%s.log", RestoreLogDir, databaseName)
}

// GetPIDFilePath returns the path to the restore PID file
func (o *RestoreOrchestrator) GetPIDFilePath(databaseName string) string {
	return fmt.Sprintf("%s/restore-%s.pid", RestoreLogDir, databaseName)
}

// CleanupPIDFile removes the PID file for a restore
func (o *RestoreOrchestrator) CleanupPIDFile(databaseName string) error {
	pidFile := o.GetPIDFilePath(databaseName)
	if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove PID file: %w", err)
	}
	return nil
}

// CheckRestoreStatus checks the status of a restore by reading markers from log file
// Returns: (status, logTail, error)
// Status can be: "success", "failed", "unknown", "not_found"
func (o *RestoreOrchestrator) CheckRestoreStatus(ctx context.Context, databaseName string) (string, string, error) {
	logFile := o.GetLogFilePath(databaseName)

	// Check if log file exists
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		return "not_found", "", nil
	}

	// Check for success marker
	successCmd := exec.CommandContext(ctx, "grep", "-q", "__BRANCHD_RESTORE_SUCCESS__", logFile)
	if err := successCmd.Run(); err == nil {
		o.logger.Debug().
			Str("database_name", databaseName).
			Msg("Found success marker in restore log")
		return "success", "", nil
	}

	// Check for failure marker
	failureCmd := exec.CommandContext(ctx, "grep", "-q", "__BRANCHD_RESTORE_FAILED__", logFile)
	if err := failureCmd.Run(); err == nil {
		o.logger.Debug().
			Str("database_name", databaseName).
			Msg("Found failure marker in restore log")

		// Read log tail for failure case
		logTail, err := o.ReadLogTail(ctx, databaseName, 50)
		if err != nil {
			o.logger.Warn().Err(err).Msg("Failed to read log tail")
			return "failed", "Failed to read log", nil
		}
		return "failed", logTail, nil
	}

	// No markers found - restore is still running or status unknown
	o.logger.Debug().
		Str("database_name", databaseName).
		Msg("No success/failure marker found in restore log")

	logTail, err := o.ReadLogTail(ctx, databaseName, 50)
	if err != nil {
		o.logger.Warn().Err(err).Msg("Failed to read log tail")
		return "unknown", "", nil
	}

	return "unknown", logTail, nil
}

// ReadLogTail reads the last N lines from a restore log file
func (o *RestoreOrchestrator) ReadLogTail(ctx context.Context, databaseName string, lines int) (string, error) {
	logFile := o.GetLogFilePath(databaseName)

	cmd := exec.CommandContext(ctx, "tail", "-n", strconv.Itoa(lines), logFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to read log tail: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}
