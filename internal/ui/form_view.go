package ui

import (
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m Form) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	lines := []string{"New todo"}
	if m.id != "" {
		lines[0] = "Edit todo"
	}
	lines[0] = m.line(lines[0], m.styles.header)
	lines = append(lines, m.fieldLines()...)
	if len(lines) < m.height && m.message != "" {
		style := m.styles.muted
		if m.writeErr != nil {
			style = m.styles.error
		}
		lines = append(lines, m.line(m.message, style))
	}
	if len(lines) < m.height && m.focus == listField {
		matches := matchingSuggestions(m.fields[listField].Value(), m.suggestions)
		for _, name := range matches {
			if len(lines) >= m.height {
				break
			}
			style, prefix := m.styles.text, "  "
			if name == m.selectedList {
				style, prefix = m.styles.selected, "> "
			}
			lines = append(lines, m.line(prefix+name, style))
		}
	}
	if len(lines) < m.height {
		lines = append(lines, m.footerLine())
	}
	return strings.Join(lines, "\n")
}

func (m Form) fieldLines() []string {
	remaining := max(0, m.height-1)
	if remaining == 0 {
		return nil
	}
	if m.id != "" && m.loading {
		return []string{m.line("Loading todo…", m.styles.muted)}
	}
	count := min(len(m.fields), remaining)
	lines := make([]string, 0, count)
	for i := range count {
		value := m.fields[i].Value()
		text := m.fields[i].Prompt + singleLine(value)
		style := m.styles.text
		if value == "" && m.fields[i].Placeholder != "" {
			text = m.fields[i].Prompt + singleLine(m.fields[i].Placeholder)
			style = m.styles.muted
			if i == listField {
				style = m.styles.text
			}
		}
		focused := i == m.focus
		if focused {
			text = "> " + text
		}
		if focused && value != "" {
			style = m.styles.selected
		}
		lines = append(lines, m.line(text, style))
	}
	return lines
}

func (m Form) footerLine() string {
	help := "tab/shift+tab fields · enter save"
	if m.id != "" && m.loading {
		help = ""
	}
	cancel := slices.Clone(m.keys.Cancel)
	if !slices.Contains(cancel, "esc") {
		cancel = append(cancel, "esc")
	}
	help = appendHelp(help, cancel, "cancel")
	help = appendHelp(help, effectiveQuitKeys(m.keys.Quit), "quit")
	return m.line(help, m.styles.muted)
}

func appendHelp(help string, bindings []string, label string) string {
	if len(bindings) == 0 {
		return help
	}
	if help != "" {
		help += " · "
	}
	return help + strings.Join(bindings, "/") + " " + label
}

func (m Form) line(text string, style lipgloss.Style) string {
	return style.Width(m.width).Render(ansi.Truncate(singleLine(text), m.width, ""))
}
func matchingSuggestions(query string, values []string) []string {
	queryRunes := []rune(strings.ToLower(strings.TrimSpace(query)))
	var out []string
	for _, value := range values {
		at := 0
		for _, r := range strings.ToLower(value) {
			if at < len(queryRunes) && r == queryRunes[at] {
				at++
			}
		}
		if at == len(queryRunes) {
			out = append(out, value)
		}
	}
	return out
}
