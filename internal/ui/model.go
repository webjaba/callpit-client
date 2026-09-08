package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/webjaba/callpit-client/internal/api"
	"github.com/webjaba/callpit-client/internal/app"
	"github.com/webjaba/callpit-client/internal/audio"
	"github.com/webjaba/callpit-client/internal/rtc"
	"github.com/webjaba/callpit-client/internal/signaling"
)

const (
	focusRoom = iota
	focusLogin
	focusSettings
	focusMic
	focusAudio
	focusLeave
	focusCount
)

type interactionMode int

const (
	modeNavigation interactionMode = iota
	modeTextInput
)

type participant struct {
	peer  signaling.Peer
	state rtc.PeerState
	self  bool
}

type deviceOption struct {
	input bool
	id    *string
	name  string
}

type Model struct {
	session       *app.Session
	configPath    string
	roomInput     textinput.Model
	tokenInput    textinput.Model
	width         int
	height        int
	focus         int
	mode          interactionMode
	busy          bool
	authenticated bool
	user          api.User
	activeRoom    string
	participants  map[string]participant
	muted         bool
	deafened      bool
	status        string
	err           error
	settings      bool
	devices       []deviceOption
	deviceCursor  int
}

func NewModel(session *app.Session, configPath string) Model {
	input := textinput.New()
	input.Placeholder = "Join room..."
	input.SetWidth(30)
	_ = input.Focus()
	tokenInput := textinput.New()
	tokenInput.Placeholder = "Enter new access key"
	tokenInput.EchoMode = textinput.EchoPassword
	tokenInput.SetWidth(36)

	return Model{
		session:      session,
		configPath:   configPath,
		roomInput:    input,
		tokenInput:   tokenInput,
		mode:         modeTextInput,
		participants: make(map[string]participant),
		status:       "press l to login",
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.roomInput.Focus(), waitSessionEvent(m.session.Events()))
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	textInputActive := m.mode == modeTextInput
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.roomInput.SetWidth(max(12, min(38, msg.Width/3-4)))
		m.tokenInput.SetWidth(max(16, min(44, msg.Width-24)))
	case tea.KeyPressMsg:
		if cmd := m.handleKey(msg); cmd != nil {
			return m, cmd
		}
	case loginResultMsg:
		m.busy = false
		m.err = msg.err
		if msg.err == nil {
			m.authenticated = true
			m.user = msg.user
			if msg.rejoined && len(m.participants) == 0 {
				m.status = "waiting for room snapshot"
			} else if !msg.rejoined {
				m.status = "authenticated"
			}
		} else {
			m.authenticated = false
			m.participants = make(map[string]participant)
			m.status = "login failed"
		}
	case joinResultMsg:
		m.busy = false
		m.err = msg.err
		if msg.err == nil {
			m.activeRoom = msg.room
			if len(m.participants) == 0 {
				m.status = "waiting for room snapshot"
			}
		} else {
			m.status = "join failed"
		}
	case devicesResultMsg:
		m.busy = false
		m.err = msg.err
		m.settings = true
		m.setMode(modeNavigation)
		m.deviceCursor = 0
		m.devices = buildDeviceOptions(msg.inputs, msg.outputs)
	case selectDeviceResultMsg:
		m.busy = false
		m.err = msg.err
		if msg.err == nil {
			m.status = "audio device saved"
		}
	case setAccessTokenResultMsg:
		m.busy = false
		m.err = msg.err
		if msg.err == nil {
			m.tokenInput.SetValue("")
			m.setMode(modeNavigation)
			m.authenticated = false
			m.user = api.User{}
			m.activeRoom = ""
			m.participants = make(map[string]participant)
			m.muted = false
			m.deafened = false
			m.status = "access key saved; press Login"
		}
	case sessionEventMsg:
		m.applyEvent(msg.event)
		return m, waitSessionEvent(m.session.Events())
	}

	if textInputActive && m.mode == modeTextInput && m.settings && m.deviceCursor == 0 {
		var cmd tea.Cmd
		m.tokenInput, cmd = m.tokenInput.Update(message)
		return m, cmd
	}
	if textInputActive && m.mode == modeTextInput && m.focus == focusRoom && !m.settings {
		var cmd tea.Cmd
		m.roomInput, cmd = m.roomInput.Update(message)
		return m, cmd
	}
	return m, nil
}

