package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/vladgrskkh/psst/internal/config"
	"github.com/vladgrskkh/psst/internal/dateparser"
	"github.com/vladgrskkh/psst/internal/domain"
	"github.com/vladgrskkh/psst/internal/service"
	"github.com/vladgrskkh/psst/pkg/option"
)

const (
	titleField = iota
	deadlineField
	descriptionField
	listField
)

type Form struct {
	reader        FormTodos
	keys          config.KeysConfig
	now           time.Time
	defaultTime   string
	defaultList   string
	generation    uint64
	id            string
	fields        [4]textinput.Model
	focus         int
	width, height int
	suggestions   []string
	suggestion    int
	loading, busy bool
	message       string
	writeErr      error
	styles        dashboardStyles
}

func newForm(
	reader FormTodos,
	theme config.ThemeConfig,
	keys config.KeysConfig,
	now time.Time,
	defaultList, defaultTime string,
	generation uint64,
	id string,
) Form {
	styles := newDashboardStyles(theme)
	labels := [4]string{"Title", "Deadline", "Description", "List"}
	fields := [4]textinput.Model{}
	for i := range fields {
		fields[i] = textinput.New()
		fields[i].Prompt = labels[i] + ": "
		fields[i].Width = defaultWidth - ansi.StringWidth(fields[i].Prompt) - 1
		fields[i].PromptStyle, fields[i].TextStyle = styles.header, styles.text
		fields[i].PlaceholderStyle = styles.muted
		fields[i].Cursor.Style, fields[i].Cursor.TextStyle = styles.selected, styles.text
		fields[i].Cursor.SetMode(cursor.CursorStatic)
	}
	listDefault := strings.TrimSpace(defaultList)
	fields[listField].Placeholder = listDefault
	fields[0].Focus()
	return Form{
		reader:      reader,
		keys:        keys,
		now:         now,
		defaultTime: defaultTime,
		defaultList: listDefault,
		generation:  generation,
		id:          id,
		fields:      fields,
		width:       defaultWidth,
		height:      defaultHeight,
		loading:     true,
		suggestion:  -1,
		styles:      styles,
	}
}
func (m Form) load() tea.Cmd {
	g, reader := m.generation, m.reader
	return func() tea.Msg {
		todos, err := reader.List("")
		return formLoaded{generation: g, todos: todos, err: err}
	}
}

//nolint:gocognit // Message-specific transitions stay explicit in the Elm model.
func (m Form) Update(msg tea.Msg) (Form, tea.Cmd) {
	switch msg := msg.(type) {
	case formLoaded:
		if msg.generation != m.generation {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.message = safeError(msg.err)
			return m, nil
		}
		seen := map[string]bool{}
		for _, todo := range msg.todos {
			if todo == nil || todo.Deleted {
				continue
			}
			name := strings.TrimSpace(todo.ListName)
			if name != "" && !seen[name] {
				seen[name] = true
				m.suggestions = append(m.suggestions, name)
			}
			if todo.ID == m.id {
				m = m.fill(*todo)
			}
		}
		if m.defaultList != "" && !seen[m.defaultList] {
			seen[m.defaultList] = true
			m.suggestions = append(m.suggestions, m.defaultList)
		}
		if name := strings.TrimSpace(m.fields[3].Value()); name != "" && !seen[name] {
			m.suggestions = append(m.suggestions, name)
		}
		slices.Sort(m.suggestions)
	case formSaved:
		if msg.generation != m.generation {
			return m, nil
		}
		m.busy = false
		if msg.err != nil {
			m.writeErr = msg.err
			m.message = "Could not save: " + safeError(msg.err)
		}
	case tea.WindowSizeMsg:
		m.width, m.height = max(0, msg.Width), max(0, msg.Height)
		for i := range m.fields {
			m.fields[i].Width = max(1, m.width-ansi.StringWidth(m.fields[i].Prompt)-1)
		}
	case tea.KeyMsg:
		return m.updateKey(msg)
	default:
		var cmd tea.Cmd
		m.fields[m.focus], cmd = m.fields[m.focus].Update(msg)
		m.message = m.dateFeedback()
		return m, cmd
	}
	return m, nil
}
func (m Form) fill(todo domain.Todo) Form {
	m.fields[0].SetValue(todo.Title)
	if deadline, ok := todo.Deadline.Get(); ok {
		m.fields[1].SetValue(deadline.In(m.now.Location()).Format("2006-01-02 15:04"))
	}
	m.fields[2].SetValue(todo.Description)
	m.fields[3].SetValue(todo.ListName)
	return m
}

