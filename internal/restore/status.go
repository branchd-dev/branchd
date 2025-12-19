package restore

// Status represents the current state of a restore operation
type Status string

const (
	// StatusPending indicates the restore has not started yet
	StatusPending Status = "pending"

	// StatusRunning indicates the restore is currently in progress
	StatusRunning Status = "running"

	// StatusSuccess indicates the restore completed successfully
	StatusSuccess Status = "success"

	// StatusFailed indicates the restore failed
	StatusFailed Status = "failed"

	// StatusNotFound indicates no restore log was found
	StatusNotFound Status = "not_found"

	// StatusUnknown indicates the restore status could not be determined
	StatusUnknown Status = "unknown"
)

// IsTerminal returns true if the status represents a final state
func (s Status) IsTerminal() bool {
	return s == StatusSuccess || s == StatusFailed
}

// IsRunning returns true if the restore is still in progress
func (s Status) IsRunning() bool {
	return s == StatusRunning || s == StatusPending
}
