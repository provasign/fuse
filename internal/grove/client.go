// Package grove is Fuse's adapter to the in-process Grove engine. The previous
// HTTP client + auto-spawn of `grove serve` is gone: Fuse now links against
// `grove/pkg/grove` and opens the on-disk index directly. Public types
// (Client, Edge, ImpactNode, SymbolRecord, EnsureRunning) are preserved so the
// rest of Fuse compiles unchanged.
package grove

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	groveeng "github.com/tabladrum/grove-suite/grove/pkg/grove"
)

// Edge mirrors the engine edge shape Fuse consumes.
type Edge struct {
	From       string  `json:"from"`
	To         string  `json:"to"`
	Type       string  `json:"type"`
	Confidence float64 `json:"confidence"`
}

// ImpactNode is one entry in the blast-radius response.
type ImpactNode struct {
	ID       string `json:"id"`
	FilePath string `json:"filePath"`
	Name     string `json:"name"`
}

// SymbolRecord is the wire-format subset Fuse uses.
type SymbolRecord struct {
	ID       string `json:"id"`
	FilePath string `json:"filePath"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// Client wraps an embedded Grove engine. BaseURL/Token are accepted for API
// compatibility but unused.
type Client struct {
	BaseURL string
	Token   string

	root string

	mu  sync.Mutex
	eng *groveeng.Engine
}

// New returns a Client. baseURL is retained for compatibility and ignored.
func New(baseURL string) *Client {
	return &Client{BaseURL: baseURL}
}

// WithTokenFromDir records the project root so Fuse can open the engine at
// <dir>/.grove/grove.db on first use. No token is read (none exists).
func (c *Client) WithTokenFromDir(dir string) *Client {
	if abs, err := filepath.Abs(dir); err == nil {
		c.root = abs
	} else {
		c.root = dir
	}
	return c
}

// Health returns nil if the engine is open (or can be opened lazily).
func (c *Client) Health(ctx context.Context) error {
	_, err := c.ensure(ctx)
	return err
}

func (c *Client) ensure(ctx context.Context) (*groveeng.Engine, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.eng != nil {
		return c.eng, nil
	}
	if c.root == "" {
		return nil, errors.New("grove: WithTokenFromDir(dir) must be called before use")
	}
	eng, err := groveeng.Open(ctx, groveeng.Config{RepoRoot: c.root})
	if err != nil {
		return nil, fmt.Errorf("grove open: %w", err)
	}
	c.eng = eng
	return eng, nil
}

// Close releases the engine.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.eng != nil {
		_ = c.eng.Close()
		c.eng = nil
	}
}

// Deps returns dependency edges for filePath.
func (c *Client) Deps(ctx context.Context, filePath string) ([]Edge, error) {
	e, err := c.ensure(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := e.Deps(ctx, filePath)
	if err != nil {
		return nil, err
	}
	out := make([]Edge, 0, len(raw))
	for _, ed := range raw {
		out = append(out, Edge{From: ed.From, To: ed.To, Type: string(ed.Type), Confidence: ed.Confidence})
	}
	return out, nil
}

// Impact returns nodes affected by changes to query (file or symbol).
func (c *Client) Impact(ctx context.Context, query string, maxDepth int) ([]ImpactNode, error) {
	if maxDepth <= 0 {
		maxDepth = 3
	}
	e, err := c.ensure(ctx)
	if err != nil {
		return nil, err
	}
	syms, err := e.Impact(ctx, query, maxDepth)
	if err != nil {
		return nil, err
	}
	out := make([]ImpactNode, 0, len(syms))
	for _, s := range syms {
		out = append(out, ImpactNode{ID: s.ID, FilePath: s.FilePath, Name: s.Name})
	}
	return out, nil
}

// Symbols searches symbols by query string.
func (c *Client) Symbols(ctx context.Context, query string, limit int) ([]SymbolRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	e, err := c.ensure(ctx)
	if err != nil {
		return nil, err
	}
	syms, err := e.Symbols(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]SymbolRecord, 0, len(syms))
	for _, s := range syms {
		out = append(out, SymbolRecord{ID: s.ID, FilePath: s.FilePath, Name: s.Name, Kind: string(s.Kind)})
	}
	return out, nil
}

// Index triggers (re-)indexing of dir.
func (c *Client) Index(ctx context.Context, dir string) error {
	e, err := c.ensure(ctx)
	if err != nil {
		return err
	}
	_, err = e.Index(ctx, dir)
	return err
}

// EnsureRunning is a no-op in the embedded model — Grove is in-process. Kept
// for API compatibility; baseURL/binary/timeout are ignored. root is required
// so we know which repository's .grove/ to attach to.
func EnsureRunning(ctx context.Context, baseURL, binary, root string, timeout time.Duration) error {
	if root == "" {
		return errors.New("grove EnsureRunning: root is required (embedded mode)")
	}
	c := New(baseURL).WithTokenFromDir(root)
	return c.Health(ctx)
}
