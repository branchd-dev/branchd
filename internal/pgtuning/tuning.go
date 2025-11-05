package pgtuning

import (
	"fmt"
	"math"

	"github.com/branchd-dev/branchd/internal/sysinfo"
)

// RestoreSettings contains optimized PostgreSQL settings for restore operations
type RestoreSettings struct {
	ParallelJobs int

	Fsync                         bool
	SynchronousCommit             bool
	FullPageWrites                bool
	Autovacuum                    bool
	MaintenanceWorkMem            string
	MaxWalSize                    string
	CheckpointTimeout             string
	WalBuffers                    string
	MaxParallelMaintenanceWorkers int
}

// CalculateOptimalSettings calculates optimal PostgreSQL settings based on system resources
func CalculateOptimalSettings(resources sysinfo.Resources) RestoreSettings {
	settings := RestoreSettings{
		Fsync:             false,
		SynchronousCommit: false,
		FullPageWrites:    false,
		Autovacuum:        false,
	}

	settings.ParallelJobs = int(math.Max(2, float64(resources.CPUCores/2)))

	memPerWorkerGB := (resources.TotalMemoryGB * 0.25) / float64(settings.ParallelJobs)
	if memPerWorkerGB > 2.0 {
		memPerWorkerGB = 1.0
	}
	if memPerWorkerGB < 0.25 {
		memPerWorkerGB = 0.25
	}
	settings.MaintenanceWorkMem = fmt.Sprintf("%.0fMB", memPerWorkerGB*1024)

	// Calculate max_wal_size based on available disk and restore size
	if resources.AvailableDiskGB > 50 {
		settings.MaxWalSize = "10GB"
	} else {
		settings.MaxWalSize = "3GB"
	}
	settings.CheckpointTimeout = "30min"

	settings.WalBuffers = "16MB"

	settings.MaxParallelMaintenanceWorkers = min(settings.ParallelJobs, 6)

	return settings
}

// GenerateAlterSystemSQL generates ALTER SYSTEM SET commands for PostgreSQL tuning
func (s RestoreSettings) GenerateAlterSystemSQL() []string {
	sql := []string{
		fmt.Sprintf("ALTER SYSTEM SET fsync = %t", s.Fsync),
		fmt.Sprintf("ALTER SYSTEM SET synchronous_commit = %t", s.SynchronousCommit),
		fmt.Sprintf("ALTER SYSTEM SET full_page_writes = %t", s.FullPageWrites),
		fmt.Sprintf("ALTER SYSTEM SET autovacuum = %t", s.Autovacuum),
		fmt.Sprintf("ALTER SYSTEM SET maintenance_work_mem = '%s'", s.MaintenanceWorkMem),
		fmt.Sprintf("ALTER SYSTEM SET max_wal_size = '%s'", s.MaxWalSize),
		fmt.Sprintf("ALTER SYSTEM SET checkpoint_timeout = '%s'", s.CheckpointTimeout),
		fmt.Sprintf("ALTER SYSTEM SET wal_buffers = '%s'", s.WalBuffers),
		fmt.Sprintf("ALTER SYSTEM SET max_parallel_maintenance_workers = %d", s.MaxParallelMaintenanceWorkers),
	}
	return sql
}

// GenerateResetSQL generates ALTER SYSTEM RESET commands to restore defaults
func GenerateResetSQL() []string {
	return []string{
		"ALTER SYSTEM RESET fsync",
		"ALTER SYSTEM RESET synchronous_commit",
		"ALTER SYSTEM RESET full_page_writes",
		"ALTER SYSTEM RESET autovacuum",
		"ALTER SYSTEM RESET maintenance_work_mem",
		"ALTER SYSTEM RESET max_wal_size",
		"ALTER SYSTEM RESET checkpoint_timeout",
		"ALTER SYSTEM RESET wal_buffers",
		"ALTER SYSTEM RESET max_parallel_maintenance_workers",
	}
}
