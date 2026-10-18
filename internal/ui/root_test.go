package ui_test

import (
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

func TestRootRoutesConfiguredCreateAndEditWhilePreservingDashboard(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Keys.New, cfg.Keys.Edit = []string{"ctrl+n"}, []string{"ctrl+e"}
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	store := &memoryTodos{
		items: []*domain.Todo{{ID: "target", Title: "selected", ListName: "home"}},
	}
	todos := service.NewTodoService(store, fixedClock{now})
	var root tea.Model = initializedRoot(t, todos, cfg, now)
	root, cmd := root.Update(key("ctrl+n"))
	if cmd == nil {
		t.Fatal("configured New key did not emit route request")
	}
	root = settle(t, root, cmd)
	if !strings.Contains(ansi.Strip(root.View()), "New todo") {
		t.Fatalf("configured New key did not route into the form: %s", ansi.Strip(root.View()))
	}
	root, cmd = root.Update(key("esc"))
	if cmd == nil {
		t.Fatal("Escape did not cancel form")
	}
	root = settle(t, root, cmd)
	assertSelected(t, root, "selected")
	assertInactiveRootKeys(t, root, []string{"n", "e"}, "selected")
	root, cmd = root.Update(key("ctrl+e"))
	if cmd == nil {
		t.Fatal("configured Edit key did not load selected todo")
	}
	root = settle(t, root, cmd)
	if !strings.Contains(ansi.Strip(root.View()), "selected") {
		t.Fatalf("configured edit did not show selected todo: %s", ansi.Strip(root.View()))
	}
}

func TestRootKeepsDashboardLoadAndFilterWhileFormIsOpen(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	store := &memoryTodos{items: []*domain.Todo{
		{ID: "match", Title: "matching task", ListName: "home"},
		{ID: "other", Title: "other task", ListName: "home"},
	}}
	todos := service.NewTodoService(store, fixedClock{now})
	cfg := config.Default()
	var root tea.Model = initializedRoot(t, todos, cfg, now)
	root, _ = root.Update(key("/"))
	root = typeText(root, "matching")
	root, dashboardLoad := root.Update(key("enter"))
	if dashboardLoad == nil {
		t.Fatal("filter submission did not defer dashboard load")
	}
	root, route := root.Update(key("n"))
	if route == nil {
		t.Fatal("new binding did not open form during dashboard load")
	}
	root = settle(t, root, route)
	root, _ = root.Update(dashboardLoad())
	root, cancel := root.Update(key("esc"))
	root = settle(t, root, cancel)
	if !strings.Contains(ansi.Strip(root.View()), "Filter: matching") ||
		!strings.Contains(ansi.Strip(root.View()), "> [ ] matching task") ||
		strings.Contains(ansi.Strip(root.View()), "Loading todos") {
		t.Fatalf(
			"dashboard load/filter/selection was lost across form cancel: %s",
			ansi.Strip(root.View()),
		)
	}
}

func TestFormTreatsPrintableQuitBindingAsTitleInput(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	cfg := config.Default()
	cfg.Keys.Quit = []string{"q", "space", "ctrl+c"}
	store := &memoryTodos{}
	todos := service.NewTodoService(store, fixedClock{now})
	root := openForm(t, ui.NewRoot(todos, todos, cfg.Theme, cfg.Keys, now, "psst", "09:00"))
	root, cmd := root.Update(key("q"))
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("printable q quit instead of entering the title")
		}
	}
	root = typeText(root, "uick")
	root, _ = root.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	root = typeText(root, "task")
	if !strings.Contains(ansi.Strip(root.View()), "Title: quick task") {
		t.Fatalf("printable quit binding was not preserved in the focused field: %s", ansi.Strip(root.View()))
	}
}

func TestFormControlQuitBindingsAndPastedText(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		keys []string
		key  tea.KeyMsg
	}{
		{name: "default ctrl+c", keys: []string{"q", "ctrl+c"}, key: key("ctrl+c")},
		{name: "configured ctrl+x", keys: []string{"ctrl+x"}, key: key("ctrl+x")},
		{name: "configured alt+q", keys: []string{"alt+q"}, key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}, Alt: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Keys.Quit = tc.keys
			store := &memoryTodos{}
			todos := service.NewTodoService(store, fixedClock{now})
			root := openForm(t, ui.NewRoot(todos, todos, cfg.Theme, cfg.Keys, now, "psst", "09:00"))
			_, cmd := root.Update(tc.key)
			assertQuit(t, cmd)
		})
	}

	cfg := config.Default()
	store := &memoryTodos{}
	todos := service.NewTodoService(store, fixedClock{now})
	root := openForm(t, ui.NewRoot(todos, todos, cfg.Theme, cfg.Keys, now, "psst", "09:00"))
	root, cmd := root.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q+uick"), Paste: true})
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("pasted printable text triggered quit")
		}
	}
	if !strings.Contains(ansi.Strip(root.View()), "Title: q+uick") {
		t.Fatalf("paste did not remain text input: %s", ansi.Strip(root.View()))
	}
}

