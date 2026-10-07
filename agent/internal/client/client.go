// Package client is a minimal HTTP client for the ServerHub agent API on
// the Go/Gin backend. Auth is a hashed agent token sent as
// `Authorization: Bearer <token>` (see server/internal/middleware/agent.go).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"serverhub-agent/internal/metrics"
)

// Client posts heartbeats and metrics to ServerHub.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New builds a client with a 15s per-request timeout.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Heartbeat marks the server ONLINE/CONNECTED (POST /agent/heartbeat).
func (c *Client) Heartbeat(ctx context.Context) error {
	return c.post(ctx, "/server-hub/api/agent/heartbeat", map[string]any{})
}

// SendMetrics stores a snapshot and refreshes host info (POST /agent/metrics).
func (c *Client) SendMetrics(ctx context.Context, s metrics.Snapshot) error {
	return c.post(ctx, "/server-hub/api/agent/metrics", s)
}

func (c *Client) post(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("server returned HTTP %d: %s", resp.StatusCode, string(detail))
}
