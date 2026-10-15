package ui_test

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/vladgrskkh/psst/internal/config"
	"github.com/vladgrskkh/psst/internal/domain"
	"github.com/vladgrskkh/psst/internal/ui"
	"github.com/vladgrskkh/psst/pkg/option"
)

type todoReader struct {
	todos  []*domain.Todo
	err    error
	calls  int
	filter string
}

func (r *todoReader) List(filter string) ([]*domain.Todo, error) {
	r.calls++
	r.filter = filter
	return r.todos, r.err
}

func TestDashboardLoadsOnlyThroughInitCommand(t *testing.T) {
	t.Parallel()
	r := &todoReader{todos: []*domain.Todo{{ID: "1", Title: "buy milk"}}}
	cfg := config.Default()
	var m tea.Model = ui.NewDashboard(r, cfg.Theme, cfg.Keys, time.Now())
	if r.calls != 0 || !strings.Contains(m.View(), "Loading") {
		t.Fatalf("constructor should show loading without I/O: calls=%d view=%q", r.calls, m.View())
	}
	cmd := m.Init()
	if cmd == nil || r.calls != 0 {
		t.Fatal("Init must return a deferred loading command")
	}
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = m.Update(key("j"))
	if r.calls != 0 {
		t.Fatal("Update performed I/O")
	}
	m, next := m.Update(cmd())
	if next != nil || r.calls != 1 || r.filter != "" {
		t.Fatalf("load should list one unfiltered snapshot: calls=%d filter=%q", r.calls, r.filter)
	}
	assertSelected(t, m, "buy milk")
	for _, k := range []string{"j", "k", "x", "n", "e", "d", "tab", "s"} {
		m, next = m.Update(key(k))
		if next != nil {
			t.Fatalf("read-only key %q returned a command", k)
		}
	}
	if r.calls != 1 {
		t.Fatal("interaction re-read the snapshot")
	}
}

func TestDashboardFeedbackAndQuit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		err   error
		todos []*domain.Todo
		want  string
		done  string
	}{
		{name: "empty", want: "No todos", done: "DONE (0)"},
		{name: "error", err: errors.New("database unavailable"), want: "database unavailable"},
		{name: "completed", todos: []*domain.Todo{{Title: "hidden completed", Done: true}},
			want: "No open todos", done: "DONE (1)"},
		{name: "deleted", todos: []*domain.Todo{{Title: "hidden deleted", Deleted: true},
			{Title: "hidden deleted completed", Done: true, Deleted: true}}, want: "No todos", done: "DONE (0)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			m := loaded(t, &todoReader{todos: tc.todos, err: tc.err}, cfg, time.Now(), 80, 24)
			view := ansi.Strip(m.View())
			if !strings.Contains(view, tc.want) || (tc.done != "" && !strings.Contains(view, tc.done)) {
				t.Fatalf("missing feedback %q / %q in:\n%s", tc.want, tc.done, view)
			}
			for _, k := range []string{"j", "down", "k", "up", "x", "space"} {
				m, _ = m.Update(key(k))
			}
			if strings.Contains(m.View(), "hidden") || strings.Contains(m.View(), "> [ ]") {
				t.Fatalf("completed/deleted rows became visible or selectable: %s", m.View())
			}
			for _, k := range []string{"q", "ctrl+c"} {
				_, cmd := m.Update(key(k))
				assertQuit(t, cmd)
			}
		})
	}
	cfg := config.Default()
	var loading tea.Model = ui.NewDashboard(&todoReader{}, cfg.Theme, cfg.Keys, time.Now())
	_, cmd := loading.Update(key("q"))
	assertQuit(t, cmd)
}

