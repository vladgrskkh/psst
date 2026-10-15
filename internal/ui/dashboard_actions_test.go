package ui_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/vladgrskkh/psst/internal/config"
	"github.com/vladgrskkh/psst/internal/domain"
	"github.com/vladgrskkh/psst/internal/service"
	"github.com/vladgrskkh/psst/internal/ui"
)

// Copies match the store's snapshot semantics so optimistic mutations cannot
// accidentally pass by modifying a shared todo pointer.
type actionStore struct {
	todos           map[string]domain.Todo
	putErr, listErr error
	puts, lists     int
}

func (s *actionStore) Get(id string) (*domain.Todo, error) {
	todo, ok := s.todos[id]
	if !ok {
		return nil, service.ErrNotFound
	}
	return &todo, nil
}

func (s *actionStore) List() ([]*domain.Todo, error) {
	s.lists++
	if s.listErr != nil {
		return nil, s.listErr
	}
	todos := make([]*domain.Todo, 0, len(s.todos))
	for _, todo := range s.todos {
		todos = append(todos, &todo)
	}
	return todos, nil
}

func (s *actionStore) Put(todo *domain.Todo) error {
	s.puts++
	if s.putErr != nil {
		return s.putErr
	}
	s.todos[todo.ID] = *todo
	return nil
}

type actionClock struct{}

func (actionClock) Now() time.Time { return time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC) }

func actionFixture(t *testing.T, cfg *config.Config) (tea.Model, *actionStore) {
	t.Helper()
	s := &actionStore{todos: map[string]domain.Todo{
		"a": {ID: "a", Title: "milk", ListName: "Tasks"},
		"b": {ID: "b", Title: "milk", ListName: "Tasks"},
		"c": {ID: "c", Title: "tea", ListName: "Tasks"},
	}}
	var m tea.Model = ui.NewDashboard(service.NewTodoService(s, actionClock{}), cfg.Theme, cfg.Keys, actionClock{}.Now())
	if s.lists != 0 || s.puts != 0 {
		t.Fatal("constructor performed I/O")
	}
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
	m, _ = m.Update(m.Init()())
	return m, s
}

func requireCommand(t *testing.T, cmd tea.Cmd) tea.Cmd {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected deferred command")
	}
	return cmd
}

func TestDashboardActionToggleReloadsAppliedFilter(t *testing.T) {
	t.Parallel()
	for _, binding := range []string{"space", "x"} {
		t.Run(binding, func(t *testing.T) {
			t.Parallel()
			testToggleReloadsAppliedFilter(t, binding)
		})
	}
}

func testToggleReloadsAppliedFilter(t *testing.T, binding string) {
	t.Helper()
	m, s := actionFixture(t, config.Default())
	m, cmd := submitFilter(t, m, "milk")
	m, _ = m.Update(cmd())
	m, _ = m.Update(key("j")) // second duplicate title, ID b
	m, cmd = m.Update(key(binding))
	requireCommand(t, cmd)
	if s.puts != 0 || s.lists != 2 {
		t.Fatal("Update performed mutation I/O")
	}
	if !strings.Contains(strings.ToLower(m.View()), "saving") {
		t.Fatalf("missing pending status: %s", m.View())
	}
	assertSelected(t, m, "milk")
	completion := cmd()
	if s.todos["a"].Done || !s.todos["b"].Done || s.puts != 1 {
		t.Fatalf("wrong duplicate toggled: %+v", s.todos)
	}
	if !s.todos["b"].CompletedAt.IsPresent() {
		t.Fatal("service did not set completion timestamp")
	}
	m, reload := m.Update(completion)
	requireCommand(t, reload)
	if s.lists != 2 || strings.Contains(m.View(), "DONE (1)") {
		t.Fatal("completion eagerly reloaded or changed rows")
	}
	for _, k := range []string{"x", "space", "d", "y", "/", "j", "k"} {
		var next tea.Cmd
		m, next = m.Update(key(k))
		if next != nil {
			t.Fatalf("reload pending accepted %q", k)
		}
	}
	m, _ = m.Update(reload())
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "DONE (1)") || !strings.Contains(view, "LATER (1)") || strings.Contains(view, "tea") ||
		!strings.Contains(view, "Filter: milk") {
		t.Fatalf("reload lost filter or counts: %s", view)
	}
	assertSelected(t, m, "milk")
}

