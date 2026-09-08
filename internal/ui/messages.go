package ui

import (
	"github.com/webjaba/callpit-client/internal/api"
	"github.com/webjaba/callpit-client/internal/app"
	"github.com/webjaba/callpit-client/internal/audio"
)

type loginResultMsg struct {
	user     api.User
	rejoined bool
	err      error
}

type joinResultMsg struct {
	room string
	err  error
}

type devicesResultMsg struct {
	inputs  []audio.Device
	outputs []audio.Device
	err     error
}

type selectDeviceResultMsg struct {
	err error
}

type setAccessTokenResultMsg struct {
	err error
}

type sessionEventMsg struct {
	event app.Event
}