func TestDashboardCalendarGrouping(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, zone, now, tomorrow, later string
	}{
		{"local midnight", "Europe/Moscow", "2026-10-13T23:30:00+03:00",
			"2026-10-14T00:15:00+03:00", "2026-10-15T00:00:00+03:00"},
		{"spring DST", "America/New_York", "2026-03-08T00:30:00-05:00",
			"2026-03-09T00:15:00-04:00", "2026-03-10T00:00:00-04:00"},
		{"fall DST", "America/New_York", "2026-11-01T00:30:00-04:00",
			"2026-11-02T00:15:00-05:00", "2026-11-03T00:00:00-05:00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			loc, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Fatal(err)
			}
			now := timestamp(t, tc.now).In(loc)
			todos := []*domain.Todo{
				{ID: "later", Title: "future", Deadline: option.Some(timestamp(t, tc.later).UTC())},
				{ID: "none", Title: "unscheduled"},
				{ID: "tomorrow", Title: "next calendar day", Deadline: option.Some(timestamp(t, tc.tomorrow).UTC())},
				{ID: "today", Title: "later today", Deadline: option.Some(now.Add(time.Minute).UTC())},
				{ID: "equal", Title: "exactly now", Deadline: option.Some(now.UTC())},
				{ID: "past", Title: "earlier today", Deadline: option.Some(now.Add(-time.Minute).UTC())},
				{ID: "old", Title: "previous day", Deadline: option.Some(now.AddDate(0, 0, -1).UTC())},
				{ID: "done", Title: "hidden completed", Done: true},
				{ID: "deleted", Title: "hidden deleted", Deleted: true},
			}
			m := loaded(t, &todoReader{todos: todos}, config.Default(), now, 80, 30)
			want := map[string][]string{
				"OVERDUE":  {"previous day", "earlier today"},
				"TODAY":    {"exactly now", "later today"},
				"TOMORROW": {"next calendar day"},
				"LATER":    {"future", "unscheduled"},
			}
			if got := renderedGroups(m.View()); !reflect.DeepEqual(got, want) {
				t.Fatalf("calendar groups = %#v; want %#v\n%s", got, want, m.View())
			}
			if !strings.Contains(m.View(), "DONE (1)") || strings.Contains(m.View(), "hidden") {
				t.Fatalf("completed/deleted presentation: %s", m.View())
			}
		})
	}
}

func TestDashboardSortPreservesSnapshot(t *testing.T) {
	t.Parallel()
	todos := []*domain.Todo{
		{ID: "z", Title: "zeta"}, {ID: "b", Title: "alpha"}, {ID: "a", Title: "alpha"},
		{ID: "done", Title: "finished", Done: true},
	}
	before := make([]domain.Todo, len(todos))
	for i, todo := range todos {
		before[i] = *todo
	}
	m := loaded(t, &todoReader{todos: todos}, config.Default(), time.Now(), 80, 24)
	for _, title := range []string{"alpha", "alpha", "zeta", "zeta"} {
		assertSelected(t, m, title)
		m, _ = m.Update(key("j"))
	}
	for i, todo := range todos {
		if !reflect.DeepEqual(*todo, before[i]) {
			t.Fatalf("source snapshot changed at %d", i)
		}
	}
	// Later changes to the reader's objects cannot alter the loaded presentation.
	todos[0].Title = "changed outside dashboard"
	if strings.Contains(m.View(), "changed outside dashboard") {
		t.Fatal("view aliases reader data")
	}
}

func TestDashboardNavigationAndRemaps(t *testing.T) {
	t.Parallel()
	for _, custom := range []bool{false, true} {
		t.Run(strconv.FormatBool(custom), func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			up, down, quit := "k", "j", "q"
			if custom {
				cfg.Keys.Up, cfg.Keys.Down, cfg.Keys.Quit = []string{"ctrl+p"}, []string{"ctrl+n"}, []string{"esc"}
				up, down, quit = "ctrl+p", "ctrl+n", "esc"
			}
			m := loaded(t, &todoReader{todos: []*domain.Todo{
				{ID: "a", Title: "alpha"}, {ID: "b", Title: "beta"},
				{Title: "hidden", Done: true},
			}}, cfg, time.Now(), 100, 24)
			m, _ = m.Update(key(up))
			assertSelected(t, m, "alpha")
			if custom {
				assertInactiveKeys(t, m, []string{"k", "up", "j", "down", "q", "ctrl+c"}, "alpha")
			}
			m, _ = m.Update(key(down))
			assertSelected(t, m, "beta")
			m, _ = m.Update(key(down))
			assertSelected(t, m, "beta")
			if !custom {
				m, _ = m.Update(key("up"))
				assertSelected(t, m, "alpha")
				m, _ = m.Update(key("down"))
				assertSelected(t, m, "beta")
			}
			m, _ = m.Update(key(up))
			assertSelected(t, m, "alpha")
			_, cmd := m.Update(key(quit))
			assertQuit(t, cmd)
			excluded := []string{"toggle", "edit", "delete", "sync", "new", "progress"}
			if custom {
				excluded = append(excluded, "k/up", "j/down", "q/ctrl+c")
			}
			assertHelp(t, m, []string{up, down, quit}, excluded)
		})
	}
}