func TestDashboardActionDeletePinsTarget(t *testing.T) {
	t.Parallel()
	m, s := actionFixture(t, config.Default())
	m, _ = m.Update(key("j"))
	m, cmd := m.Update(key("d"))
	if cmd != nil || s.puts != 0 || s.lists != 1 || !strings.Contains(m.View(), "Delete") ||
		!strings.Contains(m.View(), "milk") {
		t.Fatalf("delete must prompt without I/O: %s", m.View())
	}
	assertHelp(
		t,
		m,
		[]string{"y confirm", "esc/n cancel", "q/ctrl+c quit"},
		[]string{"toggle", "filter", "down", "delete"},
	)
	for _, k := range []string{"j", "k", "/", "space", "x", "d"} {
		before := m.View()
		m, cmd = m.Update(key(k))
		if cmd != nil || m.View() != before {
			t.Fatalf("prompt retargeted on %q", k)
		}
	}
	m, cmd = m.Update(key("y"))
	requireCommand(t, cmd)
	if s.puts != 0 {
		t.Fatal("confirm performed I/O")
	}
	m, reload := m.Update(cmd())
	if !s.todos["b"].Deleted || s.todos["a"].Deleted || !s.todos["b"].DeletedAt.IsPresent() {
		t.Fatalf("wrong deletion: %+v", s.todos)
	}
	m, _ = m.Update(requireCommand(t, reload)())
	if !strings.Contains(m.View(), "LATER (2)") {
		t.Fatalf("delete did not reload counts: %s", m.View())
	}
	assertSelected(t, m, "milk")
}

func TestDashboardActionDeleteCancelHasNoSideEffects(t *testing.T) {
	t.Parallel()
	for _, cancel := range []string{"n", "esc"} {
		t.Run(cancel, func(t *testing.T) {
			t.Parallel()
			m, s := actionFixture(t, config.Default())
			m, _ = m.Update(key("j"))
			before := m.View()
			m, _ = m.Update(key("d"))
			m, cmd := m.Update(key(cancel))
			if cmd != nil || m.View() != before || s.puts != 0 || s.lists != 1 {
				t.Fatal("cancel changed selection or performed I/O")
			}
		})
	}
}

func TestDashboardActionPendingBlocksChangesAndDefersQuit(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"x", "d"} {
		t.Run(action, func(t *testing.T) {
			t.Parallel()
			testPendingBlocksChangesAndDefersQuit(t, action)
		})
	}
}

func testPendingBlocksChangesAndDefersQuit(t *testing.T, action string) {
	t.Helper()
	m, s := actionFixture(t, config.Default())
	m, cmd := m.Update(key(action))
	if action == "d" {
		m, cmd = m.Update(key("y"))
	}
	requireCommand(t, cmd)
	before := m.View()
	for _, k := range []string{"x", "space", "d", "y", "n", "esc", "/", "j", "k"} {
		var repeated tea.Cmd
		m, repeated = m.Update(key(k))
		if repeated != nil || m.View() != before {
			t.Fatalf("pending accepted %q", k)
		}
	}
	m, _ = m.Update(tea.WindowSizeMsg{Width: 45, Height: 5})
	assertBounds(t, m.View(), 45, 5)
	assertHelp(t, m, []string{"quit"}, []string{"filter", "toggle", "delete", "down"})
	m, quit := m.Update(key("q"))
	if quit != nil || s.puts != 0 {
		t.Fatal("quit must wait for the accepted mutation")
	}
	if !strings.Contains(strings.ToLower(m.View()), "quit requested") {
		t.Fatalf("missing deferred quit status: %s", m.View())
	}
	_, quit = m.Update(cmd())
	assertQuit(t, quit)
	if s.puts != 1 || s.lists != 1 {
		t.Fatal("quit lost mutation or performed redundant reload")
	}
}

func TestDashboardActionFailureKeepsSelectionAndAllowsRetry(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"x", "d"} {
		t.Run(action, func(t *testing.T) {
			t.Parallel()
			testFailureKeepsSelectionAndAllowsRetry(t, action)
		})
	}
}

