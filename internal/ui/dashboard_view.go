package ui

import (
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
	if m.height > chromeHeight {
		lines = append(lines, m.renderLine("psst", m.styles.header))
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
	if m.height > chromeHeight {
		lines = append(lines, m.renderLine(m.help, m.styles.muted))
	}
	return strings.Join(lines, "\n")
}

func (m Dashboard) contentHeight() int {
	if m.height > chromeHeight {
		return m.height - chromeHeight
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
	}{{keys.Up, "up"}, {keys.Down, "down"}, {keys.Quit, "quit"}} {
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
