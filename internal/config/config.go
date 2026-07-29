// Package config stores the preferences that shouldn't be retyped every run. A
// missing or broken file is never fatal; the defaults stand in silently.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	DefaultSource string `json:"default_source,omitempty"`

	// by provider ID; env wins over this, see Key
	Keys map[string]string `json:"keys,omitempty"`
}

func Default() Config { return Config{DefaultSource: "lcsc"} }

// Dir is "" when the OS won't tell us.
func Dir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "bomexpo")
}

func Path() string {
	d := Dir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "config.json")
}

// Load falls back to Default for anything missing or unreadable.
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

// Save writes owner-only, since the file may hold credentials.
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

// Key prefers BOMEXPO_<SOURCE>_KEY, so a key never has to reach disk to be used.
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
