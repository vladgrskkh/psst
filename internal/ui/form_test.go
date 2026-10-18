package ui_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/vladgrskkh/psst/internal/config"
	"github.com/vladgrskkh/psst/internal/domain"
	"github.com/vladgrskkh/psst/internal/service"
	"github.com/vladgrskkh/psst/internal/ui"
	"github.com/vladgrskkh/psst/pkg/option"
)

func TestCreateFormCreatesThroughTodoServiceAndRefreshesSuggestions(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	store := &memoryTodos{}
	todos := service.NewTodoService(store, fixedClock{now})
	cfg := config.Default()
	root := openForm(t, ui.NewRoot(todos, todos, cfg.Theme, cfg.Keys, now, "personal", "09:00"))
	var cmd tea.Cmd
	for _, input := range []string{"Buy tea", "tomorrow 10:30", "with lemon", "work"} {
		if input == "work" {
			root, _ = root.Update(key("ctrl+a"))
			root, _ = root.Update(key("ctrl+k"))
		}
		root = typeText(root, input)
		root, _ = root.Update(key("tab"))
	}
	root, cmd = root.Update(key("enter"))
	if cmd == nil {
		t.Fatal("valid create did not return a deferred service command")
	}
	settle(t, root, cmd)
	created, err := todos.List("")
	if err != nil || len(created) != 1 {
		t.Fatalf("local create result = %v, %v; want one todo", created, err)
	}
	got := created[0]
	if got.Title != "Buy tea" || got.Description != "with lemon" || got.ListName != "work" {
		t.Fatalf("created todo = %#v", got)
	}
	if !got.CreatedAt.Equal(now) || !got.UpdatedAt.Equal(now) {
		t.Fatalf("create timestamps = %v / %v; want %v", got.CreatedAt, got.UpdatedAt, now)
	}
	deadline, ok := got.Deadline.Get()
	if !ok || !deadline.Equal(time.Date(2026, time.October, 19, 10, 30, 0, 0, time.UTC)) {
		t.Fatalf("deadline = %v, present=%v", deadline, ok)
	}
}

func TestNewFormListTypingReplacesDefaultAndBlankUsesDefault(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, typedList, wantList string
	}{
		{name: "typed list replaces default", typedList: "groceries", wantList: "groceries"},
		{name: "untouched list uses default", wantList: "psst"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &memoryTodos{}
			todos := service.NewTodoService(store, fixedClock{now})
			cfg := config.Default()
			root := openForm(t, ui.NewRoot(todos, todos, cfg.Theme, cfg.Keys, now, "psst", "09:00"))
			root = typeText(root, "Buy oats")
			for range 3 {
				root, _ = root.Update(key("tab"))
			}
			if tc.typedList == "" && !strings.Contains(ansi.Strip(root.View()), "List: psst") {
				t.Fatalf("blank list field did not show its default guidance: %s", ansi.Strip(root.View()))
			}
			root = typeText(root, tc.typedList)
			root, cmd := root.Update(key("enter"))
			if cmd == nil {
				t.Fatal("valid create did not submit")
			}
			settle(t, root, cmd)
			created, err := todos.List("")
			if err != nil || len(created) != 1 {
				t.Fatalf("create result = %d todos, %v", len(created), err)
			}
			if created[0].ListName != tc.wantList {
				t.Fatalf("saved ListName = %q; want %q", created[0].ListName, tc.wantList)
			}
		})
	}
}

