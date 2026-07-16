package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qiz029/vibe-pushover/internal/config"
)

func TestSaveAndLoadCredentials(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "config.json")
	want := config.Credentials{AppToken: "app-token", UserKey: "user-key"}

	if err := config.Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0o600 {
		t.Fatalf("config mode = %o, want 600", gotMode)
	}
}
