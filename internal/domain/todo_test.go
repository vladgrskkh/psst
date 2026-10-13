package domain_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/vladgrskkh/psst/internal/domain"
	"github.com/vladgrskkh/psst/pkg/option"
)

func TestNewTodo(t *testing.T) {
	t.Parallel()

	todo := domain.NewTodo("buy milk")
	if todo.ID == "" {
		t.Fatal("ID must be set")
	}
	if todo.Title != "buy milk" {
		t.Fatalf("Title = %q; want %q", todo.Title, "buy milk")
	}
	if todo.ReminderID.IsPresent() || todo.Deadline.IsPresent() {
		t.Fatal("new todo must not have ReminderID or Deadline")
	}
}

func TestTodoJSONUsesSnakeCaseKeys(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(domain.NewTodo("buy milk"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, `"reminder_id"`) || strings.Contains(got, `"ReminderID"`) {
		t.Fatalf("Todo JSON keys are not snake_case: %s", got)
	}
}

func TestTodoJSONRoundTrip(t *testing.T) {
	t.Parallel()

	in := domain.NewTodo("buy milk")
	in.ReminderID = option.Some("rem-1")
	in.Deadline = option.Some(time.Date(2026, 8, 20, 14, 0, 0, 0, time.UTC))
	in.CompletedAt = option.Some(time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC))
	in.CreatedAt = time.Now()
	in.UpdatedAt = time.Now()
	in.Deleted = true
	in.DeletedAt = option.Some(time.Now())
	in.RemoteSnapshot = `{"title":"x"}`
	in.LocalSnapshot = `{"title":"x"}`

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out domain.Todo
	if unmarshalErr := json.Unmarshal(data, &out); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	if out.Title != in.Title || out.ReminderID.ValueOr("") != "rem-1" ||
		!out.Deadline.IsPresent() || !out.CompletedAt.IsPresent() ||
		!out.Deleted || !out.DeletedAt.IsPresent() {
		t.Fatalf("round trip mismatch: %+v", out)
	}
}