func TestEditFormShowsLoadingAndRejectsDraftKeysUntilTodoArrives(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	store := &memoryTodos{items: []*domain.Todo{{ID: "stored", Title: "Stored title", ListName: "home"}}}
	todos := service.NewTodoService(store, fixedClock{now})
	cfg := config.Default()
	var root tea.Model = initializedRoot(t, todos, cfg, now)
	root, route := root.Update(key("e"))
	if route == nil {
		t.Fatal("edit binding did not request the selected todo")
	}
	root, load := root.Update(route())
	if load == nil {
		t.Fatal("edit form did not defer selected-todo load")
	}
	root = typeText(root, "Draft text")
	loadingView := ansi.Strip(root.View())
	if !strings.Contains(loadingView, "Loading todo") || strings.Contains(loadingView, "Draft text") {
		t.Fatalf("edit accepted input or hid its load state: %s", loadingView)
	}
	root, _ = root.Update(load())
	if !strings.Contains(ansi.Strip(root.View()), "Title: Stored title") {
		t.Fatalf("loaded todo did not initialize the edit fields: %s", ansi.Strip(root.View()))
	}

	var cancelRoot tea.Model = initializedRoot(t, todos, cfg, now)
	cancelRoot, route = cancelRoot.Update(key("e"))
	cancelRoot, load = cancelRoot.Update(route())
	cancelRoot, cancel := cancelRoot.Update(key("esc"))
	cancelRoot = settle(t, cancelRoot, cancel)
	assertSelected(t, cancelRoot, "Stored title")
	cancelRoot, _ = cancelRoot.Update(load())
	assertSelected(t, cancelRoot, "Stored title")
}

func TestNewFormRemainsEditableWhileSuggestionsLoad(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	store := &memoryTodos{}
	todos := service.NewTodoService(store, fixedClock{now})
	cfg := config.Default()
	var root tea.Model = ui.NewRoot(todos, todos, cfg.Theme, cfg.Keys, now, "psst", "09:00")
	root, route := root.Update(key("n"))
	root, load := root.Update(route())
	if load == nil {
		t.Fatal("new form did not defer suggestion load")
	}
	root = typeText(root, "New draft")
	root, _ = root.Update(load())
	if !strings.Contains(ansi.Strip(root.View()), "Title: New draft") {
		t.Fatalf("suggestion load discarded new-form input: %s", ansi.Strip(root.View()))
	}
}

func TestFormFooterShowsOnlyEffectiveQuitBindings(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	cfg := config.Default()
	cfg.Keys.Quit = []string{"q", "space", "ctrl+x", "alt+q"}
	store := &memoryTodos{}
	todos := service.NewTodoService(store, fixedClock{now})
	root := openForm(t, ui.NewRoot(todos, todos, cfg.Theme, cfg.Keys, now, "psst", "09:00"))
	view := ansi.Strip(root.View())
	if !strings.Contains(view, "ctrl+x/alt+q quit") {
		t.Fatalf("form help omitted active control/Alt quit keys: %s", view)
	}
	for _, printable := range []string{"q/space", "space quit", " · q quit"} {
		if strings.Contains(view, printable) {
			t.Fatalf("form help advertises printable field input as quit: %s", view)
		}
	}
}

func initializedRoot(
	t *testing.T,
	todos *service.TodoService,
	cfg *config.Config,
	now time.Time,
) ui.Root {
	t.Helper()
	var root tea.Model = ui.NewRoot(todos, todos, cfg.Theme, cfg.Keys, now, "personal", "09:00")
	cmd := root.(ui.Root).Init()
	root, _ = root.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	root, cmd = root.Update(cmd())
	if cmd != nil {
		root, _ = root.Update(cmd())
	}
	return root.(ui.Root)
}

func assertInactiveRootKeys(t *testing.T, model tea.Model, keys []string, selected string) {
	t.Helper()
	for _, name := range keys {
		current, cmd := model.Update(key(name))
		if cmd != nil || strings.Contains(ansi.Strip(current.View()), "New todo") {
			t.Fatalf("superseded key %q acted: %s", name, ansi.Strip(current.View()))
		}
		model = current
	}
	assertSelected(t, model, selected)
}