func (m Model) View() tea.View {
	width := max(60, m.width)
	height := max(18, m.height)
	header := m.renderHeader(width)
	bodyHeight := max(8, height-lipgloss.Height(header)-3)
	body := m.renderParticipants(width, bodyHeight)
	if m.settings {
		body = m.renderSettings(width, bodyHeight)
	}
	footer := m.renderFooter(width)

	content := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	view := tea.NewView(baseStyle.Width(width).Height(height).Render(content))
	view.AltScreen = true
	return view
}

func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	if key == "ctrl+c" || key == "q" && m.mode == modeNavigation {
		m.session.Close()
		return tea.Quit
	}
	if m.mode == modeTextInput {
		switch key {
		case "enter":
			if m.settings && m.deviceCursor == 0 {
				return m.saveAccessToken()
			}
			return m.startJoin()
		case "esc":
			if m.settings && m.deviceCursor == 0 {
				m.tokenInput.SetValue("")
			}
			m.setMode(modeNavigation)
		}
		return nil
	}
	if m.settings {
		switch key {
		case "esc", "s":
			m.settings = false
			m.setFocus(focusSettings)
		case "up", "k":
			m.deviceCursor = max(0, m.deviceCursor-1)
		case "down", "j":
			m.deviceCursor = min(len(m.devices), m.deviceCursor+1)
		case "enter":
			if m.deviceCursor == 0 {
				m.tokenInput.SetValue("")
				m.setMode(modeTextInput)
			} else if !m.busy {
				option := m.devices[m.deviceCursor-1]
				m.busy = true
				return selectDeviceCmd(m.session, option)
			}
		}
		return nil
	}
	switch key {
	case "left", "right", "up", "down":
		m.setFocus(spatialFocus(m.focus, key))
		return nil
	case "tab":
		m.setFocus((m.focus + 1) % focusCount)
		return nil
	case "shift+tab":
		m.setFocus((m.focus - 1 + focusCount) % focusCount)
		return nil
	case "/":
		m.setFocus(focusRoom)
		m.setMode(modeTextInput)
		return nil
	case "l":
		return m.startLogin()
	case "m":
		m.toggleMic()
		return nil
	case "d":
		m.toggleAudio()
		return nil
	case "s":
		return m.openSettings()
	case "x":
		m.leave()
		return nil
	case "enter":
		switch m.focus {
		case focusRoom:
			m.setMode(modeTextInput)
		case focusLogin:
			return m.startLogin()
		case focusSettings:
			return m.openSettings()
		case focusMic:
			m.toggleMic()
		case focusAudio:
			m.toggleAudio()
		case focusLeave:
			m.leave()
		}
	}
	return nil
}

func (m *Model) setFocus(focus int) {
	m.focus = focus
	m.syncTextInput()
}

func (m *Model) setMode(mode interactionMode) {
	m.mode = mode
	m.syncTextInput()
}

func (m *Model) syncTextInput() {
	m.roomInput.Blur()
	m.tokenInput.Blur()
	if m.mode != modeTextInput {
		return
	}
	if m.settings && m.deviceCursor == 0 {
		_ = m.tokenInput.Focus()
		return
	}
	if m.mode == modeTextInput && m.focus == focusRoom && !m.settings {
		_ = m.roomInput.Focus()
	}
}

func spatialFocus(focus int, key string) int {
	switch focus {
	case focusRoom:
		if key == "right" {
			return focusLogin
		}
		if key == "down" {
			return focusAudio
		}
	case focusLogin:
		if key == "left" {
			return focusRoom
		}
		if key == "down" {
			return focusLeave
		}
	case focusSettings:
		if key == "right" {
			return focusMic
		}
		if key == "up" {
			return focusRoom
		}
	case focusMic:
		if key == "left" {
			return focusSettings
		}
		if key == "right" {
			return focusAudio
		}
		if key == "up" {
			return focusRoom
		}
	case focusAudio:
		if key == "left" {
			return focusMic
		}
		if key == "right" {
			return focusLeave
		}
		if key == "up" {
			return focusRoom
		}
	case focusLeave:
		if key == "left" {
			return focusAudio
		}
		if key == "up" {
			return focusLogin
		}
	}
	return focus
}

func (m *Model) startLogin() tea.Cmd {
	if m.busy {
		return nil
	}
	m.busy = true
	m.err = nil
	m.status = "authenticating"
	m.participants = make(map[string]participant)
	return loginCmd(m.session, m.activeRoom)
}

func (m *Model) startJoin() tea.Cmd {
	if m.busy || !m.authenticated {
		if !m.authenticated {
			m.err = fmt.Errorf("login first")
		}
		return nil
	}
	room := strings.TrimSpace(m.roomInput.Value())
	if room == "" {
		return nil
	}
	m.busy = true
	m.err = nil
	m.participants = make(map[string]participant)
	m.status = "joining " + room
	return joinCmd(m.session, room)
}