func TestDashboardSpaceBindings(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"down", "quit"} {
		t.Run(action, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			excluded := "j/down"
			if action == "down" {
				cfg.Keys.Down = []string{"space"}
			} else {
				cfg.Keys.Quit = []string{"space"}
				excluded = "q/ctrl+c"
			}
			m := loaded(t, &todoReader{todos: []*domain.Todo{
				{ID: "a", Title: "alpha"}, {ID: "b", Title: "beta"},
			}}, cfg, time.Now(), 100, 24)
			assertHelp(t, m, []string{"space " + action}, []string{excluded})
			m, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
			if action == "down" {
				if cmd != nil {
					t.Fatal("space navigation returned a command")
				}
				assertSelected(t, m, "beta")
			} else {
				assertQuit(t, cmd)
			}
		})
	}
}

func TestDashboardDisabledBindings(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Keys.Up, cfg.Keys.Down, cfg.Keys.Quit = nil, nil, nil
	m := loaded(t, &todoReader{todos: []*domain.Todo{{Title: "alpha"}, {Title: "beta"}}}, cfg,
		time.Now(), 80, 24)
	assertInactiveKeys(t, m, []string{"j", "down", "q", "ctrl+c"}, "alpha")
	assertHelp(t, m, nil, []string{"quit", "k/up", "j/down"})
}

func TestDashboardViewportKeepsSelectionVisible(t *testing.T) {
	t.Parallel()
	todos := make([]*domain.Todo, 40)
	now := time.Date(2026, time.October, 13, 12, 0, 0, 0, time.UTC)
	for i := range todos {
		todos[i] = &domain.Todo{ID: strconv.Itoa(i), Title: fmt.Sprintf("task %02d", i),
			Deadline: option.Some(now.Add(time.Duration(i-10) * time.Hour))}
	}
	m := loaded(t, &todoReader{todos: todos}, config.Default(), now, 80, 24)
	for _, size := range []tea.WindowSizeMsg{
		{Width: 80, Height: 24}, {Width: 16, Height: 6}, {Width: 1, Height: 1},
		{Width: 8, Height: 2}, {Width: 8, Height: 3}, {Width: 30, Height: 8},
		{Width: 0, Height: 0}, {Width: -1, Height: -3}, {Width: 80, Height: 24},
	} {
		m, _ = m.Update(size)
		assertViewport(t, m, size)
		for _, k := range []string{"j", "k"} {
			for step := range len(todos) + 2 {
				m, _ = m.Update(key(k))
				assertViewport(t, m, size)
				if size.Width >= 13 && size.Height > 0 {
					index := min(step+1, len(todos)-1)
					if k == "k" {
						index = max(len(todos)-step-2, 0)
					}
					assertSelected(t, m, fmt.Sprintf("task %02d", index))
				}
			}
		}
	}
}

func TestDashboardLastRowRevealsCollapsedDoneCount(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 13, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		group    string
		deadline option.Option[time.Time]
	}{
		{"OVERDUE", option.Some(now.Add(-time.Hour))},
		{"TODAY", option.Some(now.Add(time.Hour))},
		{"TOMORROW", option.Some(now.AddDate(0, 0, 1))},
		{"LATER", option.None[time.Time]()},
	} {
		t.Run(tc.group, func(t *testing.T) {
			t.Parallel()
			todos := make([]*domain.Todo, 0, 31)
			for i := range 30 {
				todos = append(todos, &domain.Todo{
					ID: strconv.Itoa(i), Title: fmt.Sprintf("task %02d", i), Deadline: tc.deadline,
				})
			}
			todos = append(todos, &domain.Todo{Title: "hidden completed", Done: true})
			m := loaded(t, &todoReader{todos: todos}, config.Default(), now, 80, 24)
			for range todos {
				m, _ = m.Update(key("j"))
			}
			assertSelected(t, m, "task 29")
			if !strings.Contains(m.View(), "DONE (1)") {
				t.Fatalf("collapsed count is unreachable after %s tasks:\n%s", tc.group, m.View())
			}
			for _, size := range []tea.WindowSizeMsg{
				{Width: 16, Height: 3}, {Width: 16, Height: 5},
				{Width: 0, Height: 0}, {Width: 60, Height: 15},
			} {
				m, _ = m.Update(size)
				assertBounds(t, m.View(), size.Width, size.Height)
				if size.Width > 0 && size.Height > 0 {
					assertSelected(t, m, "task 29")
				}
			}
		})
	}
}

