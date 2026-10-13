package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/vladgrskkh/psst/internal/config"
)

type dashboardStyles struct {
	header   lipgloss.Style
	text     lipgloss.Style
	muted    lipgloss.Style
	error    lipgloss.Style
	selected lipgloss.Style
	groups   [groupCount]lipgloss.Style
}

func newDashboardStyles(theme config.ThemeConfig) dashboardStyles {
	base := lipgloss.NewStyle().Background(lipgloss.Color(theme.Bg))
	return dashboardStyles{
		header:   base.Foreground(lipgloss.Color(theme.Accent)).Bold(true),
		text:     base.Foreground(lipgloss.Color(theme.Text)),
		muted:    base.Foreground(lipgloss.Color(theme.Muted)),
		error:    base.Foreground(lipgloss.Color(theme.Danger)),
		selected: base.Foreground(lipgloss.Color(theme.Cursor)).Background(lipgloss.Color(theme.Surface)).Bold(true),
		groups: [groupCount]lipgloss.Style{
			base.Foreground(lipgloss.Color(theme.Overdue)).Bold(true),
			base.Foreground(lipgloss.Color(theme.Today)).Bold(true),
			base.Foreground(lipgloss.Color(theme.Tomorrow)).Bold(true),
			base.Foreground(lipgloss.Color(theme.Later)).Bold(true),
		},
	}
}
