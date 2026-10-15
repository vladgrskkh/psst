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
	if m.height > chromeHeight || m.input.Focused() || (m.query != "" && m.height > 1) {
		header := "psst"
		if m.input.Focused() {
			header = m.input.View()
		} else if m.query != "" {
			header = "Filter: " + singleLine(m.query) + " · psst"
		}
		lines = append(lines, m.renderLine(header, m.styles.header))
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

func (m Dashboard) footerHelp() string {
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
	for _, binding := range []struct {
		keys   []string
		action string
	}{{keys.Up, "up"}, {keys.Down, "down"}, {keys.Filter, "filter"}, {keys.Quit, "quit"}} {
		if len(binding.keys) > 0 {
			actions = append(actions, singleLine(strings.Join(binding.keys, "/"))+" "+binding.action)
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