func TestDashboardTitlesOccupyOneSafeRow(t *testing.T) {
	t.Parallel()
	title := "first\nsecond\r\tthird \x1b[31mRED\x1b[0m \x1b]8;;https://example.com\x1b\\link\x1b]8;;\x1b\\ \x00end\x08"
	m := loaded(t, &todoReader{todos: []*domain.Todo{
		{ID: "a", Title: title}, {ID: "b", Title: strings.Repeat("界🙂", 100)},
	}}, config.Default(), time.Now(), 80, 24)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "first second third RED link end") {
		t.Fatalf("title was not flattened safely: %q", view)
	}
	if strings.ContainsAny(view, "\x1b\x00\x08\r\t") || strings.Contains(m.View(), "https://example.com") {
		t.Fatalf("title retained terminal controls: %q", m.View())
	}
	if strings.Count(view, "[ ]") != 2 {
		t.Fatalf("todos do not occupy two rows: %q", view)
	}
	assertBounds(t, m.View(), 80, 24)
	for _, width := range []int{1, 2, 5, 7, 13} {
		m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 10})
		m, _ = m.Update(key("j"))
		assertBounds(t, m.View(), width, 10)
	}
}

func TestDashboardFilterSubmitsThroughCommand(t *testing.T) {
	t.Parallel()
	r := &todoReader{todos: []*domain.Todo{{Title: "alpha"}, {Title: "beta"}}}
	m := loaded(t, r, config.Default(), time.Now(), 100, 24)
	m, _ = m.Update(key("j"))
	assertSelected(t, m, "beta")
	assertHelp(t, m, []string{"/ filter"}, nil)
	m, _ = m.Update(key("/"))
	for _, text := range []string{"q", "j", "k", " milk"} {
		var cmd tea.Cmd
		m, cmd = m.Update(key(text))
		if cmd != nil {
			t.Fatal("typing dispatched a dashboard action")
		}
	}
	assertSelected(t, m, "beta")
	if r.calls != 1 || !strings.Contains(ansi.Strip(m.View()), "Filter: qjk milk") {
		t.Fatalf("editing should retain the snapshot and show text: calls=%d view=%s", r.calls, m.View())
	}
	assertHelp(t, m, []string{"enter", "submit", "esc", "cancel"}, []string{"q/ctrl+c quit"})
	m, cmd := m.Update(key("enter"))
	if cmd == nil || r.calls != 1 || !strings.Contains(m.View(), "Loading") {
		t.Fatalf("submit must defer I/O and show loading: calls=%d view=%s", r.calls, m.View())
	}
	r.todos = []*domain.Todo{{Title: "qjk milk beta"}, {Title: "qjk milk alpha"}, {Title: "done", Done: true}}
	m, _ = m.Update(cmd())
	if r.filter != "qjk milk" || r.calls != 2 {
		t.Fatalf("submitted List argument = %q, calls=%d", r.filter, r.calls)
	}
	assertSelected(t, m, "qjk milk alpha")
	if !strings.Contains(ansi.Strip(m.View()), "Filter: qjk milk") || !strings.Contains(m.View(), "DONE (1)") {
		t.Fatalf("filtered result lost query or collapsed count: %s", m.View())
	}
	m, _ = m.Update(key("j"))
	assertSelected(t, m, "qjk milk beta")
}

func TestDashboardFilterCancelRestoresAppliedView(t *testing.T) {
	t.Parallel()
	r := &todoReader{todos: []*domain.Todo{{Title: "milk alpha"}, {Title: "milk beta"}}}
	m := loaded(t, r, config.Default(), time.Now(), 100, 24)
	m, cmd := submitFilter(t, m, "milk")
	m, _ = m.Update(cmd())
	m, _ = m.Update(key("j"))
	before := m.View()
	m, _ = m.Update(key("/"))
	if !strings.Contains(ansi.Strip(m.View()), "Filter: milk") {
		t.Fatalf("editor did not reuse the applied query: %s", m.View())
	}
	m, _ = m.Update(key(" draft"))
	m, cmd = m.Update(key("esc"))
	if cmd != nil || r.calls != 2 || m.View() != before {
		t.Fatalf("cancel changed the applied view or performed I/O: calls=%d view=%s", r.calls, m.View())
	}
	assertSelected(t, m, "milk beta")
}