func TestEditFormLoadsAndUpdatesSelectedTodoWithoutLosingFilter(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	store := &memoryTodos{items: []*domain.Todo{
		{
			ID:          "tea",
			Title:       "Buy tea",
			Description: "green",
			ListName:    "home",
			Deadline:    option.Some(now.Add(24 * time.Hour)),
		},
		{
			ID:       "bread",
			Title:    "Buy bread",
			ListName: "home",
			Deadline: option.Some(now.Add(time.Hour)),
		},
	}}
	todos := service.NewTodoService(store, fixedClock{now})
	cfg := config.Default()
	var root tea.Model = initializedRoot(t, todos, cfg, now)
	root, _ = root.Update(key("/"))
	root = typeText(root, "Buy")
	root, cmd := root.Update(key("enter"))
	if cmd == nil {
		t.Fatal("filter did not schedule load")
	}
	root = settle(t, root, cmd)
	root, _ = root.Update(key("j"))
	assertSelected(t, root, "Buy tea")
	assertSelected(t, root, "Buy tea")
	root, cmd = root.Update(key("e"))
	if cmd == nil {
		t.Fatalf("edit binding did not request selected todo: %s", ansi.Strip(root.View()))
	}
	root = settle(t, root, cmd)
	if !strings.Contains(ansi.Strip(root.View()), "Buy tea") ||
		!strings.Contains(ansi.Strip(root.View()), "green") {
		t.Fatalf("edit form was not initialized from selected todo: %s", ansi.Strip(root.View()))
	}
	root, _ = root.Update(key("tab"))
	root, _ = root.Update(key("tab"))
	root, _ = root.Update(key("tab"))
	root, _ = root.Update(key("ctrl+a"))
	root, _ = root.Update(key("ctrl+k"))
	root = typeText(root, "office")
	root, cmd = root.Update(key("enter"))
	if cmd == nil {
		t.Fatal("valid edit did not schedule update")
	}
	root, cmd = root.Update(cmd())
	if cmd == nil {
		t.Fatal("successful edit did not defer dashboard reload")
	}
	root = settle(t, root, cmd)
	updated, err := todos.List("")
	if err != nil {
		t.Fatal(err)
	}
	if updated[0].ListName != "office" ||
		!strings.Contains(ansi.Strip(root.View()), "Filter: Buy") {
		t.Fatalf(
			"edit lost update or dashboard filter: todo=%#v view=%s",
			updated[0],
			ansi.Strip(root.View()),
		)
	}
	if !updated[0].UpdatedAt.Equal(now) {
		t.Fatalf("edit timestamp = %v; want %v", updated[0].UpdatedAt, now)
	}
	assertSelected(t, root, "Buy tea")
}

func TestCreateFormCancelAndFailedWriteErrorSurviveCancel(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	store := &memoryTodos{putErr: errors.New("disk\n\x1b[31m unavailable")}
	todos := service.NewTodoService(store, fixedClock{now})
	cfg := config.Default()
	var root tea.Model = ui.NewRoot(todos, todos, cfg.Theme, cfg.Keys, now, "personal", "09:00")
	root, cmd := root.Update(key("n"))
	root = settle(t, root, cmd)
	root = typeText(root, "Task")
	root, _ = root.Update(key("tab"))
	root, _ = root.Update(key("tab"))
	root, _ = root.Update(key("tab"))
	root, _ = root.Update(key("ctrl+a"))
	root, _ = root.Update(key("ctrl+k"))
	root = typeText(root, "personal")
	root, cmd = root.Update(key("enter"))
	if cmd == nil {
		t.Fatal("valid form did not attempt write")
	}
	root = settle(t, root, cmd)
	if !strings.Contains(ansi.Strip(root.View()), "disk unavailable") {
		t.Fatalf("write error not safely shown: %s", ansi.Strip(root.View()))
	}
	root, cmd = root.Update(key("esc"))
	root = settle(t, root, cmd)
	root, cmd = root.Update(key("q"))
	if cmd != nil {
		_ = cmd()
	}
	if root.(ui.Root).Err() == nil || !strings.Contains(root.(ui.Root).Err().Error(), "disk") {
		t.Fatalf("unresolved write error lost after cancel/quit: %v", root.(ui.Root).Err())
	}
	if strings.ContainsAny(root.(ui.Root).Err().Error(), "\n\r\x1b") {
		t.Fatalf("user-visible root error was not sanitized: %q", root.(ui.Root).Err())
	}
}

