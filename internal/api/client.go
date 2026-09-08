package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

type RTCConfig struct {
	ICEServers []ICEServer `json:"ice_servers"`
}

type Client struct {
	baseURL *url.URL
	http    *http.Client
}

func New(serverURL string) (*Client, error) {
	baseURL, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("parse server URL: %w", err)
	}

	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (c *Client) User(ctx context.Context, token string) (User, error) {
	var user User
	if err := c.get(ctx, token, "/api/v1/user", &user); err != nil {
		return User{}, err
	}
	return user, nil
}

func (c *Client) RTCConfig(ctx context.Context, token string) (RTCConfig, error) {
	var cfg RTCConfig
	if err := c.get(ctx, token, "/api/v1/rtc-config", &cfg); err != nil {
		return RTCConfig{}, err
	}
	return cfg, nil
}

func (c *Client) get(ctx context.Context, token, path string, result any) error {
	u := *c.baseURL
	u.Path = strings.TrimRight(u.Path, "/") + path
	u.RawPath = ""

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}
