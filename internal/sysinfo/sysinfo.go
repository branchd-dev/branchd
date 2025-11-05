package sysinfo

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Resources represents system resources for restore optimization
type Resources struct {
	CPUCores        int
	TotalMemoryGB   float64
	AvailableDiskGB float64
}

// Metrics represents detailed VM metrics for API responses
type Metrics struct {
	CPUCount        int     `json:"cpu_count"`
	MemoryTotalGB   float64 `json:"memory_total_gb"`
	MemoryUsedGB    float64 `json:"memory_used_gb"`
	MemoryFreeGB    float64 `json:"memory_free_gb"`
	DiskTotalGB     float64 `json:"disk_total_gb"`
	DiskUsedGB      float64 `json:"disk_used_gb"`
	DiskAvailableGB float64 `json:"disk_available_gb"`
	DiskUsedPercent float64 `json:"disk_used_percent"`
}

// GetResources returns basic system resources for restore tuning
func GetResources() (Resources, error) {
	metrics, err := GetMetrics(context.Background())
	if err != nil {
		// Return defaults on error
		return Resources{
			CPUCores:        runtime.NumCPU(),
			TotalMemoryGB:   4.0,
			AvailableDiskGB: 10.0,
		}, err
	}

	return Resources{
		CPUCores:        metrics.CPUCount,
		TotalMemoryGB:   metrics.MemoryTotalGB,
		AvailableDiskGB: metrics.DiskAvailableGB,
	}, nil
}

// GetMetrics returns detailed system metrics
func GetMetrics(ctx context.Context) (Metrics, error) {
	metrics := Metrics{
		CPUCount: runtime.NumCPU(),
	}

	// Get memory info
	if err := getMemoryInfo(&metrics); err != nil {
		return metrics, fmt.Errorf("failed to get memory info: %w", err)
	}

	// Get disk info from ZFS pool
	if err := getZFSDiskInfo(ctx, &metrics); err != nil {
		return metrics, fmt.Errorf("failed to get disk info: %w", err)
	}

	return metrics, nil
}

// getMemoryInfo reads memory information from /proc/meminfo
func getMemoryInfo(metrics *Metrics) error {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return fmt.Errorf("failed to open /proc/meminfo: %w", err)
	}
	defer file.Close()

	var memTotal, memAvailable float64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		value, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			continue
		}

		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			memTotal = value / (1024 * 1024) // KB to GB
		case strings.HasPrefix(line, "MemAvailable:"):
			memAvailable = value / (1024 * 1024) // KB to GB
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading /proc/meminfo: %w", err)
	}

	metrics.MemoryTotalGB = memTotal
	metrics.MemoryFreeGB = memAvailable
	metrics.MemoryUsedGB = memTotal - memAvailable

	return nil
}

// getZFSDiskInfo retrieves disk information from ZFS pool "tank"
func getZFSDiskInfo(ctx context.Context, metrics *Metrics) error {
	// Get ZFS pool info: available and used space
	cmd := exec.CommandContext(ctx, "bash", "-c",
		"zfs list -H -o available,used -p tank | head -1")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get ZFS pool info: %w", err)
	}

	fields := strings.Fields(strings.TrimSpace(string(output)))
	if len(fields) < 2 {
		return fmt.Errorf("unexpected ZFS output format")
	}

	// Parse available space
	available, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return fmt.Errorf("failed to parse available space: %w", err)
	}
	metrics.DiskAvailableGB = available / (1024 * 1024 * 1024)

	// Parse used space
	used, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return fmt.Errorf("failed to parse used space: %w", err)
	}
	metrics.DiskUsedGB = used / (1024 * 1024 * 1024)

	// Calculate totals
	metrics.DiskTotalGB = metrics.DiskAvailableGB + metrics.DiskUsedGB
	if metrics.DiskTotalGB > 0 {
		metrics.DiskUsedPercent = (metrics.DiskUsedGB / metrics.DiskTotalGB) * 100
	}

	return nil
}