func testFailureKeepsSelectionAndAllowsRetry(t *testing.T, action string) {
	t.Helper()
	m, s := actionFixture(t, config.Default())
	m, _ = m.Update(key("j"))
	s.putErr = errors.New("disk\n\x1b[31mfailed\x1b[0m\u202e 界🙂")
	m, cmd := m.Update(key(action))
	if action == "d" {
		m, cmd = m.Update(key("y"))
	}
	m, next := m.Update(requireCommand(t, cmd)())
	if next != nil || s.lists != 1 {
		t.Fatal("failed mutation reloaded or reported success")
	}
	assertSelected(t, m, "milk")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "disk failed 界🙂") || strings.Contains(view, "\u202e") {
		t.Fatalf("missing sanitized error: %s", view)
	}
	assertDashboardError(t, m, s.putErr)
	s.putErr = nil
	m, cmd = m.Update(key(action))
	if action == "d" {
		m, cmd = m.Update(key("y"))
	}
	assertDashboardError(t, m, nil)
	m, next = m.Update(requireCommand(t, cmd)())
	m, _ = m.Update(requireCommand(t, next)())
	assertDashboardError(t, m, nil)
	if action == "x" && (!s.todos["b"].Done || s.todos["a"].Done) {
		t.Fatal("failure changed selection before retry")
	}
	if action == "d" && (!s.todos["b"].Deleted || s.todos["a"].Deleted) {
		t.Fatal("failure changed pinned target before retry")
	}
	m, _ = m.Update(key("j"))
	assertSelected(t, m, "tea")
	m, cmd = submitFilter(t, m, "tea")
	m, _ = m.Update(cmd())
	assertSelected(t, m, "tea")
}

func TestDashboardActionReloadFailureDoesNotReplayMutation(t *testing.T) {
	t.Parallel()
	m, s := actionFixture(t, config.Default())
	m, cmd := m.Update(key("x"))
	m, reload := m.Update(requireCommand(t, cmd)())
	s.listErr = errors.New("reload failed")
	m, next := m.Update(requireCommand(t, reload)())
	if next != nil || !strings.Contains(m.View(), "reload failed") || s.puts != 1 {
		t.Fatalf("reload failure: %s", m.View())
	}
	s.listErr = nil
	m, cmd = submitFilter(t, m, "milk")
	m, _ = m.Update(cmd())
	if s.puts != 1 || !strings.Contains(m.View(), "DONE (1)") {
		t.Fatal("recovery replayed mutation")
	}
	_, quit := m.Update(key("q"))
	assertQuit(t, quit)
}

func TestDashboardActionIgnoresOldReadsAndCompletions(t *testing.T) {
	t.Parallel()
	for _, staleError := range []bool{false, true} {
		t.Run(strconv.FormatBool(staleError), func(t *testing.T) {
			t.Parallel()
			testIgnoresOldReadsAndCompletions(t, staleError)
		})
	}
}

func testIgnoresOldReadsAndCompletions(t *testing.T, staleError bool) {
	t.Helper()
	m, s := actionFixture(t, config.Default())
	oldRead := m.Init()
	if staleError {
		s.listErr = errors.New("obsolete failure")
	}
	stale := oldRead()
	s.listErr = nil
	m, cmd := m.Update(key("x"))
	requireCommand(t, cmd)
	before := m.View()
	m, _ = m.Update(stale)
	if m.View() != before {
		t.Fatal("old read replaced pending mutation")
	}
	completion := cmd()
	m, reload := m.Update(completion)
	requireCommand(t, reload)
	m, duplicate := m.Update(completion)
	if duplicate != nil {
		t.Fatal("duplicate completion reloaded")
	}
	before = m.View()
	m, _ = m.Update(stale)
	if m.View() != before {
		t.Fatal("old read replaced pending reload")
	}
	result := reload()
	m, _ = m.Update(result)
	before = m.View()
	m, _ = m.Update(stale)
	if m.View() != before {
		t.Fatal("old read restored stale rows/error")
	}
	m, cmd = m.Update(key("x"))
	requireCommand(t, cmd)
	before = m.View()
	m, duplicate = m.Update(completion)
	if duplicate != nil || m.View() != before {
		t.Fatal("old completion consumed a new operation")
	}
	m, _ = m.Update(result)
	if m.View() != before {
		t.Fatal("old reload result consumed a new operation")
	}
	m, reload = m.Update(cmd())
	m, _ = m.Update(requireCommand(t, reload)())
	if !strings.Contains(m.View(), "DONE (2)") {
		t.Fatalf("second operation lost: %s", m.View())
	}
}

