package domain

import (
	"time"

	"github.com/vladgrskkh/psst/pkg/option"
)

// SyncState records the last successful pull and push times.
type SyncState struct {
	LastPullAt time.Time `json:"last_pull_at"`
	LastPushAt time.Time `json:"last_push_at"`
}

// TodoSnapshot contains the fields reconciled with a remote reminder.
type TodoSnapshot struct {
	Title       string                   `json:"title"`
	Description string                   `json:"notes"`
	Deadline    option.Option[time.Time] `json:"dueDate"`
	ListID      string                   `json:"listID"`
	Done        bool                     `json:"completed"`
	CompletedAt option.Option[time.Time] `json:"completionDate"`
	Deleted     bool                     `json:"deleted"`
}
