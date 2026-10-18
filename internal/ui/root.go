package ui

import (
	"errors"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vladgrskkh/psst/internal/config"
	"github.com/vladgrskkh/psst/internal/domain"
	"github.com/vladgrskkh/psst/internal/service"
)

// FormTodos is the local todo surface needed by the shared create/edit form.
type FormTodos interface {
	List(string) ([]*domain.Todo, error)
	Create(service.CreateTodoParams) (*domain.Todo, error)
	Update(string, service.UpdateTodoParams) (*domain.Todo, error)
}

type Root struct {
	dashboard                Dashboard
	form                     Form
	active                   rootScreen
	formTodos                FormTodos
	theme                    config.ThemeConfig
	keys                     config.KeysConfig
	now                      time.Time
	defaultList, defaultTime string
	width, height            int
	generation               uint64
	finalErr                 error
	quitAfterSave            bool
}

type rootScreen uint8

const (
	screenDashboard rootScreen = iota
	screenForm
)

type formRequested struct{ editID string }
type formLoaded struct {
	generation uint64
	todos      []*domain.Todo
	err        error
}
type formSaved struct {
	generation uint64
	err        error
}
type formCancelled struct{ generation uint64 }

// NewRoot composes the dashboard and shared create/edit form.
func NewRoot(
	dashboardTodos DashboardTodos,
	formTodos FormTodos,
	theme config.ThemeConfig,
	keys config.KeysConfig,
	now time.Time,
	defaultList, defaultTime string,
) Root {
	keys.New = slices.Clone(keys.New)
	keys.Edit = slices.Clone(keys.Edit)
	keys.Cancel = slices.Clone(keys.Cancel)
	keys.Quit = slices.Clone(keys.Quit)
	return Root{
		dashboard:   NewDashboard(dashboardTodos, theme, keys, now),
		formTodos:   formTodos,
		theme:       theme,
		keys:        keys,
		now:         now,
		defaultList: defaultList,
		defaultTime: defaultTime,
		width:       defaultWidth,
		height:      defaultHeight,
	}
}
func (m Root) Init() tea.Cmd { return m.dashboard.Init() }
func (m Root) Err() error    { return errors.Join(m.finalErr, m.dashboard.Err()) }
func (m Root) View() string {
	if m.active == screenForm {
		return m.form.View()
	}
	return m.dashboard.View()
}

func (m Root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case formRequested:
		return m.openForm(msg)
	case formLoaded:
		return m.loadForm(msg)
	case formSaved:
		return m.finishFormSave(msg)
	case formCancelled:
		return m.cancelForm(msg)
	case todosLoaded, todoToggled, todoDeleted:
		return m.updateDashboard(msg)
	case tea.WindowSizeMsg:
		return m.resize(msg)
	case tea.KeyMsg:
		return m.updateKey(msg)
	default:
		return m.updateActive(msg)
	}
}

func (m Root) openForm(msg formRequested) (tea.Model, tea.Cmd) {
	m.generation++
	m.active = screenForm
	m.form = newForm(
		m.formTodos,
		m.theme,
		m.keys,
		m.now,
		m.defaultList,
		m.defaultTime,
		m.generation,
		msg.editID,
	)
	m.form, _ = m.form.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	return m, m.form.load()
}

func (m Root) loadForm(msg formLoaded) (tea.Model, tea.Cmd) {
	if m.active != screenForm || msg.generation != m.generation {
		return m, nil
	}
	m.form, _ = m.form.Update(msg)
	return m, nil
}

func (m Root) cancelForm(msg formCancelled) (tea.Model, tea.Cmd) {
	if m.active == screenForm && msg.generation == m.generation {
		m.active = screenDashboard
	}
	return m, nil
}

func (m Root) resize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width, m.height = max(0, msg.Width), max(0, msg.Height)
	dashboardModel, cmd := m.dashboard.Update(msg)
	dashboard, ok := dashboardModel.(Dashboard)
	if ok {
		m.dashboard = dashboard
	}
	if m.active == screenForm {
		m.form, _ = m.form.Update(msg)
		return m, nil
	}
	return m, cmd
}

func (m Root) finishFormSave(msg formSaved) (tea.Model, tea.Cmd) {
	if m.active != screenForm || msg.generation != m.generation {
		return m, nil
	}
	m.form, _ = m.form.Update(msg)
	if msg.err != nil {
		m.finalErr = safeMutationError{err: msg.err}
		if m.quitAfterSave {
			return m, tea.Quit
		}
		return m, nil
	}
	m.finalErr = nil
	if m.quitAfterSave {
		return m, tea.Quit
	}
	if m.dashboard.hasSelectedTodo() {
		m.dashboard.restoreID = m.dashboard.rows[m.dashboard.cursor].id
	}
	m.active = screenDashboard
	m.dashboard.request++
	m.dashboard.rows = []dashboardRow{{text: loadingTodosText, style: m.dashboard.styles.muted}}
	m.dashboard.cursor, m.dashboard.offset = -1, 0
	return m, m.dashboard.loadTodos()
}

func (m Root) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.active == screenForm {
		return m.updateFormKey(msg)
	}
	if cmd, routed := m.routeDashboardKey(msg); routed {
		return m, cmd
	}
	return m.updateDashboard(msg)
}

func (m Root) updateFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.isFormQuit(msg) {
		if m.form.busy {
			m.quitAfterSave = true
			return m, nil
		}
		return m, tea.Quit
	}
	if m.form.busy {
		return m, nil
	}
	var cmd tea.Cmd
	m.form, cmd = m.form.Update(msg)
	if m.form.busy {
		m.finalErr = nil
	}
	return m, cmd
}

func (m Root) routeDashboardKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if msg.Paste || m.dashboard.input.Focused() || m.dashboard.phase != phaseIdle {
		return nil, false
	}
	name := msg.String()
	if name == " " {
		name = keySpace
	}
	if slices.Contains(m.keys.Quit, name) || slices.Contains(m.keys.Filter, name) {
		return nil, false
	}
	if slices.Contains(m.keys.New, name) {
		return func() tea.Msg { return formRequested{} }, true
	}
	if slices.Contains(m.keys.Edit, name) && m.dashboard.hasSelectedTodo() {
		id := m.dashboard.rows[m.dashboard.cursor].id
		return func() tea.Msg { return formRequested{editID: id} }, true
	}
	return nil, false
}

func (m Root) updateDashboard(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.dashboard.Update(msg)
	dashboard, ok := model.(Dashboard)
	if !ok {
		return m, nil
	}
	m.dashboard = dashboard
	return m, cmd
}

func (m Root) updateActive(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.active == screenForm {
		var cmd tea.Cmd
		m.form, cmd = m.form.Update(msg)
		return m, cmd
	}
	return m.updateDashboard(msg)
}

func (m Root) isFormQuit(msg tea.KeyMsg) bool {
	name := msg.String()
	return !msg.Paste && (msg.Alt || (msg.Type != tea.KeyRunes && msg.Type != tea.KeySpace)) &&
		slices.Contains(m.dashboard.filterQuitKeys(), name)
}

var _ tea.Model = Root{}

type safeMutationError struct{ err error }

func (e safeMutationError) Error() string { return singleLine(e.err.Error()) }
func (e safeMutationError) Unwrap() error { return e.err }
