package ui

import (
	"slices"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/vladgrskkh/psst/internal/config"
)

// View renders only the visible rows within the terminal's cell dimensions.
func (m Dashboard) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	lines := make([]string, 0, min(m.height, len(m.rows)+chromeHeight))
	status := m.phaseStatus()
	if m.height > chromeHeight || m.input.Focused() || status != "" || (m.query != "" && m.height > 1) {
		lines = append(lines, m.headerView(status))
	}
	end := min(len(m.rows), m.offset+m.contentHeight())
	for i := m.offset; i < end; i++ {
		row := m.rows[i]
		text, style := row.text, row.style
		if row.selectable {
			prefix := "  "
			if i == m.cursor {
				prefix, style = "> ", m.styles.selected
			}
			text = prefix + text
		}
		lines = append(lines, m.renderLine(text, style))
	}
	if m.height > chromeHeight || (m.input.Focused() && m.height > 1) {
		lines = append(lines, m.renderLine(m.footerHelp(), m.styles.muted))
	}
	return strings.Join(lines, "\n")
}

func (m Dashboard) headerView(status string) string {
	if status != "" {
		style := m.styles.header
		if m.actionError != "" {
			style = m.styles.error
		}
		return m.renderLine(status, style)
	}
	if m.input.Focused() {
		return m.renderLine(m.input.View(), m.styles.header)
	}
	if m.query != "" {
		return m.renderLine("Filter: "+singleLine(m.query)+" · psst", m.styles.header)
	}
	return m.renderLine("psst", m.styles.header)
}

func (m Dashboard) footerHelp() string {
	if m.phase != phaseIdle {
		return m.phaseHelp()
	}
	if !m.input.Focused() {
		return m.help
	}
	quit := m.filterQuitKeys()
	var cancel, actions []string
	for _, name := range m.keys.Cancel {
		if !slices.Contains(quit, name) {
			cancel = append(cancel, name)
		}
	}
	if !slices.Contains(quit, "enter") && !slices.Contains(cancel, "enter") {
		actions = append(actions, "enter submit")
	}
	if len(cancel) > 0 {
		actions = append(actions, singleLine(strings.Join(cancel, "/"))+" cancel")
	}
	if len(quit) > 0 {
		actions = append(actions, singleLine(strings.Join(quit, "/"))+" quit")
	}
	return strings.Join(actions, " · ")
}

func (m Dashboard) contentHeight() int {
	if m.input.Focused() {
		return max(0, m.height-chromeHeight)
	}
	if m.height > chromeHeight {
		return m.height - chromeHeight
	}
	if m.phaseStatus() != "" {
		return max(0, m.height-1)
	}
	if m.query != "" && m.height > 1 {
		return m.height - 1
	}
	return m.height
}

func (m Dashboard) renderLine(text string, style lipgloss.Style) string {
	return style.Width(m.width).Render(ansi.Truncate(text, m.width, ""))
}

func dashboardHelp(keys config.KeysConfig) string {
	var actions []string
	var claimed []string
	for _, binding := range []struct {
		keys   []string
		action string
	}{
		{keys.Quit, "quit"}, {keys.Filter, "filter"}, {keys.Up, "up"}, {keys.Down, "down"},
		{keys.ToggleDone, "toggle"}, {keys.Delete, "delete"},
	} {
		effective := effectiveKeys(binding.keys, claimed)
		if len(effective) > 0 {
			actions = append(actions, singleLine(strings.Join(effective, "/"))+" "+binding.action)
			claimed = append(claimed, effective...)
		}
	}
	return strings.Join(actions, " · ")
}

// singleLine removes terminal escapes and flattens text without changing the source todo.
func singleLine(text string) string {
	text = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r), unicode.IsControl(r):
			return ' '
		case unicode.Is(unicode.Cf, r) && r != '\u200d':
			return -1
		default:
			return r
		}
	}, ansi.Strip(text))
	return strings.Join(strings.Fields(text), " ")
}
