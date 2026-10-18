package ui

import (
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

//nolint:gocognit // Rendering branches are bounded by the viewport and form state.
func (m Form) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	lines := []string{"New todo"}
	if m.id != "" {
		lines[0] = "Edit todo"
	}
	lines[0] = m.line(lines[0], m.styles.header)
	for i := range m.fields {
		if len(lines) >= m.height {
			break
		}
		value := m.fields[i].Value()
		text := m.fields[i].Prompt + singleLine(value)
		style := m.styles.text
		if value == "" && m.fields[i].Placeholder != "" {
			text = m.fields[i].Prompt + singleLine(m.fields[i].Placeholder)
			style = m.styles.muted
		}
		if i == m.focus {
			text = "> " + text
			if value != "" {
				style = m.styles.selected
			}
		}
		lines = append(lines, m.line(text, style))
	}
	if len(lines) < m.height && m.message != "" {
		style := m.styles.muted
		if m.writeErr != nil {
			style = m.styles.error
		}
		lines = append(lines, m.line(m.message, style))
	}
	if len(lines) < m.height {
		query := m.fields[3].Value()
		if m.focus != listField {
			query = ""
		}
		matches := matchingSuggestions(query, m.suggestions)
		for i, name := range matches {
			if len(lines) >= m.height {
				break
			}
			style, prefix := m.styles.muted, "  "
			if i == m.suggestion {
				style, prefix = m.styles.selected, "> "
			}
			lines = append(lines, m.line(prefix+name, style))
		}
	}
	if len(lines) < m.height {
		cancel := slices.Clone(m.keys.Cancel)
		if !slices.Contains(cancel, "esc") {
			cancel = append(cancel, "esc")
		}
		help := "tab/shift+tab fields · enter save"
		if len(cancel) > 0 {
			help += " · " + strings.Join(cancel, "/") + " cancel"
		}
		if len(m.keys.Quit) > 0 {
			help += " · " + strings.Join(m.keys.Quit, "/") + " quit"
		}
		lines = append(lines, m.line(help, m.styles.muted))
	}
	return strings.Join(lines, "\n")
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
