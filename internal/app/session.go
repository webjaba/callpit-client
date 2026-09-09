package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/webjaba/callpit-client/internal/api"
	"github.com/webjaba/callpit-client/internal/audio"
	"github.com/webjaba/callpit-client/internal/config"
	"github.com/webjaba/callpit-client/internal/rtc"
	"github.com/webjaba/callpit-client/internal/signaling"
)

const serverURL = "http://localhost:8080"

type EventType string

const (
	EventSnapshot      EventType = "snapshot"
	EventPeerJoined    EventType = "peer_joined"
	EventPeerLeft      EventType = "peer_left"
	EventPeerState     EventType = "peer_state"
	EventVoiceActivity EventType = "voice_activity"
	EventDisconnected  EventType = "disconnected"
	EventError         EventType = "error"
)

type Event struct {
	Type       EventType
	Generation uint64
	Self       *signaling.Peer
	Peers      []signaling.Peer
	Peer       *signaling.Peer
	PeerID     string
	State      rtc.PeerState
	Active     bool
	Err        error
}

type Session struct {
	configPath string
	events     chan Event
	lifecycle  sync.Mutex
	mu         sync.Mutex
	generation uint64
	cfg        config.Config
	user       api.User
	rtcConfig  api.RTCConfig
	cancel     context.CancelFunc
	signal     *signaling.Client
	call       *rtc.Call
}

func NewSession(configPath string) *Session {
	return &Session{configPath: configPath, events: make(chan Event, 64)}
}

func (s *Session) Events() <-chan Event {
	return s.events
}

func (s *Session) Login(ctx context.Context) (api.User, error) {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	s.stopCallLocked()

	cfg, err := config.Load(s.configPath)
	if err != nil {
		if errors.Is(err, config.ErrNotConfigured) {
			return api.User{}, fmt.Errorf("configure %s", s.configPath)
		}
		return api.User{}, err
	}
	client, err := api.New(serverURL)
	if err != nil {
		return api.User{}, err
	}
	user, err := client.User(ctx, cfg.AccessToken)
	if err != nil {
		return api.User{}, fmt.Errorf("login: %w", err)
	}
	rtcConfig, err := client.RTCConfig(ctx, cfg.AccessToken)
	if err != nil {
		return api.User{}, fmt.Errorf("load RTC config: %w", err)
	}

	s.mu.Lock()
	s.cfg = cfg
	s.user = user
	s.rtcConfig = rtcConfig
	s.mu.Unlock()
	return user, nil
}

func (s *Session) Join(ctx context.Context, room string) error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	s.stopCallLocked()

	s.mu.Lock()
	cfg := s.cfg
	rtcConfig := s.rtcConfig
	s.generation++
	generation := s.generation
	callCtx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Unlock()

	signal, err := signaling.Connect(ctx, serverURL, cfg.AccessToken, room)
	if err != nil {
		cancel()
		return err
	}
	call, err := rtc.New(callCtx, signal, rtcConfig, func(event rtc.StateEvent) {
		s.emit(generation, Event{Type: EventPeerState, PeerID: event.PeerID, State: event.State})
	}, func(event rtc.ActivityEvent) {
		s.emit(generation, Event{Type: EventVoiceActivity, PeerID: event.PeerID, Active: event.Active})
	})
	if err != nil {
		signal.Close()
		cancel()
		return err
	}
	if err := call.StartAudio(cfg.InputDeviceID, cfg.OutputDeviceID); err != nil {
		call.Close()
		signal.Close()
		cancel()
		return err
	}

	s.mu.Lock()
	if generation != s.generation {
		s.mu.Unlock()
		call.Close()
		signal.Close()
		cancel()
		return context.Canceled
	}
	s.signal = signal
	s.call = call
	s.mu.Unlock()

	go s.read(callCtx, generation, signal, call)
	return nil
}

func (s *Session) Devices() ([]audio.Device, []audio.Device, error) {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	s.mu.Lock()
	call := s.call
	s.mu.Unlock()
	if call != nil {
		return call.Devices()
	}
	return audio.ListDevices()
}