func TestCreateFormCancelHasNoMutation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	store := &memoryTodos{}
	todos := service.NewTodoService(store, fixedClock{now})
	cfg := config.Default()
	var root tea.Model = ui.NewRoot(todos, todos, cfg.Theme, cfg.Keys, now, "personal", "09:00")
	root, cmd := root.Update(key("n"))
	settle(t, root, cmd)
	_, cancel := root.Update(key("esc"))
	settle(t, root, cancel)
	if got, _ := todos.List(""); len(got) != 0 {
		t.Fatalf("cancel wrote todos: %#v", got)
	}
}

func TestCreateFormLeavesInvalidFieldsVisibleWithoutWriting(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	store := &memoryTodos{}
	todos := service.NewTodoService(store, fixedClock{now})
	cfg := config.Default()
	var root tea.Model = ui.NewRoot(todos, todos, cfg.Theme, cfg.Keys, now, "personal", "09:00")
	root, cmd := root.Update(key("n"))
	root = settle(t, root, cmd)
	root, _ = root.Update(key("enter"))
	view := ansi.Strip(root.View())
	if !strings.Contains(view, "Title must not be empty") || strings.Contains(view, "Save") {
		t.Fatalf("invalid title should remain in the form with inline feedback: %s", view)
	}
	if got, _ := todos.List(""); len(got) != 0 {
		t.Fatalf("invalid create wrote todos: %#v", got)
	}
}

func TestEditFormBlankDeadlineClearsExistingDeadline(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	store := &memoryTodos{
		items: []*domain.Todo{
			{
				ID:       "tea",
				Title:    "Buy tea",
				ListName: "home",
				Deadline: option.Some(now.Add(24 * time.Hour)),
			},
		},
	}
	todos := service.NewTodoService(store, fixedClock{now})
	cfg := config.Default()
	var root tea.Model = initializedRoot(t, todos, cfg, now)
	root, cmd := root.Update(key("e"))
	if cmd == nil {
		t.Fatal("edit did not schedule todo load")
	}
	root = settle(t, root, cmd)
	root, _ = root.Update(key("tab"))
	root, _ = root.Update(key("ctrl+a"))
	root, _ = root.Update(key("ctrl+k"))
	root, _ = root.Update(key("tab"))
	root, _ = root.Update(key("tab"))
	root, _ = root.Update(key("tab"))
	root, cmd = root.Update(key("enter"))
	if cmd == nil {
		t.Fatal("valid edit did not schedule update")
	}
	settle(t, root, cmd)
	updated, err := todos.List("")
	if err != nil || len(updated) != 1 {
		t.Fatalf("updated todos = %v, %v", updated, err)
	}
	if updated[0].Deadline.IsPresent() {
		t.Fatalf("blank deadline did not clear: %v", updated[0].Deadline)
	}
}