func TestDashboardFilterBlankClears(t *testing.T) {
	t.Parallel()
	for _, query := range []string{"", "  \u2003 "} {
		t.Run(fmt.Sprintf("query=%q", query), func(t *testing.T) {
			t.Parallel()
			r := &todoReader{todos: []*domain.Todo{{Title: "milk"}}}
			m := loaded(t, r, config.Default(), time.Now(), 100, 24)
			m, cmd := submitFilter(t, m, "milk")
			m, _ = m.Update(cmd())
			m, cmd = submitFilter(t, m, query)
			r.todos = []*domain.Todo{{Title: "alpha"}, {Title: "milk"}}
			m, _ = m.Update(cmd())
			if r.filter != "" || strings.Contains(ansi.Strip(m.View()), "Filter:") {
				t.Fatalf("blank did not clear the applied filter: query=%q view=%s", r.filter, m.View())
			}
			assertSelected(t, m, "alpha")
		})
	}
}

func TestDashboardFilterUnicodeEditing(t *testing.T) {
	t.Parallel()
	r := &todoReader{todos: []*domain.Todo{{Title: "чай🙂"}}}
	m := loaded(t, r, config.Default(), time.Now(), 80, 24)
	m, _ = m.Update(key("/"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("чай界"), Paste: true})
	m, _ = m.Update(key("backspace"))
	m, _ = m.Update(key("🙂"))
	if !strings.Contains(ansi.Strip(m.View()), "Filter: чай🙂") {
		t.Fatalf("Unicode paste/backspace corrupted text: %s", m.View())
	}
	m, cmd := m.Update(key("enter"))
	if cmd == nil {
		t.Fatal("Unicode query did not submit")
	}
	m, _ = m.Update(cmd())
	if r.filter != "чай🙂" {
		t.Fatalf("Unicode query = %q", r.filter)
	}
	assertSelected(t, m, "чай🙂")
}

func TestDashboardFilterRemappedBindings(t *testing.T) {
	t.Parallel()
	for _, filter := range []string{"ctrl+f", "space"} {
		t.Run(filter, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Keys.Filter, cfg.Keys.Cancel = []string{filter}, []string{"ctrl+x"}
			cfg.Keys.Up, cfg.Keys.Down, cfg.Keys.Quit = []string{"ctrl+p"}, []string{"ctrl+n"}, []string{"ctrl+c"}
			r := &todoReader{todos: []*domain.Todo{{Title: "alpha"}, {Title: "beta"}}}
			m := loaded(t, r, cfg, time.Now(), 100, 24)
			// Changing caller-owned slices must not change this model's bindings.
			cfg.Keys.Filter[0], cfg.Keys.Cancel[0] = "f", "c"
			assertHelp(t, m, []string{filter + " filter"}, []string{"/ filter"})
			before := m.View()
			m, _ = m.Update(key("/"))
			if m.View() != before {
				t.Fatal("superseded filter key opened the editor")
			}
			m, _ = m.Update(key(filter))
			m, _ = m.Update(key("q"))
			m, _ = m.Update(key("esc"))
			assertHelp(t, m, []string{"ctrl+x cancel"}, []string{"esc cancel"})
			if !strings.Contains(ansi.Strip(m.View()), "Filter: q") {
				t.Fatal("unconfigured Esc canceled or q was not text")
			}
			_, cmd := m.Update(key("ctrl+c"))
			assertQuit(t, cmd)
			m, cmd = m.Update(key("ctrl+x"))
			if cmd != nil || r.calls != 1 || m.View() != before {
				t.Fatal("configured cancel did not restore the view without I/O")
			}
			m, _ = m.Update(key(filter))
			m, _ = m.Update(key("beta"))
			m, cmd = m.Update(key("enter"))
			if cmd == nil {
				t.Fatal("remapped filter did not submit")
			}
			m, _ = m.Update(cmd())
			m, _ = m.Update(key("ctrl+n"))
			assertSelected(t, m, "beta")
			m, _ = m.Update(key("ctrl+p"))
			assertSelected(t, m, "alpha")
		})
	}
}

func TestDashboardFilterConfiguredQuitKeys(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		binding string
		msg     tea.KeyMsg
	}{
		{"ctrl+x", tea.KeyMsg{Type: tea.KeyCtrlX}},
		{"esc", tea.KeyMsg{Type: tea.KeyEsc}},
		{"up", tea.KeyMsg{Type: tea.KeyUp}},
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}},
		{"alt+q", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}, Alt: true}},
		{"alt+ ", tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}, Alt: true}},
	} {
		t.Run(tc.binding, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Keys.Quit = []string{"q", "space", tc.binding}
			r := &todoReader{todos: []*domain.Todo{{Title: "alpha"}}}
			m := loaded(t, r, cfg, time.Now(), 100, 24)
			m, _ = m.Update(key("/"))
			m, _ = m.Update(key("q"))
			m, _ = m.Update(key("space"))
			m, _ = m.Update(key("milk"))
			m, cmd := m.Update(tc.msg)
			assertQuit(t, cmd)
			if r.calls != 1 || !strings.Contains(ansi.Strip(m.View()), "Filter: q milk") {
				t.Fatalf("quit read data or changed the draft: calls=%d view=%s", r.calls, m.View())
			}
			assertHelp(t, m, []string{strings.TrimSpace(tc.binding) + " quit"},
				[]string{"q/space", "space quit", "ctrl+c quit"})
			if tc.binding == "enter" {
				assertHelp(t, m, nil, []string{"enter submit"})
			}
			if tc.binding == "esc" {
				assertHelp(t, m, nil, []string{"esc cancel"})
			}
		})
	}
}

