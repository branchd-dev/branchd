package tasks

import (
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

// Task type constants
const (
	// Database restore tasks (used for local execution)
	TypePgDumpRestoreExecute      = "pg_dump_restore:execute"
	TypePgDumpRestoreWaitComplete = "pg_dump_restore:wait_complete"
)

// TaskPayload is the common payload for all tasks
type TaskPayload struct {
	RestoreID string `json:"database_id,omitempty"`
}

// NewPgDumpRestoreExecuteTask creates a task to execute pg_dump/restore
func NewPgDumpRestoreExecuteTask(restoreID string) (*asynq.Task, error) {
	payload, err := json.Marshal(TaskPayload{
		RestoreID: restoreID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	return asynq.NewTask(TypePgDumpRestoreExecute, payload), nil
}

// NewPgDumpRestoreWaitCompleteTask creates a task to wait for pg_dump/restore completion
func NewPgDumpRestoreWaitCompleteTask(restoreID string) (*asynq.Task, error) {
	payload, err := json.Marshal(TaskPayload{
		RestoreID: restoreID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	return asynq.NewTask(TypePgDumpRestoreWaitComplete, payload), nil
}

// ParseTaskPayload parses task payload from Asynq task
func ParseTaskPayload(task *asynq.Task) (TaskPayload, error) {
	var payload TaskPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	return payload, nil
}