func (s *Session) SelectInput(id *string) error {
	return s.selectDevice(true, id)
}

func (s *Session) SelectOutput(id *string) error {
	return s.selectDevice(false, id)
}

func (s *Session) SetAccessToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("access key must not be empty")
	}

	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	cfg, err := config.Read(s.configPath)
	if err != nil {
		return err
	}
	cfg.AccessToken = token
	if err := config.Save(s.configPath, cfg); err != nil {
		return err
	}

	s.stopCallLocked()
	s.mu.Lock()
	s.cfg = cfg
	s.user = api.User{}
	s.rtcConfig = api.RTCConfig{}
	s.mu.Unlock()
	return nil
}

func (s *Session) SetMuted(value bool) {
	s.mu.Lock()
	call := s.call
	s.mu.Unlock()
	if call != nil {
		call.SetMuted(value)
	}
}

func (s *Session) SetDeafened(value bool) {
	s.mu.Lock()
	call := s.call
	s.mu.Unlock()
	if call != nil {
		call.SetDeafened(value)
	}
}

func (s *Session) Leave() {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	s.stopCallLocked()
}

func (s *Session) Close() {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	s.stopCallLocked()
}

func (s *Session) IsCurrent(event Event) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return event.Generation == s.generation
}

func (s *Session) read(ctx context.Context, generation uint64, signal *signaling.Client, call *rtc.Call) {
	for {
		event, err := signal.Read(ctx)
		if err != nil {
			if ctx.Err() == nil {
				s.emit(generation, Event{Type: EventDisconnected, Err: err})
			}
			return
		}

		switch event.Type {
		case "room.snapshot":
			if event.Self == nil {
				s.emit(generation, Event{Type: EventError, Err: fmt.Errorf("invalid room snapshot")})
				continue
			}
			s.emit(generation, Event{Type: EventSnapshot, Self: event.Self, Peers: event.Peers})
			for _, peer := range event.Peers {
				if err := call.AddPeer(peer.PeerID, true); err != nil {
					s.emit(generation, Event{Type: EventError, Err: err})
				}
			}
		case "peer.joined":
			if event.Peer == nil {
				continue
			}
			if err := call.AddPeer(event.Peer.PeerID, false); err != nil {
				s.emit(generation, Event{Type: EventError, Err: err})
				continue
			}
			s.emit(generation, Event{Type: EventPeerJoined, Peer: event.Peer})
		case "peer.left":
			call.RemovePeer(event.PeerID)
			s.emit(generation, Event{Type: EventPeerLeft, PeerID: event.PeerID})
		case "signal":
			if err := call.HandleSignal(event.SourcePeerID, event.Data); err != nil {
				s.emit(generation, Event{Type: EventError, Err: err})
			}
		}
	}
}

func (s *Session) selectDevice(input bool, id *string) error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	s.mu.Lock()
	cfg := s.cfg
	call := s.call
	s.mu.Unlock()
	if cfg.AccessToken == "" {
		loaded, err := config.Load(s.configPath)
		if err != nil {
			return err
		}
		cfg = loaded
	}

	if call != nil {
		var err error
		if input {
			err = call.SetInput(id)
		} else {
			err = call.SetOutput(id)
		}
		if err != nil {
			return err
		}
	}
	if input {
		cfg.InputDeviceID = id
	} else {
		cfg.OutputDeviceID = id
	}
	if err := config.Save(s.configPath, cfg); err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	return nil
}

func (s *Session) stopCallLocked() {
	s.mu.Lock()
	s.generation++
	cancel := s.cancel
	signal := s.signal
	call := s.call
	s.cancel = nil
	s.signal = nil
	s.call = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if signal != nil {
		_ = signal.Close()
	}
	if call != nil {
		_ = call.Close()
	}
}

func (s *Session) emit(generation uint64, event Event) {
	s.mu.Lock()
	current := generation == s.generation
	s.mu.Unlock()
	if !current {
		return
	}
	event.Generation = generation
	s.events <- event
}