func TestCreateFormSuggestionsAndRetryUseFreshLocalNames(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	store := &memoryTodos{items: []*domain.Todo{{ID: "old", Title: "Old", ListName: "home"}}}
	todos := service.NewTodoService(store, fixedClock{now})
	cfg := config.Default()
	var root tea.Model = ui.NewRoot(todos, todos, cfg.Theme, cfg.Keys, now, "personal", "09:00")
	root, cmd := root.Update(key("n"))
	root = settle(t, root, cmd)
	root = typeText(root, "New task")
	root, _ = root.Update(key("tab"))
	root, _ = root.Update(key("tab"))
	root, _ = root.Update(key("tab"))
	root, _ = root.Update(key("ctrl+a"))
	root, _ = root.Update(key("ctrl+k"))
	for _, listName := range []string{"home", "personal"} {
		if !strings.Contains(ansi.Strip(root.View()), listName) {
			t.Fatalf("missing list suggestion %q: %s", listName, ansi.Strip(root.View()))
		}
	}
	root = typeText(root, "hm")
	root, _ = root.Update(key("down"))
	root, _ = root.Update(key("enter"))
	if !strings.Contains(ansi.Strip(root.View()), "List: home") {
		t.Fatalf(
			"choosing fuzzy suggestion did not fill the list field: %s",
			ansi.Strip(root.View()),
		)
	}
	root, _ = root.Update(key("ctrl+a"))
	root, _ = root.Update(key("ctrl+k"))
	root = typeText(root, "office")
	store.putErr = errors.New("disk full")
	root, cmd = root.Update(key("enter"))
	if cmd == nil {
		t.Fatal("valid create did not attempt write")
	}
	root = settle(t, root, cmd)
	if !strings.Contains(ansi.Strip(root.View()), "disk full") {
		t.Fatalf("write failure missing: %s", ansi.Strip(root.View()))
	}
	store.putErr = nil
	root, cmd = root.Update(key("enter"))
	if cmd == nil {
		t.Fatal("retry did not attempt write")
	}
	root = settle(t, root, cmd)
	got, err := todos.List("")
	if err != nil || len(got) != 2 {
		t.Fatalf("retry result = %d todos, %v", len(got), err)
	}
	if got[1].ListName != "office" || root.(ui.Root).Err() != nil {
		t.Fatalf(
			"retry did not preserve typed list or clear error: %#v err=%v",
			got[1],
			root.(ui.Root).Err(),
		)
	}
}

func TestFormViewSanitizesPastedTextAndFitsTinyWindows(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	cfg := config.Default()
	store := &memoryTodos{}
	todos := service.NewTodoService(store, fixedClock{now})
	var root tea.Model = ui.NewRoot(todos, todos, cfg.Theme, cfg.Keys, now, "personal", "09:00")
	root, cmd := root.Update(key("n"))
	root = settle(t, root, cmd)
	root, _ = root.Update(
		tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune("界🙂 first\nsecond\x1b[31mred\x1b[0m"),
			Paste: true,
		},
	)
	for _, size := range []tea.WindowSizeMsg{{Width: 12, Height: 4}, {Width: 1, Height: 1}, {Width: 0, Height: 0}, {Width: 80, Height: 24}} {
		root, _ = root.Update(size)
		view := root.View()
		plain := ansi.Strip(view)
		if strings.ContainsAny(plain, "\x00\x08\r\t") || strings.Contains(view, "first\nsecond") {
			t.Fatalf("view retained pasted terminal controls or line breaks: %q", view)
		}
		assertBounds(t, view, size.Width, size.Height)
	}
	if !strings.Contains(ansi.Strip(root.View()), "界🙂 first second") ||
		strings.Contains(root.View(), "\x1b[31m") {
		t.Fatalf(
			"pasted Unicode/control text was not flattened safely: %s",
			ansi.Strip(root.View()),
		)
	}
}

func TestCreateFormRejectsBlankListWithoutWriting(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	store := &memoryTodos{}
	todos := service.NewTodoService(store, fixedClock{now})
	cfg := config.Default()
	root := openForm(t, ui.NewRoot(todos, todos, cfg.Theme, cfg.Keys, now, "", "09:00"))
	root = typeText(root, "Task")
	for range 3 {
		root, _ = root.Update(key("tab"))
	}
	root, _ = root.Update(key("ctrl+a"))
	root, _ = root.Update(key("ctrl+k"))
	root, cmd := root.Update(key("enter"))
	if cmd != nil || !strings.Contains(ansi.Strip(root.View()), "List name must not be empty") {
		t.Fatalf(
			"blank list should be rejected inline: cmd=%v view=%s",
			cmd,
			ansi.Strip(root.View()),
		)
	}
	if got, _ := todos.List(""); len(got) != 0 {
		t.Fatalf("blank list wrote todos: %#v", got)
	}
}