func TestDashboardFilterPrintableQuitKeysRemainText(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Keys.Quit, cfg.Keys.Cancel = []string{"q", "space", "esc"}, nil
	r := &todoReader{todos: []*domain.Todo{{Title: "q esc"}}}
	m := loaded(t, r, cfg, time.Now(), 100, 24)
	m, _ = m.Update(key("/"))
	for _, msg := range []tea.KeyMsg{
		key("q"), key("space"),
		{Type: tea.KeyRunes, Runes: []rune("esc")},
	} {
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		if cmd != nil || !strings.Contains(ansi.Strip(m.View()), "Filter:") {
			t.Fatal("configured printable quit key intercepted draft text")
		}
	}
	assertHelp(t, m, []string{"esc quit"}, []string{"q quit", "space quit", "cancel"})
	m, cmd := m.Update(key("enter"))
	if cmd == nil || r.calls != 1 {
		t.Fatal("printable draft did not submit through a deferred load")
	}
	m, _ = m.Update(cmd())
	if r.filter != "q esc" {
		t.Fatalf("printable quit keys changed submitted query: %q", r.filter)
	}
	assertSelected(t, m, "q esc")
}

func TestDashboardFilterQuitHelpAndCancelPrecedence(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Keys.Quit = []string{"q", "space", "ctrl+x", "alt+q", "esc"}
	cfg.Keys.Cancel = []string{"esc", "ctrl+x", "ctrl+p"}
	r := &todoReader{todos: []*domain.Todo{{Title: "alpha"}}}
	m := loaded(t, r, cfg, time.Now(), 120, 24)
	before := m.View()
	m, _ = m.Update(key("/"))
	assertHelp(t, m, []string{"ctrl+x/alt+q/esc quit", "ctrl+p cancel"},
		[]string{"esc/ctrl+x", "q/space", "ctrl+c quit"})
	m, cmd := m.Update(key("ctrl+p"))
	if cmd != nil || m.View() != before || r.calls != 1 {
		t.Fatal("remaining configured cancel did not restore the view without I/O")
	}
	m, _ = m.Update(key("/"))
	_, cmd = m.Update(key("ctrl+x"))
	assertQuit(t, cmd)
	_, cmd = m.Update(key("esc"))
	assertQuit(t, cmd)
}

func TestDashboardFilterDisabledBindings(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Keys.Filter, cfg.Keys.Quit = nil, nil
	r := &todoReader{todos: []*domain.Todo{{Title: "alpha"}}}
	m := loaded(t, r, cfg, time.Now(), 80, 24)
	before := m.View()
	m, cmd := m.Update(key("/"))
	if cmd != nil || m.View() != before {
		t.Fatal("disabled filter opened")
	}
	assertHelp(t, m, nil, []string{"filter", "quit"})
	cfg.Keys.Filter = []string{"/"}
	m = loaded(t, r, cfg, time.Now(), 80, 24)
	m, _ = m.Update(key("/"))
	m, cmd = m.Update(key("ctrl+c"))
	if cmd != nil || !strings.Contains(ansi.Strip(m.View()), "Filter:") {
		t.Fatal("disabled Ctrl+C quit while editing")
	}
}

func TestDashboardFilterIgnoresStaleReadsWhileLoading(t *testing.T) {
	t.Parallel()
	r := &todoReader{todos: []*domain.Todo{{Title: "stale snapshot"}}}
	cfg := config.Default()
	var m tea.Model = ui.NewDashboard(r, cfg.Theme, cfg.Keys, time.Now())
	initial := m.Init()
	m, older := submitFilter(t, m, "older")
	m, newest := submitFilter(t, m, "newest")
	loading := m.View()
	for _, stale := range []struct {
		cmd   tea.Cmd
		query string
	}{{initial, ""}, {older, "older"}} {
		m, _ = m.Update(stale.cmd())
		if r.filter != stale.query || m.View() != loading {
			t.Fatalf("stale load replaced pending filter: query=%q view=%s", r.filter, m.View())
		}
	}
	r.todos = []*domain.Todo{{Title: "newest match"}}
	m, _ = m.Update(newest())
	if r.filter != "newest" {
		t.Fatalf("latest command captured the wrong query: %q", r.filter)
	}
	assertSelected(t, m, "newest match")
}