func TestDashboardActionRemappedBindings(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Keys.ToggleDone, cfg.Keys.Delete = []string{"t"}, []string{"z"}
	cfg.Keys.Confirm, cfg.Keys.Cancel = []string{"c"}, []string{"v"}
	m, s := actionFixture(t, cfg)
	cfg.Keys.ToggleDone[0], cfg.Keys.Delete[0], cfg.Keys.Confirm[0], cfg.Keys.Cancel[0] = "x", "d", "y", "esc"
	assertHelp(t, m, []string{"t toggle", "z delete"}, []string{"space/x toggle", "d delete"})
	before := m.View()
	for _, k := range []string{"x", "space", "d"} {
		var cmd tea.Cmd
		m, cmd = m.Update(key(k))
		if cmd != nil || m.View() != before {
			t.Fatalf("superseded action %q active", k)
		}
	}
	m, _ = m.Update(key("z"))
	assertHelp(t, m, []string{"c confirm", "v/n cancel"}, []string{"y confirm", "esc cancel"})
	before = m.View()
	for _, k := range []string{"y", "esc"} {
		var cmd tea.Cmd
		m, cmd = m.Update(key(k))
		if cmd != nil || m.View() != before {
			t.Fatalf("superseded prompt key %q active", k)
		}
	}
	m, _ = m.Update(key("v"))
	m, _ = m.Update(key("z"))
	m, cmd := m.Update(key("c"))
	m, reload := m.Update(requireCommand(t, cmd)())
	m, _ = m.Update(requireCommand(t, reload)())
	if !s.todos["a"].Deleted {
		t.Fatal("remapped confirm did not delete")
	}
	m, cmd = m.Update(key("t"))
	m, reload = m.Update(requireCommand(t, cmd)())
	m, _ = m.Update(requireCommand(t, reload)())
	if !strings.Contains(m.View(), "DONE (1)") {
		t.Fatalf("remapped toggle failed to reload: %s", m.View())
	}
	if !s.todos["b"].Done {
		t.Fatal("remapped toggle did not toggle")
	}
}

func TestDashboardActionDisabledBindings(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Keys.ToggleDone, cfg.Keys.Delete = nil, nil
	m, _ := actionFixture(t, cfg)
	before := m.View()
	var cmd tea.Cmd
	for _, k := range []string{"x", "space", "d", "y"} {
		m, cmd = m.Update(key(k))
		if cmd != nil || m.View() != before {
			t.Fatalf("disabled action %q active", k)
		}
	}
	assertHelp(t, m, nil, []string{"toggle", "delete"})
	cfg.Keys.Delete, cfg.Keys.Confirm, cfg.Keys.Cancel = []string{"d"}, nil, nil
	m, s := actionFixture(t, cfg)
	m, _ = m.Update(key("d"))
	assertHelp(t, m, []string{"n cancel"}, []string{"confirm", "esc"})
	before = m.View()
	for _, k := range []string{"y", "esc"} {
		m, cmd = m.Update(key(k))
		if cmd != nil || m.View() != before {
			t.Fatalf("disabled prompt binding %q active", k)
		}
	}
	m, cmd = m.Update(key("n"))
	if cmd != nil || s.puts != 0 || strings.Contains(m.View(), "Delete") {
		t.Fatal("fixed n cancellation failed")
	}
}

