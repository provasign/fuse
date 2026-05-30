// Package grove is a small HTTP client for the Grove graph engine. Fuse uses
// Grove for cross-file blast radius and breaking change detection.
package grove

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"time"
)

// Client speaks JSON to a running Grove instance over HTTP.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New returns a Client targeting baseURL with a sensible default timeout.
func New(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Health pings Grove's /health endpoint.
func (c *Client) Health(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/health", nil)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("grove health: %s", resp.Status)
	}
	return nil
}

// Edge mirrors grove core.Edge for the wire.
type Edge struct {
	From       string  `json:"from"`
	To         string  `json:"to"`
	Type       string  `json:"type"`
	Confidence float64 `json:"confidence"`
}

// ImpactNode is a single entry in the impact response.
type ImpactNode struct {
	ID       string `json:"id"`
	FilePath string `json:"filePath"`
	Name     string `json:"name"`
}

// SymbolRecord mirrors grove core.SymbolRecord for the wire (lossy).
type SymbolRecord struct {
	ID       string `json:"id"`
	FilePath string `json:"filePath"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// Deps returns the edges declared from filePath (imports, calls, etc.).
func (c *Client) Deps(ctx context.Context, filePath string) ([]Edge, error) {
	var resp struct {
		Edges []Edge `json:"edges"`
	}
	if err := c.post(ctx, "/deps", map[string]any{"file": filePath}, &resp); err != nil {
		return nil, err
	}
	return resp.Edges, nil
}

// Impact returns nodes affected by changes to query (file path or symbol).
func (c *Client) Impact(ctx context.Context, query string, maxDepth int) ([]ImpactNode, error) {
	if maxDepth <= 0 {
		maxDepth = 3
	}
	var resp struct {
		Nodes []ImpactNode `json:"nodes"`
	}
	if err := c.post(ctx, "/impact", map[string]any{"query": query, "maxDepth": maxDepth}, &resp); err != nil {
		return nil, err
	}
	return resp.Nodes, nil
}

// Symbols searches symbols by query string.
func (c *Client) Symbols(ctx context.Context, query string, limit int) ([]SymbolRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	var resp struct {
		Symbols []SymbolRecord `json:"symbols"`
	}
	if err := c.post(ctx, "/symbols", map[string]any{"query": query, "limit": limit}, &resp); err != nil {
		return nil, err
	}
	return resp.Symbols, nil
}

// Index triggers (re-)indexing of dir.
func (c *Client) Index(ctx context.Context, dir string) error {
	return c.post(ctx, "/index", map[string]any{"dir": dir}, &struct{}{})
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("grove %s: %s: %s", path, resp.Status, string(b))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// EnsureRunning checks /health and, if unreachable, attempts to start the
// grove binary in the background. Returns nil if Grove is healthy within
// timeout, error otherwise.
func EnsureRunning(ctx context.Context, baseURL, binary string, timeout time.Duration) error {
	c := New(baseURL)
	if err := c.Health(ctx); err == nil {
		return nil
	}
	// Attempt to spawn the binary.
	cmd := exec.Command(binary, "serve", "--port", "7777")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("grove not reachable and failed to start %s: %w", binary, err)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := c.Health(ctx); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("grove not reachable at %s after %s", baseURL, timeout)
}
