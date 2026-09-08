package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/webjaba/callpit-client/internal/rtc"
)

func (m Model) renderHeader(width int) string {
	column := max(18, width/3)
	room := "# no room"
	if m.activeRoom != "" {
		room = "# " + m.activeRoom
	}
	left := roomStyle.Width(column).Padding(1, 2).Render(room)

	borderColor := colorBorder
	if m.focus == focusRoom {
		borderColor = colorBlue
		if m.mode == modeTextInput {
			borderColor = colorOrange
		}
	}
	center := baseStyle.Width(column).
		Align(lipgloss.Center).
		Padding(0, 1).
		Render(baseStyle.Border(lipgloss.RoundedBorder()).BorderForeground(borderColor).Padding(0, 1).Render(m.roomInput.View()))

	identity := "offline"
	if m.authenticated {
		identity = "● " + m.user.Username
	}
	right := baseStyle.Width(max(1, width-column*2)).Align(lipgloss.Right).Padding(1, 2).
		Render(identity + "  " + m.button("Login", focusLogin, false))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, center, right)
}

func (m Model) renderParticipants(width, height int) string {
	items := sortedParticipants(m.participants)
	if len(items) == 0 {
		message := "No one is here"
		if m.activeRoom == "" {
			message = "Enter a room and press Enter"
		}
		return baseStyle.Width(width).Height(height).Align(lipgloss.Center, lipgloss.Center).
			Render(mutedStyle.Render(message))
	}

	cardWidth := 25
	columns := max(1, width/(cardWidth+2))
	rows := make([]string, 0, (len(items)+columns-1)/columns)
	for start := 0; start < len(items); start += columns {
		end := min(len(items), start+columns)
		cards := make([]string, 0, end-start)
		for _, item := range items[start:end] {
			state := string(item.state)
			if item.self {
				state = "you · connected"
			}
			style := cardStyle
			if item.state == rtc.PeerConnected {
				style = cardConnectedStyle
			}
			cards = append(cards, style.Width(cardWidth).Render(item.peer.Username+"\n"+mutedStyle.Render(state)))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cards...))
	}
	grid := lipgloss.JoinVertical(lipgloss.Center, rows...)
	return baseStyle.Width(width).Height(height).Align(lipgloss.Center, lipgloss.Center).Render(grid)
}

func (m Model) renderSettings(width, height int) string {
	lines := []string{roomStyle.Render("Settings"), ""}
	visible := max(1, height-8)
	start := max(0, m.deviceCursor-visible/2)
	end := min(len(m.devices)+1, start+visible)
	start = max(0, end-visible)
	for i := start; i < end; i++ {
		prefix := "  "
		style := baseStyle
		if i == m.deviceCursor {
			prefix = "> "
			style = style.Foreground(colorOrange)
		}
		if i == 0 {
			value := "Access key"
			if m.mode == modeTextInput {
				value = m.tokenInput.View()
			}
			lines = append(lines, style.Render(fmt.Sprintf("%s%-3s  %s", prefix, "key", value)))
			continue
		}
		option := m.devices[i-1]
		kind := "mic"
		if !option.input {
			kind = "out"
		}
		lines = append(lines, style.Render(fmt.Sprintf("%s%-3s  %s", prefix, kind, option.name)))
	}
	hint := "↑/↓ select · enter apply · esc close"
	if m.mode == modeTextInput {
		hint = "enter save · esc cancel"
	}
	lines = append(lines, "", mutedStyle.Render(hint))
	panel := panelStyle.Width(min(64, width-8)).MaxHeight(height - 2).Render(strings.Join(lines, "\n"))
	return baseStyle.Width(width).Height(height).Align(lipgloss.Center, lipgloss.Center).Render(panel)
}

func (m Model) renderFooter(width int) string {
	column := max(18, width/3)
	left := baseStyle.Width(column).Padding(0, 2).Render(m.button("Settings", focusSettings, false))
	center := baseStyle.Width(column).Align(lipgloss.Center).Render(strings.Join([]string{
		m.button("Mic", focusMic, m.muted),
		m.button("Audio", focusAudio, m.deafened),
		m.button("Leave", focusLeave, false),
	}, " "))
	status := m.status
	style := mutedStyle
	if m.err != nil {
		status = m.err.Error()
		style = errorStyle
	}
	right := style.Width(max(1, width-column*2)).Align(lipgloss.Right).Padding(0, 2).Render(status)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, center, right)
}

func (m Model) button(label string, focus int, active bool) string {
	text := fmt.Sprintf("[ %s ]", label)
	style := baseStyle.Foreground(colorMuted)
	if active {
		style = style.Foreground(colorOrange).Bold(true)
	}
	if m.focus == focus {
		style = style.Foreground(colorBlue).Bold(true)
	}
	return style.Render(text)
}