func TestDashboardActionPromptBindingPrecedence(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                  string
		quit, cancel, confirm []string
		press, outcome        string
		want, excluded        []string
	}{
		{"quit wins", []string{"y"}, []string{"y", "esc"}, []string{"y"}, "y", "quit",
			[]string{"y quit", "esc/n cancel"}, []string{"confirm", "y/esc"}},
		{"cancel wins", []string{"q"}, []string{"y"}, []string{"y"}, "y", "cancel",
			[]string{"y/n cancel", "q quit"}, []string{"confirm"}},
		{"n cancels", []string{"q"}, nil, []string{"n", "c"}, "n", "cancel",
			[]string{"n cancel", "c confirm"}, []string{"n/c confirm"}},
		{"n quits", []string{"n"}, nil, []string{"n", "c"}, "n", "quit",
			[]string{"n quit", "c confirm"}, []string{"cancel"}},
		{"remaining confirms", []string{"q"}, []string{"y"}, []string{"y", "c"}, "c", "delete",
			[]string{"y/n cancel", "c confirm"}, []string{"y/c confirm"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Keys.Quit, cfg.Keys.Cancel, cfg.Keys.Confirm = tc.quit, tc.cancel, tc.confirm
			m, s := actionFixture(t, cfg)
			m, _ = m.Update(key("d"))
			assertHelp(t, m, tc.want, tc.excluded)
			m, cmd := m.Update(key(tc.press))
			switch tc.outcome {
			case "quit":
				assertQuit(t, cmd)
			case "cancel":
				if cmd != nil || strings.Contains(m.View(), "Delete") {
					t.Fatal("cancel lost precedence")
				}
			case "delete":
				var reload tea.Cmd
				m, reload = m.Update(requireCommand(t, cmd)())
				m, _ = m.Update(requireCommand(t, reload)())
				if !strings.Contains(m.View(), "LATER (2)") {
					t.Fatalf("confirmation did not reload: %s", m.View())
				}
				if !s.todos["a"].Deleted {
					t.Fatal("effective confirm did not delete")
				}
			}
		})
	}
}

func TestDashboardActionNoSelection(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"loading", "empty", "error", "completed"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			r := &todoReader{}
			if state == "error" {
				r.err = errors.New("load failed")
			}
			if state == "completed" {
				r.todos = []*domain.Todo{{ID: "done", Title: "done", Done: true}}
			}
			cfg := config.Default()
			var m tea.Model = ui.NewDashboard(r, cfg.Theme, cfg.Keys, actionClock{}.Now())
			if state != "loading" {
				m, _ = m.Update(m.Init()())
			}
			before := m.View()
			for _, k := range []string{"space", "x", "d", "y"} {
				var cmd tea.Cmd
				m, cmd = m.Update(key(k))
				if cmd != nil || m.View() != before {
					t.Fatalf("%s accepted action %q", state, k)
				}
			}
		})
	}
}

func TestDashboardActionPasteCannotMutateAndFilterKeysRemainText(t *testing.T) {
	t.Parallel()
	m, s := actionFixture(t, config.Default())
	before := m.View()
	for _, text := range []string{"d", "x", " ", "y", "q"} {
		var cmd tea.Cmd
		m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true})
		if cmd != nil || m.View() != before {
			t.Fatalf("paste %q dispatched an action", text)
		}
	}
	m, _ = m.Update(key("d"))
	before = m.View()
	for _, text := range []string{"y", "n", "q"} {
		var cmd tea.Cmd
		m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true})
		if cmd != nil || m.View() != before {
			t.Fatalf("prompt accepted paste %q", text)
		}
	}
	m, _ = m.Update(key("n"))
	m, _ = m.Update(key("/"))
	for _, text := range []string{"d", "x", "space", "y", "n"} {
		var cmd tea.Cmd
		m, cmd = m.Update(key(text))
		if cmd != nil {
			t.Fatal("action key escaped filter editor")
		}
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("dxy界🙂"), Paste: true})
	if !strings.Contains(ansi.Strip(m.View()), "Filter: dx yndxy界🙂") || s.puts != 0 || s.lists != 1 {
		t.Fatalf("draft action keys: %s", m.View())
	}
	assertHelp(t, m, []string{"submit", "cancel"}, []string{"toggle", "delete"})
}

func TestDashboardActionStatusBoundsAndSanitization(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"confirmation", "pending", "error"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			testActionStatusBoundsAndSanitization(t, state)
		})
	}
}

