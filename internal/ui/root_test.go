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
