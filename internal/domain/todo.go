package domain

import (
	"time"

	"github.com/google/uuid"

	"github.com/vladgrskkh/psst/pkg/option"
)

type Todo struct {
	ID             string                   `json:"id"`
	ReminderID     option.Option[string]    `json:"reminder_id"`
	ListID         option.Option[string]    `json:"list_id"`
	ListName       string                   `json:"list_name"`
	Title          string                   `json:"title"`
	Description    string                   `json:"description"`
	Deadline       option.Option[time.Time] `json:"deadline"`
	Done           bool                     `json:"done"`
	CompletedAt    option.Option[time.Time] `json:"completed_at"`
	CreatedAt      time.Time                `json:"created_at"`
	UpdatedAt      time.Time                `json:"updated_at"`
	Deleted        bool                     `json:"deleted"`
	DeletedAt      option.Option[time.Time] `json:"deleted_at"`
	RemoteSnapshot string                   `json:"remote_snapshot"`
	LocalSnapshot  string                   `json:"local_snapshot"`
}

func NewTodo(title string) *Todo {
	return &Todo{
		ID:    uuid.NewString(),
		Title: title,
	}
}
