package hooks_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/qiz029/vibe-pushover/internal/hooks"
)

func TestInstallAddsCodexHooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".codex", "hooks.json")
	changed, err := hooks.Install("codex", path, "/usr/local/bin/vibe-pushover")
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !changed {
		t.Fatal("Install() changed = false, want true")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var got struct {
		Hooks map[string][]json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(got.Hooks["Stop"]) != 1 {
		t.Fatalf("Stop hook count = %d, want 1", len(got.Hooks["Stop"]))
	}
	if len(got.Hooks["PermissionRequest"]) != 1 {
		t.Fatalf("PermissionRequest hook count = %d, want 1", len(got.Hooks["PermissionRequest"]))
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "settings.json")
	if _, err := hooks.Install("claude", path, "/opt/bin/vibe-pushover"); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	changed, err := hooks.Install("claude", path, "/opt/bin/vibe-pushover")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed = true, want false")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("second Install() rewrote the config")
	}
}

func TestInstallPreservesExistingSettingsAndHooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{
  "theme": "dark",
  "hooks": {
    "Stop": [{"hooks": [{"type": "command", "command": "existing-hook"}]}]
  }
}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := hooks.Install("claude", path, "/opt/bin/vibe-pushover"); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var got struct {
		Theme string `json:"theme"`
		Hooks struct {
			Stop []json.RawMessage `json:"Stop"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.Theme != "dark" {
		t.Fatalf("theme = %q, want dark", got.Theme)
	}
	if len(got.Hooks.Stop) != 2 {
		t.Fatalf("Stop hook count = %d, want 2", len(got.Hooks.Stop))
	}
}