func (m *Model) openSettings() tea.Cmd {
	if m.busy {
		return nil
	}
	m.busy = true
	m.err = nil
	return devicesCmd(m.session)
}

func (m *Model) saveAccessToken() tea.Cmd {
	if m.busy {
		return nil
	}
	token := strings.TrimSpace(m.tokenInput.Value())
	if token == "" {
		m.err = fmt.Errorf("access key must not be empty")
		return nil
	}
	m.busy = true
	m.err = nil
	return setAccessTokenCmd(m.session, token)
}

func (m *Model) toggleMic() {
	if m.activeRoom == "" {
		return
	}
	m.muted = !m.muted
	m.session.SetMuted(m.muted)
}

func (m *Model) toggleAudio() {
	if m.activeRoom == "" {
		return
	}
	m.deafened = !m.deafened
	m.session.SetDeafened(m.deafened)
}

func (m *Model) leave() {
	m.session.Leave()
	m.activeRoom = ""
	m.participants = make(map[string]participant)
	m.muted = false
	m.deafened = false
	m.status = "left call"
}

func (m *Model) applyEvent(event app.Event) {
	if !m.session.IsCurrent(event) {
		return
	}
	switch event.Type {
	case app.EventSnapshot:
		m.participants = make(map[string]participant, len(event.Peers)+1)
		m.participants[event.Self.PeerID] = participant{peer: *event.Self, state: rtc.PeerConnected, self: true}
		for _, peer := range event.Peers {
			m.participants[peer.PeerID] = participant{peer: peer, state: rtc.PeerConnecting}
		}
		m.status = "connected"
	case app.EventPeerJoined:
		m.participants[event.Peer.PeerID] = participant{peer: *event.Peer, state: rtc.PeerConnecting}
	case app.EventPeerLeft:
		delete(m.participants, event.PeerID)
	case app.EventPeerState:
		participant, ok := m.participants[event.PeerID]
		if ok {
			participant.state = event.State
			m.participants[event.PeerID] = participant
		}
	case app.EventDisconnected:
		m.session.Leave()
		m.participants = make(map[string]participant)
		m.err = event.Err
		m.status = "disconnected; press l to reconnect"
	case app.EventError:
		m.err = event.Err
	}
}

func waitSessionEvent(events <-chan app.Event) tea.Cmd {
	return func() tea.Msg {
		return sessionEventMsg{event: <-events}
	}
}

func loginCmd(session *app.Session, room string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		user, err := session.Login(ctx)
		if err != nil {
			return loginResultMsg{err: err}
		}
		if room != "" {
			if err := session.Join(ctx, room); err != nil {
				return loginResultMsg{err: err}
			}
		}
		return loginResultMsg{user: user, rejoined: room != ""}
	}
}

func joinCmd(session *app.Session, room string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return joinResultMsg{room: room, err: session.Join(ctx, room)}
	}
}

func devicesCmd(session *app.Session) tea.Cmd {
	return func() tea.Msg {
		inputs, outputs, err := session.Devices()
		return devicesResultMsg{inputs: inputs, outputs: outputs, err: err}
	}
}

func selectDeviceCmd(session *app.Session, option deviceOption) tea.Cmd {
	return func() tea.Msg {
		if option.input {
			return selectDeviceResultMsg{err: session.SelectInput(option.id)}
		}
		return selectDeviceResultMsg{err: session.SelectOutput(option.id)}
	}
}

func setAccessTokenCmd(session *app.Session, token string) tea.Cmd {
	return func() tea.Msg {
		return setAccessTokenResultMsg{err: session.SetAccessToken(token)}
	}
}

func buildDeviceOptions(inputs, outputs []audio.Device) []deviceOption {
	options := []deviceOption{{input: true, name: "System default microphone"}}
	for _, device := range inputs {
		id := device.ID
		options = append(options, deviceOption{input: true, id: &id, name: device.Name})
	}
	options = append(options, deviceOption{input: false, name: "System default output"})
	for _, device := range outputs {
		id := device.ID
		options = append(options, deviceOption{input: false, id: &id, name: device.Name})
	}
	return options
}

func sortedParticipants(participants map[string]participant) []participant {
	result := make([]participant, 0, len(participants))
	for _, item := range participants {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].self != result[j].self {
			return result[i].self
		}
		if result[i].peer.Username == result[j].peer.Username {
			return result[i].peer.PeerID < result[j].peer.PeerID
		}
		return result[i].peer.Username < result[j].peer.Username
	})
	return result
}
