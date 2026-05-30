// Package languages re-exports astkit's strategy/registry types so existing
// Fuse callers keep compiling. New code should import astkit directly.
package languages

import (
	"github.com/tabladrum/grove-suite/astkit"
	"github.com/tabladrum/grove-suite/astkit/strategies"
)

// Strategy is an alias to astkit.Strategy.
type Strategy = astkit.Strategy

// Registry is an alias to astkit.Registry.
type Registry = astkit.Registry

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return astkit.NewRegistry() }

// DefaultRegistry returns the registry pre-populated with every language Fuse
// supports for symbol-level merging.
func DefaultRegistry() *Registry { return strategies.Default() }
