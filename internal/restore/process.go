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

// ProcessManager handles process lifecycle for restore operations
// It manages PID files, checks process status, and reads restore logs
type ProcessManager struct {
	logger zerolog.Logger
}

// NewProcessManager creates a new process manager
func NewProcessManager(logger zerolog.Logger) *ProcessManager {
	return &ProcessManager{
		logger: logger,
	}
}

// ValidateInputs validates all required parameters for a restore
func (p *ProcessManager) ValidateInputs(
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
	if strings.ContainsAny(databaseName, "\"';\\\n\r") {
		return fmt.Errorf("database name contains invalid characters")
	}

	return nil
}

// CreateLogDirectory creates the log directory with proper permissions
func (p *ProcessManager) CreateLogDirectory(ctx context.Context) error {
	p.logger.Info().Msg("Creating log directory")

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

// CheckIfRunning checks if a restore process is already running for the given restore name
// Returns (isRunning, pid, error)
func (p *ProcessManager) CheckIfRunning(ctx context.Context, restoreName string) (bool, int, error) {
	pidFile := p.GetPIDFilePath(restoreName)

	p.logger.Debug().
		Str("restore_name", restoreName).
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
		p.logger.Warn().Msg("PID file is empty, will clean up")
		_ = os.Remove(pidFile)
		return false, 0, nil
	}

	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		p.logger.Warn().Err(err).Msg("Invalid PID in file, will clean up")
		_ = os.Remove(pidFile)
		return false, 0, nil
	}

	// Check if process is running using kill -0
	checkCmd := exec.CommandContext(ctx, "kill", "-0", strconv.Itoa(pid))
	if err := checkCmd.Run(); err != nil {
		// Process is not running
		p.logger.Info().
			Int("pid", pid).
			Msg("Found stale PID file, will clean up")
		_ = os.Remove(pidFile)
		return false, 0, nil
	}

	// Process is running
	p.logger.Info().
		Int("pid", pid).
		Str("restore_name", restoreName).
		Msg("Restore process is already running")

	return true, pid, nil
}

// GetLogFilePath returns the path to the restore log file
func (p *ProcessManager) GetLogFilePath(restoreName string) string {
	return fmt.Sprintf("%s/restore-%s.log", RestoreLogDir, restoreName)
}

// GetPIDFilePath returns the path to the restore PID file
func (p *ProcessManager) GetPIDFilePath(restoreName string) string {
	return fmt.Sprintf("%s/restore-%s.pid", RestoreLogDir, restoreName)
}

// CleanupPIDFile removes the PID file for a restore
func (p *ProcessManager) CleanupPIDFile(restoreName string) error {
	pidFile := p.GetPIDFilePath(restoreName)
	if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove PID file: %w", err)
	}
	return nil
}

// CheckStatus checks the status of a restore by reading markers from log file
// Returns: (status, logTail, error)
func (p *ProcessManager) CheckStatus(ctx context.Context, restoreName string) (Status, string, error) {
	logFile := p.GetLogFilePath(restoreName)

	// Check if log file exists
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		return StatusNotFound, "", nil
	}

	// Check for success marker
	successCmd := exec.CommandContext(ctx, "grep", "-q", "__BRANCHD_RESTORE_SUCCESS__", logFile)
	if err := successCmd.Run(); err == nil {
		p.logger.Debug().
			Str("restore_name", restoreName).
			Msg("Found success marker in restore log")
		return StatusSuccess, "", nil
	}

	// Check for failure marker
	failureCmd := exec.CommandContext(ctx, "grep", "-q", "__BRANCHD_RESTORE_FAILED__", logFile)
	if err := failureCmd.Run(); err == nil {
		p.logger.Debug().
			Str("restore_name", restoreName).
			Msg("Found failure marker in restore log")

		// Read log tail for failure case
		logTail, err := p.ReadLogTail(ctx, restoreName, 50)
		if err != nil {
			p.logger.Warn().Err(err).Msg("Failed to read log tail")
			return StatusFailed, "Failed to read log", nil
		}
		return StatusFailed, logTail, nil
	}

	// No markers found - restore is still running or status unknown
	p.logger.Debug().
		Str("restore_name", restoreName).
		Msg("No success/failure marker found in restore log")

	logTail, err := p.ReadLogTail(ctx, restoreName, 50)
	if err != nil {
		p.logger.Warn().Err(err).Msg("Failed to read log tail")
		return StatusUnknown, "", nil
	}

	return StatusUnknown, logTail, nil
}

// ReadLogTail reads the last N lines from a restore log file
func (p *ProcessManager) ReadLogTail(ctx context.Context, restoreName string, lines int) (string, error) {
	logFile := p.GetLogFilePath(restoreName)

	cmd := exec.CommandContext(ctx, "tail", "-n", strconv.Itoa(lines), logFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to read log tail: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// KillProcess kills a restore process if it's running
func (p *ProcessManager) KillProcess(ctx context.Context, restoreName string) error {
	pidFile := p.GetPIDFilePath(restoreName)
	logFile := p.GetLogFilePath(restoreName)

	// Check if PID file exists and process is running
	checkCmd := fmt.Sprintf(`
		if [ -f %s ]; then
			pid=$(cat %s)
			if kill -0 $pid 2>/dev/null; then
				echo "running:$pid"
			else
				echo "stopped"
			fi
		else
			echo "not_found"
		fi
	`, pidFile, pidFile)

	cmd := exec.CommandContext(ctx, "bash", "-c", checkCmd)
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		p.logger.Warn().
			Err(err).
			Str("restore_name", restoreName).
			Msg("Failed to check restore process status")
	}

	output := strings.TrimSpace(string(outputBytes))

	// If process is running, kill it
	if strings.HasPrefix(output, "running:") {
		pid := strings.TrimPrefix(output, "running:")

		// Kill the process (SIGTERM first, then SIGKILL if needed)
		killCmd := fmt.Sprintf(`
			pid=%s
			if kill -0 $pid 2>/dev/null; then
				kill -TERM $pid 2>/dev/null || true
				sleep 1
				if kill -0 $pid 2>/dev/null; then
					kill -KILL $pid 2>/dev/null || true
				fi
			fi
		`, pid)

		killExecCmd := exec.CommandContext(ctx, "bash", "-c", killCmd)
		if killOutput, killErr := killExecCmd.CombinedOutput(); killErr != nil {
			p.logger.Warn().
				Err(killErr).
				Str("output", string(killOutput)).
				Str("pid", pid).
				Msg("Failed to kill restore process")
		} else {
			p.logger.Info().
				Str("restore_name", restoreName).
				Str("pid", pid).
				Msg("Restore process killed successfully")
		}
	}

	// Clean up PID and log files
	cleanupCmd := fmt.Sprintf("rm -f %s %s", pidFile, logFile)
	cleanupExecCmd := exec.CommandContext(ctx, "bash", "-c", cleanupCmd)
	if cleanupOutput, cleanupErr := cleanupExecCmd.CombinedOutput(); cleanupErr != nil {
		p.logger.Warn().
			Err(cleanupErr).
			Str("output", string(cleanupOutput)).
			Msg("Failed to clean up restore files")
	} else {
		p.logger.Debug().
			Str("restore_name", restoreName).
			Msg("Restore PID and log files cleaned up")
	}

	return nil
}
