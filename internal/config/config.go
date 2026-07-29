// Package config stores the handful of preferences that shouldn't have to be
// retyped every run. A missing or broken config is never fatal — the defaults
// stand in silently, because failing to start over a preferences file would be
// absurd.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Config is the on-disk preferences file.
type Config struct {
	// DefaultSource is the parts source to open with, by provider ID.
	DefaultSource string `json:"default_source,omitempty"`

	// Keys holds credentials for sources that need them, by provider ID.
	// Environment variables win over this file — see Key.
	Keys map[string]string `json:"keys,omitempty"`
}

// Default is what bomexpo uses when there's no config to read.
func Default() Config { return Config{DefaultSource: "lcsc"} }

// Dir is where the config lives, or "" if the OS won't tell us.
func Dir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "bomexpo")
}

// Path is the config file, or "" if there's nowhere to put it.
func Path() string {
	d := Dir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "config.json")
}

// Load reads the config, falling back to Default for anything missing or
// unreadable.
func Load() Config {
	p := Path()
	if p == "" {
		return Default()
	}
	return loadFrom(p)
}

func loadFrom(path string) Config {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Default()
	}
	var c Config
	if json.Unmarshal(raw, &c) != nil {
		return Default() // a corrupt file shouldn't stop the app
	}
	if c.DefaultSource == "" {
		c.DefaultSource = Default().DefaultSource
	}
	return c
}

// Save writes the config, creating its directory. The file is owner-only
// because it may hold credentials.
func Save(c Config) error {
	p := Path()
	if p == "" {
		return nil
	}
	return saveTo(p, c)
}

func saveTo(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

// Key returns the credential for a source: the BOMEXPO_<SOURCE>_KEY environment
// variable if set, otherwise whatever the config file holds. Env comes first so
// a key never has to be written to disk to be used.
func (c Config) Key(source string) string {
	if source == "" {
		return ""
	}
	env := "BOMEXPO_" + strings.ToUpper(source) + "_KEY"
	if v := strings.TrimSpace(os.Getenv(env)); v != "" {
		return v
	}
	return strings.TrimSpace(c.Keys[source])
}
