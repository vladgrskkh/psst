package ui

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type dashboardPhase uint8

const (
	phaseIdle dashboardPhase = iota
	phaseConfirmDelete
	phaseSaving
	phaseReloading
	phaseQuitting
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

type dashboardMutationError struct {
	message string
	cause   error
}

func (e dashboardMutationError) Error() string { return e.message }

func (e dashboardMutationError) Unwrap() error { return e.cause }

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
	if m.phase != phaseIdle {
		return m.updatePhaseKey(name)
	}
	switch {
	case slices.Contains(m.keys.Quit, name):
		m.phase = phaseQuitting
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
			m.phase = phaseConfirmDelete
			m.actionError = ""
			row := m.rows[m.cursor]
			m.deleteTarget = dashboardTarget{id: row.id, title: row.title}
		}
	}
	m.offset = m.visibleOffset()
	return m, nil
}

func (m Dashboard) toggleTodo(id string) (tea.Model, tea.Cmd) {
	m.phase, m.actionError = phaseSaving, ""
	m.finalErr = nil
	m.offset = m.visibleOffset()
	m.request++ // Invalidate every read captured before the mutation.
	return m, func() tea.Msg {
		_, err := m.reader.ToggleDone(id)
		return todoToggled{operation: m.request, err: err}
	}
}

func (m Dashboard) deleteTodo() (tea.Model, tea.Cmd) {
	m.phase, m.actionError = phaseSaving, ""
	m.finalErr = nil
	m.offset = m.visibleOffset()
	m.request++
	id := m.deleteTarget.id
	m.deleteTarget = dashboardTarget{}
	return m, func() tea.Msg {
		return todoDeleted{operation: m.request, err: m.reader.Delete(id)}
	}
}

func (m Dashboard) finishMutation(operation uint64, err error, action string) (tea.Model, tea.Cmd) {
	if m.phase != phaseSaving || operation != m.request {
		return m, nil
	}
	var cmd tea.Cmd
	if err != nil {
		m.actionError = "Could not " + action + ": " + singleLine(err.Error())
		m.finalErr = dashboardMutationError{message: m.actionError, cause: err}
		m.phase = phaseIdle
		m.offset = m.visibleOffset()
	} else {
		m.phase = phaseReloading
		m.request++ // A completion can be consumed only once.
		cmd = m.loadTodos()
	}
	// Bubble Tea does not join running commands on exit. Let accepted writes
	// finish before the application's database is closed. An explicit quit is
	// honored on failure too; the CLI reports Err after terminal restoration.
	if m.quitRequested {
		m.phase = phaseQuitting
		return m, tea.Quit
	}
	return m, cmd
}

func (m Dashboard) updatePhaseKey(name string) (tea.Model, tea.Cmd) {
	switch {
	case slices.Contains(m.keys.Quit, name):
		if m.phase == phaseSaving {
			m.quitRequested = true
			return m, nil
		}
		m.phase = phaseQuitting
		return m, tea.Quit
	case m.phase != phaseConfirmDelete:
		return m, nil
	case name == "n" || slices.Contains(m.keys.Cancel, name):
		m.phase, m.deleteTarget = phaseIdle, dashboardTarget{}
	case slices.Contains(m.keys.Confirm, name):
		return m.deleteTodo()
	}
	m.offset = m.visibleOffset()
	return m, nil
}

func (m Dashboard) phaseStatus() string {
	switch m.phase {
	case phaseConfirmDelete:
		return "Delete \"" + m.deleteTarget.title + "\"?"
	case phaseSaving:
		if m.quitRequested {
			return "Saving… quit requested"
		}
		return "Saving…"
	case phaseReloading:
		return "Reloading todos…"
	case phaseIdle:
		return m.actionError
	case phaseQuitting:
		if m.actionError != "" {
			return m.actionError
		}
		return "Quitting… · psst"
	}
	return ""
}

func (m Dashboard) phaseHelp() string {
	if m.phase == phaseQuitting {
		return ""
	}
	quit := m.keys.Quit
	if m.phase != phaseConfirmDelete {
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
