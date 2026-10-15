package ui

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type dashboardAction uint8

const (
	actionIdle dashboardAction = iota
	actionConfirmDelete
	actionSaving
	actionReloading
)

type dashboardTarget struct {
	id, title string
}

type todoToggled struct {
	operation uint64
	err       error
}

type todoDeleted struct {
	operation uint64
	err       error
}

func (m Dashboard) updateDashboardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	name := msg.String()
	if name == " " {
		name = "space"
	}
	if m.input.Focused() {
		return m.updateFilter(msg, name)
	}
	if msg.Paste {
		return m, nil
	}
	if m.action != actionIdle {
		return m.updateAction(name)
	}
	switch {
	case slices.Contains(m.keys.Quit, name):
		return m, tea.Quit
	case slices.Contains(m.keys.Filter, name):
		m.actionError = ""
		m.input.SetValue(m.query)
		m.input.CursorEnd()
		return m, m.input.Focus()
	case slices.Contains(m.keys.Up, name):
		m.cursor = m.nextCursor(-1)
	case slices.Contains(m.keys.Down, name):
		m.cursor = m.nextCursor(1)
	case slices.Contains(m.keys.ToggleDone, name):
		if m.cursor >= 0 && m.rows[m.cursor].selectable {
			return m.toggleTodo(m.rows[m.cursor].id)
		}
	case slices.Contains(m.keys.Delete, name):
		if m.cursor >= 0 && m.rows[m.cursor].selectable {
			m.action = actionConfirmDelete
			m.actionError = ""
			row := m.rows[m.cursor]
			m.deleteTarget = dashboardTarget{id: row.id, title: row.title}
		}
	}
	m.offset = m.visibleOffset()
	return m, nil
}

func (m Dashboard) toggleTodo(id string) (tea.Model, tea.Cmd) {
	m.action, m.actionError = actionSaving, ""
	m.request++ // Invalidate every read captured before the mutation.
	return m, func() tea.Msg {
		_, err := m.reader.ToggleDone(id)
		return todoToggled{operation: m.request, err: err}
	}
}

func (m Dashboard) deleteTodo() (tea.Model, tea.Cmd) {
	m.action, m.actionError = actionSaving, ""
	m.request++
	id := m.deleteTarget.id
	m.deleteTarget = dashboardTarget{}
	return m, func() tea.Msg {
		return todoDeleted{operation: m.request, err: m.reader.Delete(id)}
	}
}

func (m Dashboard) finishMutation(operation uint64, err error, action string) (tea.Model, tea.Cmd) {
	if m.action != actionSaving || operation != m.request {
		return m, nil
	}
	// Bubble Tea does not join running commands on exit. Let accepted writes
	// finish before the application's database is closed.
	if m.quitRequested {
		m.action = actionIdle
		return m, tea.Quit
	}
	if err != nil {
		m.action = actionIdle
		m.actionError = "Could not " + action + ": " + singleLine(err.Error())
		return m, nil
	}
	m.action = actionReloading
	m.request++ // A completion can be consumed only once.
	return m, m.loadTodos()
}

func (m Dashboard) updateAction(name string) (tea.Model, tea.Cmd) {
	switch {
	case slices.Contains(m.keys.Quit, name):
		if m.action == actionSaving {
			m.quitRequested = true
			return m, nil
		}
		return m, tea.Quit
	case m.action != actionConfirmDelete:
		return m, nil
	case name == "n" || slices.Contains(m.keys.Cancel, name):
		m.action, m.deleteTarget = actionIdle, dashboardTarget{}
	case slices.Contains(m.keys.Confirm, name):
		return m.deleteTodo()
	}
	return m, nil
}

func (m Dashboard) actionStatus() string {
	switch m.action {
	case actionConfirmDelete:
		return "Delete \"" + m.deleteTarget.title + "\"?"
	case actionSaving:
		if m.quitRequested {
			return "Saving… quit requested"
		}
		return "Saving…"
	case actionReloading:
		return "Reloading todos…"
	case actionIdle:
		return m.actionError
	}
	return ""
}

func (m Dashboard) actionHelp() string {
	quit := m.keys.Quit
	if m.action != actionConfirmDelete {
		if len(quit) == 0 {
			return ""
		}
		return singleLine(strings.Join(quit, "/")) + " quit"
	}
	cancel := effectiveKeys(append(slices.Clone(m.keys.Cancel), "n"), quit)
	confirm := effectiveKeys(m.keys.Confirm, append(slices.Clone(quit), cancel...))
	var actions []string
	for _, binding := range []struct {
		keys   []string
		action string
	}{{confirm, "confirm"}, {cancel, "cancel"}, {quit, "quit"}} {
		if len(binding.keys) > 0 {
			actions = append(actions, singleLine(strings.Join(binding.keys, "/"))+" "+binding.action)
		}
	}
	return strings.Join(actions, " · ")
}

// effectiveKeys excludes bindings already claimed by a higher priority action.
func effectiveKeys(keys, claimed []string) []string {
	var effective []string
	for _, name := range keys {
		if !slices.Contains(claimed, name) && !slices.Contains(effective, name) {
			effective = append(effective, name)
		}
	}
	return effective
}