func TestCreateFormAllowsDuplicateTitles(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	store := &memoryTodos{}
	todos := service.NewTodoService(store, fixedClock{now})
	cfg := config.Default()
	for range 2 {
		root := openForm(t, ui.NewRoot(todos, todos, cfg.Theme, cfg.Keys, now, "personal", "09:00"))
		root = typeText(root, "Same title")
		for range 3 {
			root, _ = root.Update(key("tab"))
		}
		root, cmd := root.Update(key("enter"))
		if cmd == nil {
			t.Fatal("nonempty duplicate title was rejected")
		}
		_ = settle(t, root, cmd)
	}
	got, err := todos.List("")
	if err != nil || len(got) != 2 || got[0].Title != "Same title" || got[1].Title != "Same title" {
		t.Fatalf("duplicate-title creates = %#v, %v", got, err)
	}
}

func TestFormRejectsInvalidDateThenUsesInjectedDefaultTime(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	store := &memoryTodos{}
	todos := service.NewTodoService(store, fixedClock{now})
	cfg := config.Default()
	root := openForm(t, ui.NewRoot(todos, todos, cfg.Theme, cfg.Keys, now, "personal", "09:00"))
	root = typeText(root, "Write tests")
	root, _ = root.Update(key("tab"))
	root = typeText(root, "not a date")
	root, cmd := root.Update(key("enter"))
	if cmd != nil || !strings.Contains(ansi.Strip(root.View()), "invalid date hint") {
		t.Fatalf(
			"invalid date should be rejected inline without a write: cmd=%v view=%s",
			cmd,
			ansi.Strip(root.View()),
		)
	}
	if got, _ := todos.List(""); len(got) != 0 {
		t.Fatalf("invalid date wrote todos: %#v", got)
	}
	root, _ = root.Update(key("ctrl+a"))
	root, _ = root.Update(key("ctrl+k"))
	root = typeText(root, "tomorrow")
	root, _ = root.Update(key("tab"))
	root, _ = root.Update(key("tab"))
	root, _ = root.Update(key("tab"))
	root, cmd = root.Update(key("enter"))
	if cmd == nil {
		t.Fatal("corrected deadline did not submit")
	}
	settle(t, root, cmd)
	created, err := todos.List("")
	if err != nil || len(created) != 1 {
		t.Fatalf("corrected create = %d todos, %v", len(created), err)
	}
	deadline, ok := created[0].Deadline.Get()
	want := time.Date(2026, time.October, 19, 9, 0, 0, 0, time.UTC)
	if !ok || !deadline.Equal(want) {
		t.Fatalf("deadline = %v, present=%v; want injected tomorrow default %v", deadline, ok, want)
	}
}

func TestTodoServiceRejectsPastDeadlineAndFormAllowsCorrection(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	store := &memoryTodos{}
	todos := service.NewTodoService(store, fixedClock{now})
	cfg := config.Default()
	root := openForm(t, ui.NewRoot(todos, todos, cfg.Theme, cfg.Keys, now, "personal", "09:00"))
	root = typeText(root, "Write tests")
	root, _ = root.Update(key("tab"))
	root = typeText(root, "2026-10-17 10:00")
	root, _ = root.Update(key("tab"))
	root, _ = root.Update(key("tab"))
	root, _ = root.Update(key("tab"))
	root, cmd := root.Update(key("enter"))
	if cmd == nil {
		t.Fatal("validly parsed past deadline should reach TodoService validation")
	}
	root = settle(t, root, cmd)
	if !strings.Contains(ansi.Strip(root.View()), "deadline must be in the future") {
		t.Fatalf("service future-date validation missing from form: %s", ansi.Strip(root.View()))
	}
	if got, _ := todos.List(""); len(got) != 0 {
		t.Fatalf("past deadline wrote todos: %#v", got)
	}
	root, _ = root.Update(key("shift+tab"))
	root, _ = root.Update(key("shift+tab"))
	root, _ = root.Update(key("shift+tab"))
	root, _ = root.Update(key("ctrl+a"))
	root, _ = root.Update(key("ctrl+k"))
	root = typeText(root, "tomorrow")
	root, cmd = root.Update(key("enter"))
	if cmd == nil {
		t.Fatal("corrected future deadline did not retry")
	}
	root = settle(t, root, cmd)
	created, err := todos.List("")
	if err != nil || len(created) != 1 || root.(ui.Root).Err() != nil {
		t.Fatalf(
			"corrected write did not clear service error: todos=%d err=%v root=%v view=%s",
			len(created),
			err,
			root.(ui.Root).Err(),
			ansi.Strip(root.View()),
		)
	}
	if !created[0].UpdatedAt.Equal(now) || !created[0].CreatedAt.Equal(now) {
		t.Fatalf(
			"TodoService timestamps = created %v updated %v; want %v",
			created[0].CreatedAt,
			created[0].UpdatedAt,
			now,
		)
	}
}

