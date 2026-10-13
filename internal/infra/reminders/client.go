// Package reminders talks to the psst-bridge Swift binary (EventKit) over
// line-delimited JSON on stdin/stdout.
package reminders

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vladgrskkh/psst/internal/service"
	"github.com/vladgrskkh/psst/pkg/option"
)

const (
	defaultTimeout = 15 * time.Second

	statusAuthorized = "authorized"
)

var (
	ErrBridgeNotFound = errors.New("psst-bridge binary not found — run `make build/bridge`")
	ErrAccessDenied   = errors.New(
		"reminders access denied — enable in System Settings → Privacy & Security → Reminders",
	)
	ErrBridgeProtocol = errors.New("unexpected bridge response")
)

type Client struct {
	binPath string
	timeout time.Duration
	logger  *slog.Logger
}

// NewClient resolves the bridge binary next to the psst executable.
func NewClient(logger *slog.Logger) *Client {
	exe, err := os.Executable()
	if err != nil {
		exe = "."
	}
	return newClientAt(filepath.Join(filepath.Dir(exe), "psst-bridge"), defaultTimeout, logger)
}

func newClientAt(binPath string, timeout time.Duration, logger *slog.Logger) *Client {
	return &Client{binPath: binPath, timeout: timeout, logger: logger}
}

// call runs one request against a fresh bridge process.
func (c *Client) call(method string, params any) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	c.logger.Debug("bridge call", slog.String("method", method))

	cmd := exec.CommandContext( //nolint:gosec // binPath is set by NewClient/newClientAt, never from user input
		ctx,
		c.binPath,
	)
	request, err := json.Marshal(bridgeRequest{ID: requestID, Method: method, Params: params})
	if err != nil {
		return nil, fmt.Errorf("encode bridge request: %w", err)
	}
	request = append(request, '\n')
	cmd.Stdin = bytes.NewReader(request)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("bridge call: %w", ctx.Err())
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("bridge process failed: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("%w: %w", ErrBridgeNotFound, err)
	}

	var resp bridgeResponse
	if decodeErr := json.Unmarshal(stdout, &resp); decodeErr != nil {
		return nil, fmt.Errorf("%w: decode response: %w", ErrBridgeProtocol, decodeErr)
	}
	if resp.ID != requestID {
		return nil, fmt.Errorf("%w: id mismatch", ErrBridgeProtocol)
	}
	if resp.Error != nil {
		return nil, mapBridgeError(resp.Error)
	}
	return resp.Result, nil
}

func mapBridgeError(e *bridgeError) error {
	switch e.Code {
	case "access-denied":
		return ErrAccessDenied
	case "not-found":
		// This provider-specific response is translated to the gateway contract
		// consumed by SyncEngine.
		return service.ErrReminderNotFound
	default:
		return fmt.Errorf("bridge error (%s): %s", e.Code, e.Message)
	}
}

func (c *Client) Permissions() (string, error) {
	raw, err := c.call(methodPermissions, nil)
	if err != nil {
		return "", err
	}
	var out permissionsResult
	err = json.Unmarshal(raw, &out)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrBridgeProtocol, err)
	}
	switch out.Status {
	case statusAuthorized, "denied", "not-determined":
		return out.Status, nil
	}
	return "", fmt.Errorf("%w: unknown status %q", ErrBridgeProtocol, out.Status)
}

func (c *Client) ListLists() ([]service.ReminderList, error) {
	raw, err := c.call(methodListLists, nil)
	if err != nil {
		return nil, err
	}
	var out listListsResult
	err = json.Unmarshal(raw, &out)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBridgeProtocol, err)
	}
	lists := make([]service.ReminderList, 0, len(out.Lists))
	for _, l := range out.Lists {
		lists = append(lists, service.ReminderList{ID: l.ID, Name: l.Name})
	}
	return lists, nil
}

func (c *Client) EnsureList(name string) (service.ReminderList, error) {
	raw, err := c.call(methodEnsureList, ensureListParams{Name: name})
	if err != nil {
		return service.ReminderList{}, err
	}
	var out ensureListResult
	err = json.Unmarshal(raw, &out)
	if err != nil {
		return service.ReminderList{}, fmt.Errorf("%w: %w", ErrBridgeProtocol, err)
	}
	return service.ReminderList{ID: out.ID, Name: name}, nil
}

func (c *Client) Pull(listIDs []string) ([]service.RemoteReminder, error) {
	raw, err := c.call(methodPull, pullParams{ListIDs: listIDs})
	if err != nil {
		return nil, err
	}
	var out pullResult
	err = json.Unmarshal(raw, &out)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBridgeProtocol, err)
	}
	reminders := make([]service.RemoteReminder, 0, len(out.Reminders))
	for _, r := range out.Reminders {
		dueDate, parseErr := parseOptTime(r.DueDate)
		if parseErr != nil {
			return nil, fmt.Errorf("reminder %q dueDate: %w", r.ID, parseErr)
		}
		completionDate, parseErr := parseOptTime(r.CompletionDate)
		if parseErr != nil {
			return nil, fmt.Errorf("reminder %q completionDate: %w", r.ID, parseErr)
		}
		reminders = append(reminders, service.RemoteReminder{
			ID:             r.ID,
			ListID:         r.ListID,
			Title:          r.Title,
			Notes:          r.Notes,
			DueDate:        dueDate,
			Completed:      r.Completed,
			CompletionDate: completionDate,
		})
	}
	return reminders, nil
}

func (c *Client) Create(p service.CreateReminderParams) (string, error) {
	raw, err := c.call(methodCreate, createParams{
		ListID: p.ListID, Title: p.Title, Notes: p.Notes, DueDate: p.DueDate,
	})
	if err != nil {
		return "", err
	}
	var out createResult
	err = json.Unmarshal(raw, &out)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrBridgeProtocol, err)
	}
	return out.ID, nil
}

func (c *Client) Update(p service.UpdateReminderParams) error {
	_, err := c.call(methodUpdate, updateParams{
		ID: p.ID,
		Fields: updateFields{
			Title: p.Title, Notes: p.Notes, ListID: p.ListID,
			DueDate: p.DueDate, Completed: p.Completed, CompletionDate: p.CompletionDate,
		},
	})
	return err
}

func (c *Client) Delete(id string) error {
	_, err := c.call(methodDelete, reminderIDParams{ID: id})
	return err
}

func parseOptTime(s option.Option[string]) (option.Option[time.Time], error) {
	value, ok := s.Get()
	if !ok {
		return option.None[time.Time](), nil
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return option.None[time.Time](), err
	}
	return option.Some(t), nil
}
