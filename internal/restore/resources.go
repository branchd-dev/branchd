package restore

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/rs/zerolog"
)

// ResourceManager handles system resources for restore operations
// This includes port allocation, ZFS dataset management, and systemd services
type ResourceManager struct {
	logger zerolog.Logger
}

// NewResourceManager creates a new resource manager
func NewResourceManager(logger zerolog.Logger) *ResourceManager {
	return &ResourceManager{
		logger: logger,
	}
}

// FindAvailablePort finds an available port in the range 50000-60000 for a new restore cluster
func (r *ResourceManager) FindAvailablePort(ctx context.Context) (int, error) {
	for port := 50000; port < 60000; port++ {
		cmd := exec.CommandContext(ctx, "bash", "-c", fmt.Sprintf("ss -ln | grep -q ':%d ' && echo 'in_use' || echo 'available'", port))
		output, err := cmd.Output()
		if err != nil {
			continue
		}

		if strings.TrimSpace(string(output)) == "available" {
			r.logger.Debug().Int("port", port).Msg("Found available port")
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available ports in range 50000-60000")
}

// StopSystemdService stops and disables a systemd service
func (r *ResourceManager) StopSystemdService(ctx context.Context, serviceName string) error {
	r.logger.Info().Str("service", serviceName).Msg("Stopping PostgreSQL service")

	// Stop service
	stopCmd := fmt.Sprintf("sudo systemctl stop %s || true", serviceName)
	cmd := exec.CommandContext(ctx, "bash", "-c", stopCmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		r.logger.Warn().
			Err(err).
			Str("output", string(output)).
			Msg("Failed to stop service (continuing anyway)")
	}

	// Disable service
	disableCmd := fmt.Sprintf("sudo systemctl disable %s || true", serviceName)
	cmd = exec.CommandContext(ctx, "bash", "-c", disableCmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		r.logger.Warn().
			Err(err).
			Str("output", string(output)).
			Msg("Failed to disable service (continuing anyway)")
	}

	return nil
}

// RemoveSystemdService removes a systemd service file and reloads the daemon
func (r *ResourceManager) RemoveSystemdService(ctx context.Context, serviceName string) error {
	r.logger.Info().Msg("Removing systemd service file")

	serviceFile := fmt.Sprintf("/etc/systemd/system/%s.service", serviceName)
	removeServiceCmd := fmt.Sprintf("sudo rm -f %s && sudo systemctl daemon-reload", serviceFile)
	cmd := exec.CommandContext(ctx, "bash", "-c", removeServiceCmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		r.logger.Warn().
			Err(err).
			Str("output", string(output)).
			Msg("Failed to remove service file (continuing anyway)")
	}

	return nil
}

// DestroyZFSDataset destroys a ZFS dataset and all its children
func (r *ResourceManager) DestroyZFSDataset(ctx context.Context, datasetName string) error {
	r.logger.Info().Str("zfs_dataset", datasetName).Msg("Destroying ZFS dataset")

	destroyCmd := fmt.Sprintf("sudo zfs destroy -r %s", datasetName)
	cmd := exec.CommandContext(ctx, "bash", "-c", destroyCmd)
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	if err != nil {
		r.logger.Error().
			Err(err).
			Str("zfs_dataset", datasetName).
			Str("output", output).
			Msg("Failed to destroy ZFS dataset")
		return fmt.Errorf("failed to destroy ZFS dataset: %w", err)
	}

	r.logger.Info().
		Str("zfs_dataset", datasetName).
		Msg("ZFS dataset destroyed successfully")

	return nil
}

// KillProcessesInDirectory kills any processes that have files open in the given directory
func (r *ResourceManager) KillProcessesInDirectory(ctx context.Context, directory string) error {
	r.logger.Info().Str("directory", directory).Msg("Killing any remaining processes")

	killCmd := fmt.Sprintf(`
		pids=$(sudo lsof -t +D %s 2>/dev/null || true)
		if [ -n "$pids" ]; then
			echo "Killing processes: $pids"
			sudo kill -TERM $pids 2>/dev/null || true
			sleep 2
			# Check again and use SIGKILL if needed
			pids=$(sudo lsof -t +D %s 2>/dev/null || true)
			if [ -n "$pids" ]; then
				sudo kill -9 $pids 2>/dev/null || true
			fi
		fi
	`, directory, directory)

	cmd := exec.CommandContext(ctx, "bash", "-c", killCmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		r.logger.Warn().
			Err(err).
			Str("output", string(output)).
			Msg("Failed to kill remaining processes (continuing anyway)")
	}

	return nil
}

// CleanupRestore performs full cleanup of a restore's resources
// This includes: killing processes, stopping systemd, destroying ZFS
func (r *ResourceManager) CleanupRestore(ctx context.Context, restoreName string, processManager *ProcessManager) error {
	serviceName := fmt.Sprintf("branchd-restore-%s", restoreName)
	zfsDataset := fmt.Sprintf("tank/%s", restoreName)
	dataDir := fmt.Sprintf("/opt/branchd/%s/data", restoreName)

	// 1. Kill any active restore process (via PID file)
	if err := processManager.KillProcess(ctx, restoreName); err != nil {
		r.logger.Warn().Err(err).Msg("Failed to kill restore process (continuing)")
	}

	// 2. Stop and disable systemd service
	if err := r.StopSystemdService(ctx, serviceName); err != nil {
		r.logger.Warn().Err(err).Msg("Failed to stop systemd service (continuing)")
	}

	// 3. Remove systemd service file
	if err := r.RemoveSystemdService(ctx, serviceName); err != nil {
		r.logger.Warn().Err(err).Msg("Failed to remove systemd service (continuing)")
	}

	// 4. Kill any remaining PostgreSQL processes using the data directory
	if err := r.KillProcessesInDirectory(ctx, dataDir); err != nil {
		r.logger.Warn().Err(err).Msg("Failed to kill remaining processes (continuing)")
	}

	// 5. Destroy ZFS dataset
	if err := r.DestroyZFSDataset(ctx, zfsDataset); err != nil {
		return fmt.Errorf("failed to destroy ZFS dataset: %w", err)
	}

	return nil
}

// GetServiceName returns the systemd service name for a restore
func GetServiceName(restoreName string) string {
	return fmt.Sprintf("branchd-restore-%s", restoreName)
}

// GetZFSDatasetName returns the ZFS dataset name for a restore
func GetZFSDatasetName(restoreName string) string {
	return fmt.Sprintf("tank/%s", restoreName)
}

// GetDataDirectory returns the PostgreSQL data directory path for a restore
func GetDataDirectory(restoreName string) string {
	return fmt.Sprintf("/opt/branchd/%s/data", restoreName)
}

// GetRestoreDataPath returns the base path for a restore's data
func GetRestoreDataPath(restoreName string) string {
	return fmt.Sprintf("/opt/branchd/%s", restoreName)
}
