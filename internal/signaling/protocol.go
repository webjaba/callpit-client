package signaling

import "encoding/json"

type Peer struct {
	PeerID   string `json:"peer_id"`
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
}

type Event struct {
	Type         string          `json:"type"`
	Self         *Peer           `json:"self,omitempty"`
	Peers        []Peer          `json:"peers,omitempty"`
	Peer         *Peer           `json:"peer,omitempty"`
	PeerID       string          `json:"peer_id,omitempty"`
	SourcePeerID string          `json:"source_peer_id,omitempty"`
	Data         json.RawMessage `json:"data,omitempty"`
}

type Signal struct {
	Type      string          `json:"type"`
	SDP       string          `json:"sdp,omitempty"`
	Candidate json.RawMessage `json:"candidate,omitempty"`
}

type outboundSignal struct {
	Type         string          `json:"type"`
	TargetPeerID string          `json:"target_peer_id"`
	Data         json.RawMessage `json:"data"`
}
