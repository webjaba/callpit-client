package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

func TestSpatialFocus(t *testing.T) {
	tests := []struct {
		name  string
		input struct {
			focus int
			key   string
		}
		want int
	}{
		{name: "ok", input: struct {
			focus int
			key   string
		}{focus: focusRoom, key: "right"}, want: focusLogin},
		{name: "room down", input: struct {
			focus int
			key   string
		}{focus: focusRoom, key: "down"}, want: focusAudio},
		{name: "login left", input: struct {
			focus int
			key   string
		}{focus: focusLogin, key: "left"}, want: focusRoom},
		{name: "login down", input: struct {
			focus int
			key   string
		}{focus: focusLogin, key: "down"}, want: focusLeave},
		{name: "settings right", input: struct {
			focus int
			key   string
		}{focus: focusSettings, key: "right"}, want: focusMic},
		{name: "settings up", input: struct {
			focus int
			key   string
		}{focus: focusSettings, key: "up"}, want: focusRoom},
		{name: "mic left", input: struct {
			focus int
			key   string
		}{focus: focusMic, key: "left"}, want: focusSettings},
		{name: "mic right", input: struct {
			focus int
			key   string
		}{focus: focusMic, key: "right"}, want: focusAudio},
		{name: "mic up", input: struct {
			focus int
			key   string
		}{focus: focusMic, key: "up"}, want: focusRoom},
		{name: "audio left", input: struct {
			focus int
			key   string
		}{focus: focusAudio, key: "left"}, want: focusMic},
		{name: "audio right", input: struct {
			focus int
			key   string
		}{focus: focusAudio, key: "right"}, want: focusLeave},
		{name: "audio up", input: struct {
			focus int
			key   string
		}{focus: focusAudio, key: "up"}, want: focusRoom},
		{name: "leave left", input: struct {
			focus int
			key   string
		}{focus: focusLeave, key: "left"}, want: focusAudio},
		{name: "leave up", input: struct {
			focus int
			key   string
		}{focus: focusLeave, key: "up"}, want: focusLogin},
		{name: "edge", input: struct {
			focus int
			key   string
		}{focus: focusRoom, key: "left"}, want: focusRoom},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = mustSetup(t)

			got := spatialFocus(tt.input.focus, tt.input.key)

			require.Equal(t, tt.want, got)
		})
	}
}

func TestModelUpdate(t *testing.T) {
	textKey := tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"})
	escapeKey := tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
	enterKey := tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	rightKey := tea.KeyPressMsg(tea.Key{Code: tea.KeyRight})
	slashKey := tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"})
	qKey := tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"})
	tests := []struct {
		name  string
		input tea.KeyPressMsg
		setup func(Setup) Model
		want  struct {
			focus   int
			mode    interactionMode
			value   string
			focused bool
		}
	}{
		{
			name:  "ok",
			input: textKey,
			want: struct {
				focus   int
				mode    interactionMode
				value   string
				focused bool
			}{focus: focusRoom, mode: modeTextInput, value: "l", focused: true},
		},
		{
			name:  "escape leaves text input",
			input: escapeKey,
			want: struct {
				focus   int
				mode    interactionMode
				value   string
				focused bool
			}{focus: focusRoom, mode: modeNavigation},
		},
		{
			name:  "enter opens text input",
			input: enterKey,
			setup: func(setup Setup) Model {
				model := setup.Model
				model.setMode(modeNavigation)
				return model
			},
			want: struct {
				focus   int
				mode    interactionMode
				value   string
				focused bool
			}{focus: focusRoom, mode: modeTextInput, focused: true},
		},
		{
			name:  "arrow remains in text input",
			input: rightKey,
			want: struct {
				focus   int
				mode    interactionMode
				value   string
				focused bool
			}{focus: focusRoom, mode: modeTextInput, focused: true},
		},
		{
			name:  "slash opens text input",
			input: slashKey,
			setup: func(setup Setup) Model {
				model := setup.Model
				model.setFocus(focusLogin)
				model.setMode(modeNavigation)
				return model
			},
			want: struct {
				focus   int
				mode    interactionMode
				value   string
				focused bool
			}{focus: focusRoom, mode: modeTextInput, focused: true},
		},
		{
			name:  "enter edits access key",
			input: enterKey,
			setup: func(setup Setup) Model {
				model := setup.Model
				model.settings = true
				model.setMode(modeNavigation)
				return model
			},
			want: struct {
				focus   int
				mode    interactionMode
				value   string
				focused bool
			}{focus: focusRoom, mode: modeTextInput, focused: true},
		},
		{
			name:  "shortcut is access key text",
			input: qKey,
			setup: func(setup Setup) Model {
				model := setup.Model
				model.settings = true
				model.setMode(modeTextInput)
				return model
			},
			want: struct {
				focus   int
				mode    interactionMode
				value   string
				focused bool
			}{focus: focusRoom, mode: modeTextInput, value: "q", focused: true},
		},
		{
			name:  "escape cancels access key",
			input: escapeKey,
			setup: func(setup Setup) Model {
				model := setup.Model
				model.settings = true
				model.tokenInput.SetValue("1:discarded-secret")
				model.setMode(modeTextInput)
				return model
			},
			want: struct {
				focus   int
				mode    interactionMode
				value   string
				focused bool
			}{focus: focusRoom, mode: modeNavigation},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setup := mustSetup(t)
			model := setup.Model
			if tt.setup != nil {
				model = tt.setup(setup)
			}

			updated, _ := model.Update(tt.input)
			got := updated.(Model)

			require.Equal(t, tt.want.focus, got.focus)
			require.Equal(t, tt.want.mode, got.mode)
			focused := got.roomInput.Focused()
			value := got.roomInput.Value()
			if got.settings && got.deviceCursor == 0 {
				focused = got.tokenInput.Focused()
				value = got.tokenInput.Value()
			}
			require.Equal(t, tt.want.value, value)
			require.Equal(t, tt.want.focused, focused)
		})
	}
}

func TestModelUpdateSetAccessTokenResult(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "ok"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setup := mustSetup(t)
			setup.settings = true
			setup.authenticated = true
			setup.activeRoom = "friends"
			setup.tokenInput.SetValue("1:new-secret")
			setup.setMode(modeTextInput)

			updated, _ := setup.Update(setAccessTokenResultMsg{})
			got := updated.(Model)

			require.False(t, got.authenticated)
			require.Empty(t, got.activeRoom)
			require.Empty(t, got.tokenInput.Value())
			require.Equal(t, modeNavigation, got.mode)
			require.Equal(t, "access key saved; press Login", got.status)
		})
	}
}

func TestModelRenderSettings(t *testing.T) {
	const token = "1:plain-secret"
	tests := []struct {
		name string
	}{
		{name: "ok"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setup := mustSetup(t)
			setup.settings = true
			setup.tokenInput.SetValue(token)
			setup.setMode(modeTextInput)

			got := setup.renderSettings(100, 30)

			require.NotContains(t, got, token)
			require.Contains(t, got, "**************")
		})
	}
}