//nolint:gocognit,funlen,nestif // Key precedence is intentionally visible in one form transition.
func (m Form) updateKey(msg tea.KeyMsg) (Form, tea.Cmd) {
	name := msg.String()
	if msg.Paste {
		var cmd tea.Cmd
		m.fields[m.focus], cmd = m.fields[m.focus].Update(msg)
		m.message = m.dateFeedback()
		return m, cmd
	}
	if name == "esc" || slices.Contains(m.keys.Cancel, name) {
		return m, func() tea.Msg { return formCancelled{generation: m.generation} }
	}
	if m.focus == listField && (name == keyDown || name == keyUp) {
		matches := matchingSuggestions(m.fields[3].Value(), m.suggestions)
		if len(matches) > 0 {
			if name == keyDown {
				m.suggestion = (m.suggestion + 1) % len(matches)
			} else {
				if m.suggestion < 0 {
					m.suggestion = len(matches) - 1
				} else {
					m.suggestion = (m.suggestion + len(matches) - 1) % len(matches)
				}
			}
			return m, nil
		}
	}
	switch name {
	case "tab":
		m.fields[m.focus].Blur()
		m.focus = (m.focus + 1) % len(m.fields)
		m.fields[m.focus].Focus()
	case "shift+tab":
		m.fields[m.focus].Blur()
		m.focus = (m.focus + len(m.fields) - 1) % len(m.fields)
		m.fields[m.focus].Focus()
	case "enter":
		if m.focus == listField {
			matches := matchingSuggestions(m.fields[3].Value(), m.suggestions)
			if m.suggestion >= 0 && m.suggestion < len(matches) {
				chosen := matches[m.suggestion]
				m.fields[3].SetValue(chosen)
				m.suggestion = -1
				m.message = "List: " + chosen
				return m, nil
			}
		}
		var cmd tea.Cmd
		m, cmd = m.save()
		return m, cmd
	case keyDown, keyUp:
		if m.focus == listField {
			matches := matchingSuggestions(m.fields[3].Value(), m.suggestions)
			if len(matches) > 0 {
				if name == keyDown {
					m.suggestion = (m.suggestion + 1) % len(matches)
				} else {
					m.suggestion = (m.suggestion + len(matches) - 1) % len(matches)
				}
				m.message = "List: " + matches[m.suggestion]
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.fields[m.focus], cmd = m.fields[m.focus].Update(msg)
		return m, cmd
	default:
		var cmd tea.Cmd
		m.fields[m.focus], cmd = m.fields[m.focus].Update(msg)
		m.message = ""
		m.message = m.dateFeedback()
		if m.focus == listField {
			m.suggestion = -1
		}
		return m, cmd
	}
	return m, nil
}
func (m Form) dateFeedback() string {
	v := strings.TrimSpace(m.fields[1].Value())
	if v == "" {
		return "Deadline: none"
	}
	d, err := dateparser.Parse(v, m.now, m.defaultTime)
	if err != nil {
		return "Deadline: " + safeError(err)
	}
	return "Deadline: " + d.In(m.now.Location()).Format("Mon 2006-01-02 15:04")
}
func (m Form) save() (Form, tea.Cmd) {
	if m.busy || m.loading {
		return m, nil
	}
	title, deadlineText, description, list := m.fields[0].Value(), strings.TrimSpace(
		m.fields[1].Value(),
	), m.fields[2].Value(), m.fields[3].Value()
	if strings.TrimSpace(list) == "" {
		list = m.defaultList
	}
	var deadline option.Option[time.Time]
	if deadlineText != "" {
		d, err := dateparser.Parse(deadlineText, m.now, m.defaultTime)
		if err != nil {
			m.message = "Deadline: " + safeError(err)
			return m, nil
		}
		deadline = option.Some(d)
	}
	if strings.TrimSpace(title) == "" {
		m.message = "Title must not be empty"
		return m, nil
	}
	if strings.TrimSpace(list) == "" {
		m.message = "List name must not be empty"
		return m, nil
	}
	m.busy = true
	m.writeErr = nil
	m.message = "Saving…"
	g, reader, id := m.generation, m.reader, m.id
	if id == "" {
		p := service.CreateTodoParams{
			Title:       title,
			Description: description,
			Deadline:    deadline,
			ListName:    list,
		}
		return m, func() tea.Msg { _, err := reader.Create(p); return formSaved{generation: g, err: err} }
	}
	p := service.UpdateTodoParams{
		Title:       option.Some(title),
		Description: option.Some(description),
		Deadline:    option.Some(deadline),
		ListName:    option.Some(list),
	}
	return m, func() tea.Msg { _, err := reader.Update(id, p); return formSaved{generation: g, err: err} }
}
func safeError(err error) string { return singleLine(fmt.Sprint(err)) }