func TestDashboardFilterLatestResponseWins(t *testing.T) {
	t.Parallel()
	for _, staleError := range []bool{false, true} {
		t.Run(strconv.FormatBool(staleError), func(t *testing.T) {
			t.Parallel()
			r := &todoReader{todos: []*domain.Todo{{Title: "initial snapshot"}}}
			cfg := config.Default()
			var m tea.Model = ui.NewDashboard(r, cfg.Theme, cfg.Keys, time.Now())
			initial := m.Init()
			m, older := submitFilter(t, m, "older")
			m, newest := submitFilter(t, m, "newest")
			r.todos = []*domain.Todo{{Title: "newest match"}}
			m, _ = m.Update(newest())
			assertSelected(t, m, "newest match")
			before := m.View()
			if staleError {
				r.err = errors.New("stale read failed")
			} else {
				r.todos = []*domain.Todo{{Title: "stale match"}}
			}
			for _, stale := range []tea.Cmd{older, initial} {
				m, _ = m.Update(stale())
				if m.View() != before {
					t.Fatalf("stale response overwrote newest search: %s", m.View())
				}
			}
		})
	}
}

func TestDashboardFilterFeedback(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, want string
		err        error
		todos      []*domain.Todo
	}{
		{name: "no matches", want: "No matches"},
		{name: "error", want: "database unavailable", err: errors.New("database unavailable")},
		{name: "completed only", want: "No open todos", todos: []*domain.Todo{{Title: "done", Done: true}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := &todoReader{}
			m := loaded(t, r, config.Default(), time.Now(), 80, 24)
			m, cmd := submitFilter(t, m, "needle")
			r.err, r.todos = tc.err, tc.todos
			m, _ = m.Update(cmd())
			view := ansi.Strip(m.View())
			if !strings.Contains(view, tc.want) || !strings.Contains(view, "Filter: needle") {
				t.Fatalf("missing filter feedback %q: %s", tc.want, view)
			}
			_, quit := m.Update(key("q"))
			assertQuit(t, quit)
		})
	}
}

func TestDashboardFilterSmallWindows(t *testing.T) {
	t.Parallel()
	m := loaded(t, &todoReader{todos: []*domain.Todo{{Title: "alpha"}}}, config.Default(), time.Now(), 80, 24)
	m, _ = m.Update(key("/"))
	m, _ = m.Update(key(strings.Repeat("界🙂", 100)))
	for _, size := range []tea.WindowSizeMsg{
		{Width: 80, Height: 24}, {Width: 16, Height: 6}, {Width: 8, Height: 2},
		{Width: 1, Height: 1}, {Width: 30, Height: 1}, {Width: 30, Height: 3},
		{Width: 0, Height: 0}, {Width: -1, Height: -2}, {Width: 100, Height: 24},
	} {
		m, _ = m.Update(size)
		assertBounds(t, m.View(), size.Width, size.Height)
		if size.Width > 0 && size.Height > 0 && !strings.Contains(ansi.Strip(m.View()), "F") {
			t.Fatalf("tiny window lost editing prompt: size=%+v view=%s", size, m.View())
		}
	}
}

func TestDashboardFilterScrollAndCancel(t *testing.T) {
	t.Parallel()
	r := &todoReader{}
	m := loaded(t, r, config.Default(), time.Now(), 30, 8)
	m, cmd := submitFilter(t, m, "task")
	for i := range 30 {
		r.todos = append(r.todos, &domain.Todo{ID: strconv.Itoa(i), Title: fmt.Sprintf("task %02d", i)})
	}
	r.todos = append(r.todos, &domain.Todo{Title: "hidden done", Done: true})
	m, _ = m.Update(cmd())
	for range 30 {
		m, _ = m.Update(key("j"))
		assertBounds(t, m.View(), 30, 8)
		assertVisibleSelection(t, m)
	}
	assertSelected(t, m, "task 29")
	if !strings.Contains(m.View(), "DONE (1)") {
		t.Fatalf("filtered scrolling lost completed count: %s", m.View())
	}
	before := m.View()
	m, _ = m.Update(key("/"))
	m, _ = m.Update(key("j"))
	m, cmd = m.Update(key("esc"))
	if cmd != nil || m.View() != before || r.calls != 2 {
		t.Fatal("cancel did not preserve scrolled filtered view without I/O")
	}
	m, _ = m.Update(tea.WindowSizeMsg{Width: 16, Height: 3})
	assertSelected(t, m, "task 29")
	assertBounds(t, m.View(), 16, 3)
}