func TestRootIgnoresStaleFormListLoadAfterCancelAndReopen(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	store := &memoryTodos{items: []*domain.Todo{{ID: "old", Title: "Old", ListName: "stale-list"}}}
	todos := service.NewTodoService(store, fixedClock{now})
	cfg := config.Default()
	var root tea.Model = ui.NewRoot(todos, todos, cfg.Theme, cfg.Keys, now, "personal", "09:00")
	root, request := root.Update(key("n"))
	root, load := root.Update(request())
	stale := load()
	root, cancel := root.Update(key("esc"))
	root = settle(t, root, cancel)
	store.items = []*domain.Todo{{ID: "new", Title: "New", ListName: "fresh-list"}}
	root, request = root.Update(key("n"))
	root, _ = root.Update(request())
	root, _ = root.Update(stale)
	for range 3 {
		root, _ = root.Update(key("tab"))
	}
	if strings.Contains(ansi.Strip(root.View()), "stale-list") {
		t.Fatalf("stale load replaced active suggestions: %s", ansi.Strip(root.View()))
	}
}

func typeText(model tea.Model, text string) tea.Model {
	for _, r := range text {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return model
}

func settle(t *testing.T, model tea.Model, cmd tea.Cmd) tea.Model {
	t.Helper()
	for steps := 0; cmd != nil && steps < 5; steps++ {
		model, cmd = model.Update(cmd())
	}
	if cmd != nil {
		t.Fatal("command chain did not settle")
	}
	return model
}

func openForm(t *testing.T, model tea.Model) tea.Model {
	t.Helper()
	model, cmd := model.Update(key("n"))
	if cmd == nil {
		t.Fatal("new binding did not request form")
	}
	model, cmd = model.Update(cmd())
	if cmd == nil {
		t.Fatal("form route did not request local todo data")
	}
	model, _ = model.Update(cmd())
	return model
}

type memoryTodos struct {
	items  []*domain.Todo
	putErr error
}

func (s *memoryTodos) Get(id string) (*domain.Todo, error) {
	for _, todo := range s.items {
		if todo.ID == id {
			cloned := *todo
			return &cloned, nil
		}
	}
	return nil, service.ErrNotFound
}
func (s *memoryTodos) List() ([]*domain.Todo, error) {
	out := make([]*domain.Todo, len(s.items))
	for i, todo := range s.items {
		cloned := *todo
		out[i] = &cloned
	}
	return out, nil
}
func (s *memoryTodos) Put(todo *domain.Todo) error {
	if s.putErr != nil {
		return s.putErr
	}
	for i, existing := range s.items {
		if existing.ID == todo.ID {
			cloned := *todo
			s.items[i] = &cloned
			return nil
		}
	}
	cloned := *todo
	s.items = append(s.items, &cloned)
	return nil
}

type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }
