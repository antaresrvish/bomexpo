package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nested", "config.json")
	want := Config{DefaultSource: "jlcpcb", Keys: map[string]string{"mouser": "abc123"}}
	if err := saveTo(p, want); err != nil {
		t.Fatal(err)
	}
	got := loadFrom(p)
	if got.DefaultSource != "jlcpcb" || got.Keys["mouser"] != "abc123" {
		t.Errorf("round trip lost data: %+v", got)
	}
}

func TestSavedFileIsOwnerOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := saveTo(p, Default()); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	// it can hold credentials, so nobody else gets to read it
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("config mode = %o, want 600", perm)
	}
}

func TestLoadFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()

	missing := loadFrom(filepath.Join(dir, "absent.json"))
	if missing.DefaultSource != "lcsc" {
		t.Errorf("missing file gave %+v, want the lcsc default", missing)
	}

	broken := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(broken, []byte("{not json at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := loadFrom(broken); got.DefaultSource != "lcsc" {
		t.Errorf("corrupt file gave %+v, want the lcsc default", got)
	}

	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := loadFrom(empty); got.DefaultSource != "lcsc" {
		t.Errorf("config without a source gave %q, want lcsc", got.DefaultSource)
	}
}

func TestKeyPrefersEnv(t *testing.T) {
	c := Config{Keys: map[string]string{"mouser": "from-file"}}

	if got := c.Key("mouser"); got != "from-file" {
		t.Errorf("Key without env = %q, want from-file", got)
	}

	t.Setenv("BOMEXPO_MOUSER_KEY", "from-env")
	if got := c.Key("mouser"); got != "from-env" {
		t.Errorf("Key with env set = %q, want from-env", got)
	}

	// a blank env var must not mask the file
	t.Setenv("BOMEXPO_MOUSER_KEY", "   ")
	if got := c.Key("mouser"); got != "from-file" {
		t.Errorf("Key with a blank env = %q, want from-file", got)
	}

	if got := c.Key("nobody"); got != "" {
		t.Errorf("Key for an unconfigured source = %q, want empty", got)
	}
	if got := c.Key(""); got != "" {
		t.Errorf("Key(\"\") = %q, want empty", got)
	}
}
