package reminders

import (
	"encoding/json"
	"time"

	"github.com/vladgrskkh/psst/pkg/option"
)

const requestID = 1 // Each bridge process handles exactly one request.

const (
	methodPermissions = "permissions"
	methodListLists   = "list-lists"
	methodEnsureList  = "ensure-list"
	methodPull        = "pull"
	methodCreate      = "create"
	methodUpdate      = "update"
	methodDelete      = "delete"
)

type bridgeRequest struct {
	ID     int    `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type bridgeResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *bridgeError    `json:"error"`
}

type bridgeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ensureListParams struct {
	Name string `json:"name"`
}

type pullParams struct {
	ListIDs []string `json:"listIDs"`
}

type reminderIDParams struct {
	ID string `json:"id"`
}

type createParams struct {
	ListID  string                   `json:"listID"`
	Title   string                   `json:"title"`
	Notes   string                   `json:"notes"`
	DueDate option.Option[time.Time] `json:"dueDate"`
}

type updateParams struct {
	ID     string       `json:"id"`
	Fields updateFields `json:"fields"`
}

type updateFields struct {
	Title          option.Option[string]                   `json:"title,omitzero"`
	Notes          option.Option[string]                   `json:"notes,omitzero"`
	ListID         option.Option[string]                   `json:"listID,omitzero"`
	DueDate        option.Option[option.Option[time.Time]] `json:"dueDate,omitzero"`
	Completed      option.Option[bool]                     `json:"completed,omitzero"`
	CompletionDate option.Option[option.Option[time.Time]] `json:"completionDate,omitzero"`
}

type permissionsResult struct {
	Status string `json:"status"`
}

type bridgeList struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type listListsResult struct {
	Lists []bridgeList `json:"lists"`
}

type ensureListResult struct {
	ID string `json:"id"`
}

type bridgeReminder struct {
	ID             string                `json:"id"`
	ListID         string                `json:"listID"`
	Title          string                `json:"title"`
	Notes          string                `json:"notes"`
	DueDate        option.Option[string] `json:"dueDate"`
	Completed      bool                  `json:"completed"`
	CompletionDate option.Option[string] `json:"completionDate"`
}

type pullResult struct {
	Reminders []bridgeReminder `json:"reminders"`
}

type createResult struct {
	ID string `json:"id"`
}
