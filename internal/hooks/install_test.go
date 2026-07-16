package hooks_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/qiz029/vibe-pushover/internal/hooks"
)

func TestInstallAddsCodexHooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".codex", "hooks.json")
	changed, err := hooks.Install("codex", path, "/usr/local/bin/vibe-pushover", "")
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
	if _, err := hooks.Install("claude", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	changed, err := hooks.Install("claude", path, "/opt/bin/vibe-pushover", "")
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

	if _, err := hooks.Install("claude", path, "/opt/bin/vibe-pushover", ""); err != nil {
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

func TestInstallPassesCustomPushoverConfigToHook(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "hooks.json")
	_, err := hooks.Install("codex", path, "/opt/bin/vibe-pushover", "/tmp/custom config.json")
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Contains(data, []byte(`--config '/tmp/custom config.json'`)) {
		t.Fatalf("installed hooks do not contain custom config path: %s", data)
	}
}

func TestInstallPreservesLargeJSONNumbers(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "settings.json")
	const original = `{"large_id":9007199254740993}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("claude", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Contains(data, []byte(`9007199254740993`)) {
		t.Fatalf("large JSON number was changed: %s", data)
	}
}

func TestInstallRejectsNullAgentConfig(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("null"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("claude", path, "/opt/bin/vibe-pushover", ""); err == nil {
		t.Fatal("Install() error = nil, want invalid top-level config error")
	}
}

func TestInstallUpdatesOwnedCommandWithoutDroppingSiblingHook(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "hooks.json")
	config := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"'/old/bin' notify --agent codex --event turn-complete --ignore-errors"},{"type":"command","command":"existing-hook"}]}]}}`
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("codex", path, "/new/bin", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Contains(data, []byte("existing-hook")) {
		t.Fatalf("sibling hook was removed: %s", data)
	}
	if !bytes.Contains(data, []byte("'/new/bin' notify")) {
		t.Fatalf("owned hook was not updated: %s", data)
	}
}

func TestInstallDoesNotReplaceCompositeCommandContainingMarker(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "hooks.json")
	composite := `'/bin/sh' -c "log-and-run vibe-pushover notify --agent codex --event turn-complete"`
	config := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":` + strconv.Quote(composite) + `}]}]}}`
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("codex", path, "/new/bin", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Contains(data, []byte("log-and-run")) {
		t.Fatalf("unrelated composite hook was replaced: %s", data)
	}
	var got struct {
		Hooks map[string][]json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(got.Hooks["Stop"]) != 2 {
		t.Fatalf("Stop hook count = %d, want 2", len(got.Hooks["Stop"]))
	}
}
