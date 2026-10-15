// Package ui presents the local todo dashboard.
package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/vladgrskkh/psst/internal/config"
	"github.com/vladgrskkh/psst/internal/domain"
)

// DashboardTodos supplies local snapshots and mutations for the dashboard.
type DashboardTodos interface {
	List(string) ([]*domain.Todo, error)
	ToggleDone(string) (*domain.Todo, error)
	Delete(string) error
}

// Dashboard is the local dashboard model.
type Dashboard struct {
	reader        DashboardTodos
	now           time.Time
	keys          config.KeysConfig
	styles        dashboardStyles
	rows          []dashboardRow
	cursor        int
	offset        int
	width         int
	height        int
	help          string
	input         textinput.Model
	query         string
	request       uint64
	action        dashboardAction
	deleteTarget  dashboardTarget
	actionError   string
	finalErr      error
	quitRequested bool
}

type dashboardRow struct {
	id         string
	title      string
	text       string
	style      lipgloss.Style
	selectable bool
}

type todosLoaded struct {
	request uint64
	todos   []*domain.Todo
	err     error
}

const (
	overdueGroup = iota
	todayGroup
	tomorrowGroup
	laterGroup
	groupCount
)

const (
	defaultWidth  = 80
	defaultHeight = 24
	chromeHeight  = 2
)

// NewDashboard constructs the dashboard with a fixed launch reference time.
func NewDashboard(reader DashboardTodos, theme config.ThemeConfig, keys config.KeysConfig, now time.Time) Dashboard {
	styles := newDashboardStyles(theme)
	input := textinput.New()
	input.Prompt = "Filter: "
	input.PromptStyle, input.TextStyle = styles.header, styles.text
	input.PlaceholderStyle, input.CompletionStyle = styles.muted, styles.muted
	input.Cursor.Style, input.Cursor.TextStyle = styles.selected, styles.text
	input.Cursor.SetMode(cursor.CursorStatic)
	input.Width = defaultWidth - ansi.StringWidth(input.Prompt) - 1
	return Dashboard{
		reader: reader,
		now:    now,
		keys: config.KeysConfig{
			Up:         slices.Clone(keys.Up),
			Down:       slices.Clone(keys.Down),
			Quit:       slices.Clone(keys.Quit),
			Filter:     slices.Clone(keys.Filter),
			Cancel:     slices.Clone(keys.Cancel),
			ToggleDone: slices.Clone(keys.ToggleDone),
			Delete:     slices.Clone(keys.Delete),
			Confirm:    slices.Clone(keys.Confirm),
		},
		styles:  styles,
		rows:    []dashboardRow{{text: "Loading todos…", style: styles.muted}},
		cursor:  -1,
		width:   defaultWidth,
		height:  defaultHeight,
		help:    dashboardHelp(keys),
		input:   input,
		request: 1,
	}
}

// Init returns the initial snapshot-loading command.
func (m Dashboard) Init() tea.Cmd {
	return m.loadTodos()
}

// Err returns the unresolved mutation failure for reporting after terminal restoration.
func (m Dashboard) Err() error {
	return m.finalErr
}

func (m Dashboard) loadTodos() tea.Cmd {
	return func() tea.Msg {
		todos, err := m.reader.List(m.query)
		return todosLoaded{request: m.request, todos: todos, err: err}
	}
}

