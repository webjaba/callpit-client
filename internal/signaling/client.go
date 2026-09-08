package signaling

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/coder/websocket"
)

type Client struct {
	conn      *websocket.Conn
	writeMu   sync.Mutex
	closeOnce sync.Once
}

func Connect(ctx context.Context, serverURL, token, room string) (*Client, error) {
	if err := validateRoom(room); err != nil {
		return nil, err
	}

	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("parse server URL: %w", err)
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/rooms/" + room + "/ws"
	u.RawPath = strings.TrimRight(u.EscapedPath(), "/")

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	conn, resp, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("connect room: %s", resp.Status)
		}
		return nil, fmt.Errorf("connect room: %w", err)
	}

	return &Client{conn: conn}, nil
}

func (c *Client) Read(ctx context.Context) (Event, error) {
	_, data, err := c.conn.Read(ctx)
	if err != nil {
		return Event{}, err
	}

	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return Event{}, fmt.Errorf("decode signaling event: %w", err)
	}
	if event.Type == "" {
		return Event{}, fmt.Errorf("signaling event has no type")
	}

	return event, nil
}

func (c *Client) Send(ctx context.Context, targetPeerID string, signal any) error {
	data, err := json.Marshal(signal)
	if err != nil {
		return fmt.Errorf("encode signal: %w", err)
	}
	message, err := json.Marshal(outboundSignal{
		Type:         "signal",
		TargetPeerID: targetPeerID,
		Data:         data,
	})
	if err != nil {
		return fmt.Errorf("encode signaling message: %w", err)
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.conn.Write(ctx, websocket.MessageText, message); err != nil {
		return fmt.Errorf("write signaling message: %w", err)
	}

	return nil
}

func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		err = c.conn.Close(websocket.StatusNormalClosure, "")
	})
	return err
}

func validateRoom(room string) error {
	if room != strings.TrimSpace(room) || room == "" || utf8.RuneCountInString(room) > 64 || strings.Contains(room, "/") {
		return fmt.Errorf("invalid room name")
	}
	for _, r := range room {
		if unicode.IsControl(r) {
			return fmt.Errorf("invalid room name")
		}
	}
	return nil
}
