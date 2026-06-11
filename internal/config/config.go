// Package config loads Fuse configuration from fuse.yaml + env overrides.
package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version     int          `yaml:"version"`
	GroveURL    string       `yaml:"grove_url"`
	GroveBinary string       `yaml:"grove_binary"`
	Merge       MergeConfig  `yaml:"merge"`
	Git         GitConfig    `yaml:"git"`
	Server      ServerConfig `yaml:"server"`
}

type MergeConfig struct {
	ConfidenceMode       string  `yaml:"confidence_mode"`
	HandoffThreshold     float64 `yaml:"handoff_threshold"`
	EnableBreakingChange bool    `yaml:"enable_breaking_change"`
	EnableContext        bool    `yaml:"enable_context"`
	GroveRequired        bool    `yaml:"grove_required"`
	AutoIndex            bool    `yaml:"auto_index"`
}

type GitConfig struct {
	AutoInstall     bool   `yaml:"auto_install"`
	AttributesScope string `yaml:"attributes_scope"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

// Default returns the baseline configuration used when no file is found.
func Default() *Config {
	return &Config{
		Version:     1,
		GroveURL:    "http://localhost:7777",
		GroveBinary: "grove",
		Merge: MergeConfig{
			ConfidenceMode:       "static",
			HandoffThreshold:     0.30,
			EnableBreakingChange: true,
			EnableContext:        true,
			GroveRequired:        true,
			AutoIndex:            true,
		},
		Git: GitConfig{
			AutoInstall:     false,
			AttributesScope: "repo",
		},
		Server: ServerConfig{Port: 9999},
	}
}

// Load reads fuse.yaml from path (or returns defaults if not present) and
// applies environment overrides (GROVE_URL, FUSE_HANDOFF_THRESHOLD,
// FUSE_GROVE_REQUIRED).
func Load(path string) (*Config, error) {
	cfg := Default()
	if path != "" {
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, err
			}
		case !errors.Is(err, os.ErrNotExist):
			return nil, err
		}
	}
	if v := os.Getenv("GROVE_URL"); v != "" {
		cfg.GroveURL = v
	}
	if v := os.Getenv("FUSE_GROVE_REQUIRED"); v != "" {
		cfg.Merge.GroveRequired = v == "1" || v == "true"
	}
	return cfg, nil
}

// LocateConfig returns the first existing fuse.yaml walking up from cwd.
// Returns "" if nothing found.
func LocateConfig(cwd string) string {
	cur := cwd
	for {
		candidate := filepath.Join(cur, "fuse.yaml")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
}