func testActionStatusBoundsAndSanitization(t *testing.T, state string) {
	t.Helper()
	m, s := actionFixture(t, config.Default())
	todo := s.todos["a"]
	todo.Title = "\x1b[31m\u202eunsafe\n界🙂 " + strings.Repeat("界🙂", 80)
	s.todos["a"] = todo
	m, _ = m.Update(m.Init()())
	m, _ = m.Update(key("d"))
	want := "Delete"
	if state != "confirmation" {
		var cmd tea.Cmd
		m, cmd = m.Update(key("y"))
		requireCommand(t, cmd)
		want = "Saving"
		if state == "error" {
			s.putErr = errors.New("unsafe\x1b[31m\nerror\u202e 界🙂 " + strings.Repeat("界🙂", 80))
			m, _ = m.Update(cmd())
			want = "Could not delete"
		}
	}
	for _, size := range []tea.WindowSizeMsg{
		{Width: 80, Height: 24}, {Width: 16, Height: 6}, {Width: 8, Height: 2},
		{Width: 1, Height: 1}, {Width: 40, Height: 1}, {Width: 30, Height: 3},
		{Width: 0, Height: 0}, {Width: -1, Height: -2}, {Width: 160, Height: 24},
	} {
		m, _ = m.Update(size)
		assertBounds(t, m.View(), size.Width, size.Height)
		view := ansi.Strip(m.View())
		if strings.Contains(view, "\u202e") {
			t.Fatal("unsafe control reached output")
		}
		if size.Width > 0 && size.Height > 0 && !strings.Contains(view, want[:1]) {
			t.Fatalf("status hidden: size=%+v view=%s", size, view)
		}
		if size.Width >= 40 && size.Height > 0 && !strings.Contains(view, want) {
			t.Fatalf("status hidden despite space: %s", view)
		}
	}
}

func TestDashboardActionQuitAfterFailedWriteAndDuringReload(t *testing.T) {
	t.Parallel()
	for _, fail := range []bool{false, true} {
		t.Run(strconv.FormatBool(fail), func(t *testing.T) {
			t.Parallel()
			m, s := actionFixture(t, config.Default())
			m, cmd := m.Update(key("x"))
			requireCommand(t, cmd)
			if fail {
				s.putErr = errors.New("write failed")
				var next tea.Cmd
				m, next = m.Update(key("ctrl+c"))
				if next != nil {
					t.Fatal("quit raced failing write")
				}
				_, next = m.Update(cmd())
				assertQuit(t, next)
			} else {
				var reload tea.Cmd
				m, reload = m.Update(cmd())
				requireCommand(t, reload)
				_, next := m.Update(key("ctrl+c"))
				assertQuit(t, next)
			}
			if s.puts != 1 || s.lists != 1 {
				t.Fatal("quit performed extra I/O")
			}
		})
	}
}

func TestDashboardActionDeferredQuitStaysTerminal(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, action string
		fail         bool
	}{
		{"toggle", "x", false}, {"delete", "d", false},
		{"failed toggle", "x", true}, {"failed delete", "d", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testDeferredQuitStaysTerminal(t, tc.action, tc.fail)
		})
	}
}

func testDeferredQuitStaysTerminal(t *testing.T, action string, fail bool) {
	t.Helper()
	m, s := actionFixture(t, config.Default())
	m, write := m.Update(key(action))
	if action == "d" {
		m, write = m.Update(key("y"))
	}
	requireCommand(t, write)
	if fail {
		s.putErr = errors.New("disk\n\x1b[31mfailed\x1b[0m\u202e 界🙂")
	}
	m, quit := m.Update(key("q"))
	if quit != nil {
		t.Fatal("quit raced the accepted write")
	}
	completion := write()
	m, quit = m.Update(completion)
	requireCommand(t, quit)
	assertDashboardError(t, m, s.putErr)
	if fail {
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "disk failed 界🙂") || strings.Contains(view, "\u202e") ||
			strings.Contains(view, "Saving") {
			t.Fatalf("quitting model lost the sanitized write error: %s", view)
		}
	}
	// Deliver x before the quit command runs, exactly as the event loop can.
	assertTerminalBeforeQuit(t, m, quit, []tea.Msg{completion})
	if s.puts != 1 || s.lists != 1 {
		t.Fatal("quitting replayed the mutation or reloaded")
	}
}

func TestDashboardActionQuitPathsStayTerminal(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"idle", "confirmation", "filter", "loading", "reload"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			testQuitPathStaysTerminal(t, path)
		})
	}
}

