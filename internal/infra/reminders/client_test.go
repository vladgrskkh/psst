//nolint:testpackage // white-box tests: newTestClient relies on unexported newClientAt
package reminders

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vladgrskkh/psst/internal/service"
	"github.com/vladgrskkh/psst/pkg/option"
)

// fakeBridgePath is built once in TestMain and read by newTestClient.
var fakeBridgePath string //nolint:gochecknoglobals // test scaffolding shared between TestMain and newTestClient

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakebridge")
	if err != nil {
		panic(err)
	}
	fakeBridgePath = filepath.Join(dir, "fakebridge")
	cmd := exec.Command("go", "build", "-o", fakeBridgePath, "./fakebridge")
	out, err := cmd.CombinedOutput()
	if err != nil {
		panic(string(out))
	}
	code := m.Run()
	err = os.RemoveAll(dir)
	if err != nil {
		panic(err)
	}
	os.Exit(code)
}

func newTestClient(t *testing.T, script map[string]any) *Client {
	t.Helper()
	data, err := json.Marshal(script)
	if err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(t.TempDir(), "script.json")
	err = os.WriteFile(scriptPath, data, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_BRIDGE_SCRIPT", scriptPath)
	return newClientAt(fakeBridgePath, 15*time.Second, slog.New(slog.DiscardHandler))
}

//nolint:paralleltest // uses t.Setenv via newTestClient, cannot run in parallel
func TestPermissions(t *testing.T) {
	c := newTestClient(t, map[string]any{"permissions": map[string]any{"status": statusAuthorized}})
	got, err := c.Permissions()
	if err != nil {
		t.Fatal(err)
	}
	if got != statusAuthorized {
		t.Fatalf("got %q; want authorized", got)
	}
}

//nolint:paralleltest // uses t.Setenv via newTestClient, cannot run in parallel
func TestListLists(t *testing.T) {
	c := newTestClient(t, map[string]any{"list-lists": map[string]any{
		"lists": []map[string]any{{"id": "l1", "name": "psst"}, {"id": "l2", "name": "work"}},
	}})
	lists, err := c.ListLists()
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 2 || lists[0].ID != "l1" || lists[1].Name != "work" {
		t.Fatalf("unexpected lists: %+v", lists)
	}
}

//nolint:paralleltest // uses t.Setenv via newTestClient, cannot run in parallel
func TestPull(t *testing.T) {
	c := newTestClient(t, map[string]any{"pull": map[string]any{
		"reminders": []map[string]any{{
			"id": "r1", "listID": "l1", "title": "buy milk", "notes": "oat",
			"dueDate": "2026-08-20T14:00:00+02:00", "completed": false, "completionDate": nil,
		}},
	}})
	reminders, err := c.Pull([]string{"l1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(reminders) != 1 || reminders[0].Title != "buy milk" {
		t.Fatalf("unexpected reminders: %+v", reminders)
	}
	if !reminders[0].DueDate.IsPresent() ||
		reminders[0].DueDate.ValueOr(time.Time{}).Hour() != 14 {
		t.Fatalf("dueDate parse failed: %+v", reminders[0].DueDate)
	}
	if reminders[0].CompletionDate.IsPresent() {
		t.Fatal("completionDate should be None")
	}
}

//nolint:paralleltest // uses t.Setenv via newTestClient, cannot run in parallel
func TestPullReturnsMalformedTimestampError(t *testing.T) {
	c := newTestClient(t, map[string]any{"pull": map[string]any{
		"reminders": []map[string]any{{
			"id": "r1", "listID": "l1", "title": "buy milk", "dueDate": "not-a-time",
		}},
	}})
	if _, err := c.Pull([]string{"l1"}); err == nil {
		t.Fatal("Pull() succeeded with an invalid dueDate")
	}
}

func TestUpdateWireFieldsDistinguishUnsetClearAndFalse(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(updateParams{
		ID: "r1",
		Fields: updateFields{
			Title:     option.Some("new title"),
			DueDate:   option.Some(option.None[time.Time]()),
			Completed: option.Some(false),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(
		data,
	), `{"id":"r1","fields":{"title":"new title","dueDate":null,"completed":false}}`; got != want {
		t.Fatalf("update request = %s; want %s", got, want)
	}
}

//nolint:paralleltest // uses t.Setenv via newTestClient, cannot run in parallel
func TestCreate(t *testing.T) {
	c := newTestClient(t, map[string]any{"create": map[string]any{"id": "r-new"}})
	id, err := c.Create(service.CreateReminderParams{ListID: "l1", Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if id != "r-new" {
		t.Fatalf("got %q; want r-new", id)
	}
}

//nolint:paralleltest // uses t.Setenv via newTestClient, cannot run in parallel
func TestAccessDenied(t *testing.T) {
	script := map[string]any{"permissions": map[string]any{
		"error": map[string]any{"code": "access-denied", "message": "no"},
	}}
	c := newTestClient(t, script)
	_, err := c.Permissions()
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("err = %v; want ErrAccessDenied", err)
	}
}

//nolint:paralleltest // uses t.Setenv via newTestClient, cannot run in parallel
func TestReminderNotFound(t *testing.T) {
	script := map[string]any{"update": map[string]any{
		"error": map[string]any{"code": "not-found", "message": "no such reminder"},
	}}
	c := newTestClient(t, script)
	err := c.Update(service.UpdateReminderParams{ID: "nope"})
	if !errors.Is(err, service.ErrReminderNotFound) {
		t.Fatalf("err = %v; want service.ErrReminderNotFound", err)
	}
}

func TestBridgeNotFound(t *testing.T) {
	t.Parallel()
	c := newClientAt(filepath.Join(t.TempDir(), "missing-bridge"), 2*time.Second, slog.New(slog.DiscardHandler))
	_, err := c.Permissions()
	if !errors.Is(err, ErrBridgeNotFound) {
		t.Fatalf("err = %v; want ErrBridgeNotFound", err)
	}
}

func TestCallReturnsBridgeProcessFailure(t *testing.T) {
	t.Setenv("FAKE_BRIDGE_EXIT_CODE", "1")
	c := newTestClient(t, map[string]any{"permissions": map[string]any{"status": statusAuthorized}})
	_, err := c.Permissions()
	if err == nil || !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("Permissions() error = %v; want bridge exit failure", err)
	}
}
