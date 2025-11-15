package tasks

import (
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

// Task type constants
const (
	TypeTriggerRestore      = "restore:trigger"
	TypeRestoreWaitComplete = "restore:wait_complete"
)

// TaskPayload is the common payload for all tasks
type TaskPayload struct {
	RestoreID string `json:"database_id,omitempty"`
}

// NewTriggerRestoreTask creates a task to trigger a database restore
func NewTriggerRestoreTask(restoreID string) (*asynq.Task, error) {
	payload, err := json.Marshal(TaskPayload{
		RestoreID: restoreID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	return asynq.NewTask(TypeTriggerRestore, payload), nil
}

// NewTriggerRestoreWaitCompleteTask creates a task to wait for restore completion
func NewTriggerRestoreWaitCompleteTask(restoreID string) (*asynq.Task, error) {
	payload, err := json.Marshal(TaskPayload{
		RestoreID: restoreID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	return asynq.NewTask(TypeRestoreWaitComplete, payload), nil
}

// ParseTaskPayload parses task payload from Asynq task
func ParseTaskPayload(task *asynq.Task) (TaskPayload, error) {
	var payload TaskPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	return payload, nil
}
