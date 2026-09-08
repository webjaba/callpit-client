package rtc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/pion/webrtc/v4"

	"github.com/webjaba/callpit-client/internal/api"
	"github.com/webjaba/callpit-client/internal/audio"
	"github.com/webjaba/callpit-client/internal/signaling"
)

type PeerState string

const (
	PeerConnecting PeerState = "connecting"
	PeerConnected  PeerState = "connected"
	PeerFailed     PeerState = "failed"
)

type StateEvent struct {
	PeerID string
	State  PeerState
}

type peerConnection struct {
	connection        *webrtc.PeerConnection
	remoteDescription bool
	pendingCandidates []webrtc.ICECandidateInit
}

type Call struct {
	ctx       context.Context
	signal    *signaling.Client
	audio     *audio.Engine
	track     *webrtc.TrackLocalStaticSample
	config    webrtc.Configuration
	onState   func(StateEvent)
	mu        sync.Mutex
	peers     map[string]*peerConnection
	closeOnce sync.Once
}

func New(ctx context.Context, signal *signaling.Client, rtcConfig api.RTCConfig, onState func(StateEvent)) (*Call, error) {
	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
		"microphone",
		"audio",
	)
	if err != nil {
		return nil, fmt.Errorf("create local audio track: %w", err)
	}
	engine, err := audio.New(track)
	if err != nil {
		return nil, err
	}

	iceServers := make([]webrtc.ICEServer, 0, len(rtcConfig.ICEServers))
	for _, server := range rtcConfig.ICEServers {
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs:       server.URLs,
			Username:   server.Username,
			Credential: server.Credential,
		})
	}

	return &Call{
		ctx:     ctx,
		signal:  signal,
		audio:   engine,
		track:   track,
		config:  webrtc.Configuration{ICEServers: iceServers},
		onState: onState,
		peers:   make(map[string]*peerConnection),
	}, nil
}

func (c *Call) StartAudio(inputID, outputID *string) error {
	return c.audio.Start(inputID, outputID)
}

func (c *Call) Devices() ([]audio.Device, []audio.Device, error) {
	return c.audio.Devices()
}

func (c *Call) SetInput(id *string) error {
	return c.audio.SetInput(id)
}

func (c *Call) SetOutput(id *string) error {
	return c.audio.SetOutput(id)
}

func (c *Call) SetMuted(value bool) {
	c.audio.SetMuted(value)
}

func (c *Call) SetDeafened(value bool) {
	c.audio.SetDeafened(value)
}

func (c *Call) AddPeer(peerID string, initiator bool) error {
	pc, err := webrtc.NewPeerConnection(c.config)
	if err != nil {
		return fmt.Errorf("create peer connection: %w", err)
	}
	sender, err := pc.AddTrack(c.track)
	if err != nil {
		pc.Close()
		return fmt.Errorf("add local audio track: %w", err)
	}

	entry := &peerConnection{connection: pc}
	c.mu.Lock()
	if _, exists := c.peers[peerID]; exists {
		c.mu.Unlock()
		pc.Close()
		return nil
	}
	c.peers[peerID] = entry
	c.mu.Unlock()
	keep := false
	defer func() {
		if !keep {
			c.RemovePeer(peerID)
		}
	}()

	go func() {
		buffer := make([]byte, 1500)
		for {
			if _, _, err := sender.Read(buffer); err != nil {
				return
			}
		}
	}()

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		_ = c.signal.Send(c.ctx, peerID, struct {
			Type      string                  `json:"type"`
			Candidate webrtc.ICECandidateInit `json:"candidate"`
		}{Type: "ice-candidate", Candidate: candidate.ToJSON()})
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateConnected:
			c.onState(StateEvent{PeerID: peerID, State: PeerConnected})
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateDisconnected:
			c.onState(StateEvent{PeerID: peerID, State: PeerFailed})
		}
	})
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if track.Kind() == webrtc.RTPCodecTypeAudio {
			c.audio.AddRemote(peerID, track)
		}
	})

	c.onState(StateEvent{PeerID: peerID, State: PeerConnecting})
	if !initiator {
		keep = true
		return nil
	}

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("create offer: %w", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("set local offer: %w", err)
	}
	err = c.signal.Send(c.ctx, peerID, struct {
		Type string `json:"type"`
		SDP  string `json:"sdp"`
	}{Type: "offer", SDP: offer.SDP})
	if err != nil {
		return err
	}
	keep = true
	return nil
}

func (c *Call) HandleSignal(peerID string, data json.RawMessage) error {
	var signal signaling.Signal
	if err := json.Unmarshal(data, &signal); err != nil {
		return fmt.Errorf("decode WebRTC signal: %w", err)
	}

	c.mu.Lock()
	peer := c.peers[peerID]
	c.mu.Unlock()
	if peer == nil {
		return nil
	}

	switch signal.Type {
	case "offer":
		if err := c.setRemoteDescription(peer, webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: signal.SDP}); err != nil {
			return err
		}
		answer, err := peer.connection.CreateAnswer(nil)
		if err != nil {
			return fmt.Errorf("create answer: %w", err)
		}
		if err := peer.connection.SetLocalDescription(answer); err != nil {
			return fmt.Errorf("set local answer: %w", err)
		}
		return c.signal.Send(c.ctx, peerID, struct {
			Type string `json:"type"`
			SDP  string `json:"sdp"`
		}{Type: "answer", SDP: answer.SDP})
	case "answer":
		return c.setRemoteDescription(peer, webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: signal.SDP})
	case "ice-candidate":
		var candidate webrtc.ICECandidateInit
		if err := json.Unmarshal(signal.Candidate, &candidate); err != nil {
			return fmt.Errorf("decode ICE candidate: %w", err)
		}
		c.mu.Lock()
		if !peer.remoteDescription {
			peer.pendingCandidates = append(peer.pendingCandidates, candidate)
			c.mu.Unlock()
			return nil
		}
		c.mu.Unlock()
		return peer.connection.AddICECandidate(candidate)
	default:
		return fmt.Errorf("unknown WebRTC signal type %q", signal.Type)
	}
}

func (c *Call) RemovePeer(peerID string) {
	c.mu.Lock()
	peer := c.peers[peerID]
	delete(c.peers, peerID)
	c.mu.Unlock()
	if peer != nil {
		c.audio.RemoveRemote(peerID)
		_ = peer.connection.Close()
	}
}

func (c *Call) Close() error {
	var result error
	c.closeOnce.Do(func() {
		c.mu.Lock()
		peers := c.peers
		c.peers = make(map[string]*peerConnection)
		c.mu.Unlock()
		for _, peer := range peers {
			if err := peer.connection.Close(); err != nil && result == nil {
				result = err
			}
		}
		if err := c.audio.Close(); err != nil && result == nil {
			result = err
		}
	})
	return result
}

func (c *Call) setRemoteDescription(peer *peerConnection, description webrtc.SessionDescription) error {
	if description.SDP == "" {
		return fmt.Errorf("empty session description")
	}
	if err := peer.connection.SetRemoteDescription(description); err != nil {
		return fmt.Errorf("set remote description: %w", err)
	}

	c.mu.Lock()
	peer.remoteDescription = true
	candidates := peer.pendingCandidates
	peer.pendingCandidates = nil
	c.mu.Unlock()
	for _, candidate := range candidates {
		if err := peer.connection.AddICECandidate(candidate); err != nil && err != io.EOF {
			return fmt.Errorf("add pending ICE candidate: %w", err)
		}
	}
	return nil
}
