package ui

import "charm.land/lipgloss/v2"

var (
	colorBackground = lipgloss.Color("#0A0A0A")
	colorForeground = lipgloss.Color("#F2F2F2")
	colorMuted      = lipgloss.Color("#737373")
	colorBorder     = lipgloss.Color("#3F3F46")
	colorOrange     = lipgloss.Color("#FF8C42")
	colorBlue       = lipgloss.Color("#5E6AD2")

	baseStyle = lipgloss.NewStyle().
			Background(colorBackground).
			Foreground(colorForeground)
	mutedStyle = baseStyle.Foreground(colorMuted)
	roomStyle  = baseStyle.Bold(true).Foreground(colorBlue)
	errorStyle = baseStyle.Foreground(colorOrange)
	cardStyle  = baseStyle.
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(1, 2)
	cardConnectedStyle = cardStyle.BorderForeground(colorBlue)
	panelStyle         = baseStyle.
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorder).
				Padding(1, 2)
)