func testQuitPathStaysTerminal(t *testing.T, path string) {
	t.Helper()
	m, s := actionFixture(t, config.Default())
	var read tea.Cmd
	switch path {
	case "idle":
	case "confirmation":
		m, _ = m.Update(key("d"))
	case "filter":
		m, _ = m.Update(key("/"))
	case "loading":
		m, read = submitFilter(t, m, "milk")
	case "reload":
		var write tea.Cmd
		m, write = m.Update(key("x"))
		m, read = m.Update(requireCommand(t, write)())
	}
	if read == nil {
		read = m.Init()
	}
	// Capture both outcomes with the current generation: a late successful
	// reload must not restore idle, and a late failure must not replace the view.
	lateResult := read()
	s.listErr = errors.New("late load failure")
	lateError := read()
	s.listErr = nil
	writes, reads := s.puts, s.lists
	m, quit := m.Update(key("ctrl+c"))
	requireCommand(t, quit)
	assertDashboardError(t, m, nil)
	before := m.View()
	for _, msg := range []tea.Msg{lateResult, lateError} {
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		if cmd != nil || m.View() != before {
			t.Fatal("late load changed the terminal quitting model")
		}
	}
	assertTerminalBeforeQuit(t, m, quit, nil)
	if s.puts != writes || s.lists != reads {
		t.Fatal("quitting performed additional I/O")
	}
}

func assertTerminalBeforeQuit(t *testing.T, m tea.Model, quit tea.Cmd, late []tea.Msg) {
	t.Helper()
	before := m.View()
	// Check immediately, then again after each late completion/load message.
	for i := 0; i <= len(late); i++ {
		if i > 0 {
			var cmd tea.Cmd
			m, cmd = m.Update(late[i-1])
			if cmd != nil || m.View() != before {
				t.Fatal("late message changed the terminal quitting model")
			}
		}
		for _, name := range []string{"x", "space", "d", "y", "n", "esc", "/", "enter", "j", "k", "q", "ctrl+c", "x", "d", "y"} {
			var cmd tea.Cmd
			m, cmd = m.Update(key(name))
			if cmd != nil || m.View() != before {
				t.Fatalf("quitting accepted %q before QuitMsg", name)
			}
		}
	}
	assertHelp(t, m, nil, []string{"toggle", "delete", "filter", "cancel", "confirm", "quit", "submit"})
	assertQuit(t, quit)
}

func assertDashboardError(t *testing.T, m tea.Model, cause error) {
	t.Helper()
	result, ok := m.(interface{ Err() error })
	if !ok {
		t.Fatal("dashboard does not expose its final mutation error")
	}
	err := result.Err()
	if !errors.Is(err, cause) {
		t.Fatalf("dashboard error = %v; want cause %v", err, cause)
	}
	if cause != nil && (!strings.Contains(err.Error(), "disk failed 界🙂") ||
		strings.ContainsAny(err.Error(), "\n\x1b\u202e")) {
		t.Fatalf("final error is not sanitized: %q", err.Error())
	}
}

func TestDashboardActionToggleKeepsSelectionInTwoRows(t *testing.T) {
	t.Parallel()
	m, s := actionFixture(t, config.Default())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 2})
	assertSelected(t, m, "milk")
	m, write := m.Update(key("x"))
	if !strings.Contains(m.View(), "Saving") {
		t.Fatalf("missing saving status: %s", m.View())
	}
	// Check both transitions directly, without a resize to repair the viewport.
	if !strings.Contains(ansi.Strip(m.View()), "> [ ] milk") {
		t.Errorf("starting toggle hid the selected row: %s", m.View())
	}
	s.putErr = errors.New("disk full")
	m, _ = m.Update(requireCommand(t, write)())
	if !strings.Contains(m.View(), "Could not toggle todo: disk full") {
		t.Fatalf("missing failure status: %s", m.View())
	}
	assertSelected(t, m, "milk")
	assertBounds(t, m.View(), 80, 2)
}

func TestDashboardActionDismissedStatusKeepsUnresolvedErrorOnQuit(t *testing.T) {
	t.Parallel()
	for _, dismiss := range []string{"filter", "confirmation"} {
		t.Run(dismiss, func(t *testing.T) {
			t.Parallel()
			m, s := actionFixture(t, config.Default())
			s.putErr = errors.New("disk\n\x1b[31mfailed\x1b[0m\u202e 界🙂")
			m, write := m.Update(key("x"))
			m, _ = m.Update(requireCommand(t, write)())
			if dismiss == "filter" {
				m, _ = m.Update(key("/"))
				m, _ = m.Update(key("esc"))
			} else {
				m, _ = m.Update(key("d"))
				m, _ = m.Update(key("n"))
			}
			m, quit := m.Update(key("q"))
			assertQuit(t, quit)
			assertDashboardError(t, m, s.putErr)
		})
	}
}