// Update handles dashboard messages.
func (m Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Quit commands run asynchronously; no later message may accept more work.
	if m.action == actionQuitting {
		return m, nil
	}
	switch msg := msg.(type) {
	case todosLoaded:
		if msg.request != m.request {
			return m, nil
		}
		if m.action == actionReloading {
			m.action = actionIdle
		}
		m.cursor, m.offset = -1, 0
		if msg.err != nil {
			m.rows = []dashboardRow{{
				text: "Could not load todos: " + singleLine(msg.err.Error()), style: m.styles.error,
			}}
		} else {
			m.rows, m.cursor = m.dashboardRows(msg.todos)
		}
	case todoToggled:
		return m.finishMutation(msg.operation, msg.err, "toggle todo")
	case todoDeleted:
		return m.finishMutation(msg.operation, msg.err, "delete todo")
	case tea.WindowSizeMsg:
		m.width, m.height = max(0, msg.Width), max(0, msg.Height)
		m.input.Width = max(1, m.width-ansi.StringWidth(m.input.Prompt)-1)
		m.input.SetCursor(m.input.Position())
	case tea.KeyMsg:
		return m.updateDashboardKey(msg)
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	m.offset = m.visibleOffset()
	return m, nil
}

func (m Dashboard) updateFilter(msg tea.KeyMsg, name string) (tea.Model, tea.Cmd) {
	switch {
	case !msg.Paste && (msg.Alt || (msg.Type != tea.KeyRunes && msg.Type != tea.KeySpace)) &&
		slices.Contains(m.filterQuitKeys(), name):
		m.action = actionQuitting
		return m, tea.Quit
	case slices.Contains(m.keys.Cancel, name):
		m.input.Blur()
		return m, nil
	case name == "enter":
		m.query = strings.TrimSpace(m.input.Value())
		m.input.Blur()
		m.request++
		m.rows = []dashboardRow{{text: "Loading todos…", style: m.styles.muted}}
		m.cursor, m.offset = -1, 0
		return m, m.loadTodos()
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

// filterQuitKeys excludes printable bindings reserved for draft text.
func (m Dashboard) filterQuitKeys() []string {
	var keys []string
	for _, name := range m.keys.Quit {
		if name != "space" && utf8.RuneCountInString(name) > 1 {
			keys = append(keys, name)
		}
	}
	return keys
}

func (m Dashboard) dashboardRows(todos []*domain.Todo) ([]dashboardRow, int) {
	var groups [groupCount][]domain.Todo
	done, open := 0, 0
	for _, todo := range todos {
		if todo == nil || todo.Deleted {
			continue
		}
		if todo.Done {
			done++
			continue
		}
		group := deadlineGroup(*todo, m.now)
		groups[group] = append(groups[group], *todo)
		open++
	}
	rows := make([]dashboardRow, 0, open+groupCount+1)
	cursor := -1
	if open == 0 {
		message := "No todos."
		if done > 0 {
			message = "No open todos."
		} else if m.query != "" {
			message = "No matches."
		}
		rows = append(rows, dashboardRow{text: message, style: m.styles.muted})
	}
	for group, label := range [groupCount]string{"OVERDUE", "TODAY", "TOMORROW", "LATER"} {
		slices.SortFunc(groups[group], compareTodos)
		rows = append(rows, dashboardRow{
			text: fmt.Sprintf("%s (%d)", label, len(groups[group])), style: m.styles.groups[group],
		})
		for _, todo := range groups[group] {
			if cursor < 0 {
				cursor = len(rows)
			}
			rows = append(rows, dashboardRow{
				id: todo.ID, title: singleLine(todo.Title),
				text: "[ ] " + singleLine(todo.Title), style: m.styles.text, selectable: true,
			})
		}
	}
	rows = append(rows, dashboardRow{text: fmt.Sprintf("DONE (%d) · collapsed", done), style: m.styles.muted})
	return rows, cursor
}

func (m Dashboard) nextCursor(direction int) int {
	if m.cursor < 0 {
		return m.cursor
	}
	for row := m.cursor + direction; row >= 0 && row < len(m.rows); row += direction {
		if m.rows[row].selectable {
			return row
		}
	}
	return m.cursor
}

func (m Dashboard) visibleOffset() int {
	height := m.contentHeight()
	if height == 0 {
		return m.offset
	}
	offset := m.offset
	if m.cursor >= 0 {
		if m.cursor < offset {
			offset = m.cursor
		} else if m.cursor >= offset+height {
			offset = m.cursor - height + 1
		}
		// At the final selectable todo, reveal trailing sections without hiding the cursor.
		if m.nextCursor(1) == m.cursor {
			offset = min(m.cursor, max(0, len(m.rows)-height))
		}
	}
	return min(max(0, offset), max(0, len(m.rows)-height))
}

func deadlineGroup(todo domain.Todo, now time.Time) int {
	deadline, present := todo.Deadline.Get()
	if !present {
		return laterGroup
	}
	if deadline.Before(now) {
		return overdueGroup
	}
	local := deadline.In(now.Location())
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrow := start.AddDate(0, 0, 1)
	if local.Before(tomorrow) {
		return todayGroup
	}
	if local.Before(tomorrow.AddDate(0, 0, 1)) {
		return tomorrowGroup
	}
	return laterGroup
}

// compareTodos orders deadlines first, then titles and IDs to break ties.
func compareTodos(a, b domain.Todo) int {
	aDue, aPresent := a.Deadline.Get()
	bDue, bPresent := b.Deadline.Get()
	if aPresent != bPresent {
		if aPresent {
			return -1
		}
		return 1
	}
	if order := aDue.Compare(bDue); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Title, b.Title); order != 0 {
		return order
	}
	return cmp.Compare(a.ID, b.ID)
}
