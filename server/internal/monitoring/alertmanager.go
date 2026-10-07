package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AlertmanagerClient is a thin HTTP client for the Alertmanager API v2.
type AlertmanagerClient struct {
	baseURL string
	http    *http.Client
}

func NewAlertmanagerClient(baseURL string, timeoutSec int) *AlertmanagerClient {
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	return &AlertmanagerClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
	}
}

// Available reports whether an Alertmanager URL is configured.
func (c *AlertmanagerClient) Available() bool { return c != nil && c.baseURL != "" }

func (c *AlertmanagerClient) doJSON(ctx context.Context, method, path string, body []byte) ([]byte, int, error) {
	if !c.Available() {
		return nil, 0, fmt.Errorf("alertmanager not configured")
	}
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("alertmanager unreachable: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return b, resp.StatusCode, err
}

// Alerts returns all current alerts from Alertmanager.
func (c *AlertmanagerClient) Alerts(ctx context.Context) (json.RawMessage, error) {
	b, status, err := c.doJSON(ctx, http.MethodGet, "/api/v2/alerts", nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("alertmanager alerts: http %d", status)
	}
	return json.RawMessage(b), nil
}

// Silences returns all silences.
func (c *AlertmanagerClient) Silences(ctx context.Context) (json.RawMessage, error) {
	b, status, err := c.doJSON(ctx, http.MethodGet, "/api/v2/silences", nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("alertmanager silences: http %d", status)
	}
	return json.RawMessage(b), nil
}

// CreateSilence posts a new silence. body must be a valid Alertmanager silence JSON.
func (c *AlertmanagerClient) CreateSilence(ctx context.Context, body []byte) (json.RawMessage, error) {
	b, status, err := c.doJSON(ctx, http.MethodPost, "/api/v2/silences", body)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return nil, fmt.Errorf("alertmanager create silence: http %d: %s", status, string(b))
	}
	return json.RawMessage(b), nil
}

// DeleteSilence deletes a silence by ID.
func (c *AlertmanagerClient) DeleteSilence(ctx context.Context, id string) error {
	_, status, err := c.doJSON(ctx, http.MethodDelete, "/api/v2/silence/"+id, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return fmt.Errorf("alertmanager delete silence: http %d", status)
	}
	return nil
}

// Status checks Alertmanager health (/-/healthy).
func (c *AlertmanagerClient) Status(ctx context.Context) (bool, string) {
	if !c.Available() {
		return false, "not configured"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/-/healthy", nil)
	if err != nil {
		return false, err.Error()
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, err.Error()
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return true, "ok"
	}
	return false, fmt.Sprintf("http %d", resp.StatusCode)
}