func submitFilter(t *testing.T, m tea.Model, query string) (tea.Model, tea.Cmd) {
	t.Helper()
	m, _ = m.Update(key("/"))
	m, _ = m.Update(key("ctrl+u"))
	m, _ = m.Update(key(query))
	m, cmd := m.Update(key("enter"))
	if cmd == nil {
		t.Fatal("filter submission did not return a load command")
	}
	return m, cmd
}

func loaded(t *testing.T, reader *todoReader, cfg *config.Config, now time.Time, width, height int) tea.Model {
	t.Helper()
	var m tea.Model = ui.NewDashboard(reader, cfg.Theme, cfg.Keys, now)
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("missing load command")
	}
	m, _ = m.Update(cmd())
	return m
}

func key(name string) tea.KeyMsg {
	switch name {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "ctrl+n":
		return tea.KeyMsg{Type: tea.KeyCtrlN}
	case "ctrl+p":
		return tea.KeyMsg{Type: tea.KeyCtrlP}
	case "ctrl+f":
		return tea.KeyMsg{Type: tea.KeyCtrlF}
	case "ctrl+x":
		return tea.KeyMsg{Type: tea.KeyCtrlX}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
	}
}

func timestamp(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func assertSelected(t *testing.T, m tea.Model, title string) {
	t.Helper()
	if !strings.Contains(ansi.Strip(m.View()), "> [ ] "+title) {
		t.Fatalf("selected row should be %q:\n%s", title, m.View())
	}
}

func assertVisibleSelection(t *testing.T, m tea.Model) {
	t.Helper()
	count := 0
	for line := range strings.SplitSeq(ansi.Strip(m.View()), "\n") {
		if strings.HasPrefix(line, ">") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one visible selected row: %q", m.View())
	}
}

func assertViewport(t *testing.T, m tea.Model, size tea.WindowSizeMsg) {
	t.Helper()
	assertBounds(t, m.View(), size.Width, size.Height)
	if size.Width > 0 && size.Height > 0 {
		assertVisibleSelection(t, m)
	}
}

func assertInactiveKeys(t *testing.T, m tea.Model, keys []string, selected string) {
	t.Helper()
	for _, k := range keys {
		var cmd tea.Cmd
		m, cmd = m.Update(key(k))
		if cmd != nil {
			t.Fatalf("inactive binding %q returned a command", k)
		}
		assertSelected(t, m, selected)
	}
}

func assertHelp(t *testing.T, m tea.Model, bindings, excluded []string) {
	t.Helper()
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	footer := lines[len(lines)-1]
	for _, binding := range bindings {
		if !strings.Contains(footer, binding) {
			t.Fatalf("footer omits %q: %s", binding, footer)
		}
	}
	for _, action := range excluded {
		if strings.Contains(footer, action) {
			t.Fatalf("footer advertises inactive %q: %s", action, footer)
		}
	}
}

func assertQuit(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("quit did not return a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("quit command did not produce QuitMsg")
	}
}

func assertBounds(t *testing.T, view string, width, height int) {
	t.Helper()
	if width <= 0 || height <= 0 {
		if view != "" {
			t.Fatalf("nonpositive window rendered %q", view)
		}
		return
	}
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		t.Fatalf("%d rendered lines exceed height %d: %q", len(lines), height, view)
	}
	for _, line := range lines {
		if got := ansi.StringWidth(line); got > width {
			t.Fatalf("row width %d exceeds %d: %q", got, width, line)
		}
	}
}

func renderedGroups(view string) map[string][]string {
	out := make(map[string][]string)
	group := ""
	for line := range strings.SplitSeq(ansi.Strip(view), "\n") {
		for _, name := range []string{"OVERDUE", "TODAY", "TOMORROW", "LATER"} {
			if strings.HasPrefix(line, name) {
				group = name
			}
		}
		if _, title, found := strings.Cut(line, "[ ] "); found {
			out[group] = append(out[group], strings.TrimSpace(title))
		}
	}
	return out
}
