package hooks_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/qiz029/vibe-pushover/internal/hooks"
)

func TestDefaultPathPiHonorsAgentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "custom-pi-agent")
	t.Setenv("PI_CODING_AGENT_DIR", dir)
	t.Setenv("HOME", "")

	got, err := hooks.DefaultPath("pi")
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	want := filepath.Join(dir, "extensions", "vibe-pushover", "index.ts")
	if got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestDefaultPathsForAdditionalAgents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("COPILOT_HOME", "~/copilot-home")
	t.Setenv("GEMINI_CLI_HOME", "~/gemini-home")
	t.Setenv("XDG_CONFIG_HOME", "~/.xdg")

	tests := map[string]string{
		"copilot":  filepath.Join(home, "copilot-home", "hooks", "vibe-pushover.json"),
		"droid":    filepath.Join(home, ".factory", "settings.json"),
		"gemini":   filepath.Join(home, "gemini-home", ".gemini", "settings.json"),
		"goose":    filepath.Join(home, ".agents", "plugins", "vibe-pushover"),
		"opencode": filepath.Join(home, ".xdg", "opencode", "plugins", "vibe-pushover.ts"),
	}
	for agent, want := range tests {
		got, err := hooks.DefaultPath(agent)
		if err != nil {
			t.Fatalf("DefaultPath(%q) error = %v", agent, err)
		}
		if got != want {
			t.Errorf("DefaultPath(%q) = %q, want %q", agent, got, want)
		}
	}
}

func TestDefaultPathKimiUsesKimiCodeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := hooks.DefaultPath("kimi")
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	want := filepath.Join(home, ".kimi-code", "config.toml")
	if got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestDefaultPathKimiHonorsKimiCodeHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KIMI_CODE_HOME", "~/custom-kimi")

	got, err := hooks.DefaultPath("kimi")
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	want := filepath.Join(home, "custom-kimi", "config.toml")
	if got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestDefaultPathPiExpandsTildeAgentDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PI_CODING_AGENT_DIR", "~/.custom-pi")

	got, err := hooks.DefaultPath("pi")
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	want := filepath.Join(home, ".custom-pi", "extensions", "vibe-pushover", "index.ts")
	if got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

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

func TestInstallAddsGeminiTurnCompleteHook(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".gemini", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"theme":"dark","hooks":{"BeforeTool":[{"hooks":[{"type":"command","command":"existing"}]}]}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	changed, err := hooks.Install("gemini", path, "/opt/bin/vibe-pushover", "")
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
	if len(got.Hooks["AfterAgent"]) != 1 {
		t.Fatalf("AfterAgent hook count = %d, want 1", len(got.Hooks["AfterAgent"]))
	}
	if len(got.Hooks["BeforeTool"]) != 1 || !bytes.Contains(data, []byte(`"theme": "dark"`)) {
		t.Fatalf("Gemini install did not preserve sibling config: %s", data)
	}
	if bytes.Contains(data, []byte("PermissionRequest")) {
		t.Fatalf("Gemini config contains an unsupported permission hook: %s", data)
	}
	if bytes.Contains(data, []byte(`"async"`)) {
		t.Fatalf("Gemini config contains a Claude-specific async field: %s", data)
	}
	changed, err = hooks.Install("gemini", path, "/opt/bin/vibe-pushover", "")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed Gemini hooks")
	}
}

func TestInstallAddsDroidCompletionAndAttentionHooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".factory", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"model":"fast","hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"existing"}]}]}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("droid", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
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
	if len(got.Hooks["Stop"]) != 1 || len(got.Hooks["Notification"]) != 1 {
		t.Fatalf("hooks = %#v, want Stop and Notification", got.Hooks)
	}
	if len(got.Hooks["PreToolUse"]) != 1 || !bytes.Contains(data, []byte(`"model": "fast"`)) {
		t.Fatalf("Droid install did not preserve sibling config: %s", data)
	}
	if !bytes.Contains(data, []byte("--agent droid --event attention-required")) {
		t.Fatalf("Notification hook is not wired to attention-required: %s", data)
	}
	if bytes.Contains(data, []byte(`"async"`)) {
		t.Fatalf("Droid config contains a Claude-specific async field: %s", data)
	}
	changed, err := hooks.Install("droid", path, "/opt/bin/vibe-pushover", "")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed Droid hooks")
	}
}

func TestInstallAddsCopilotPersonalHooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".copilot", "hooks", "vibe-pushover.json")
	if _, err := hooks.Install("copilot", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json"); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var got struct {
		Version int                         `json:"version"`
		Hooks   map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.Version != 1 || len(got.Hooks["agentStop"]) != 1 || len(got.Hooks["notification"]) != 1 {
		t.Fatalf("Copilot manifest = %#v", got)
	}
	for _, event := range []string{"agentStop", "notification"} {
		if got.Hooks[event][0]["type"] != "command" || got.Hooks[event][0]["timeoutSec"] != float64(10) {
			t.Fatalf("%s hook = %#v", event, got.Hooks[event][0])
		}
	}
	if got.Hooks["notification"][0]["matcher"] != "permission_prompt|elicitation_dialog" {
		t.Fatalf("notification matcher = %#v", got.Hooks["notification"][0]["matcher"])
	}
	if !bytes.Contains(data, []byte("--event attention-required")) {
		t.Fatalf("Copilot attention hook is not wired correctly: %s", data)
	}
	if !bytes.Contains(data, []byte("--config '/tmp/pushover.json'")) {
		t.Fatalf("Copilot hooks do not use custom config: %s", data)
	}
	changed, err := hooks.Install("copilot", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed Copilot hooks")
	}
}

func TestInstallCreatesGooseOpenPlugin(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".agents", "plugins", "vibe-pushover")
	if _, err := hooks.Install("goose", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	manifest, err := os.ReadFile(filepath.Join(path, "plugin.json"))
	if err != nil {
		t.Fatalf("ReadFile(plugin.json) error = %v", err)
	}
	if !bytes.Contains(manifest, []byte(`"name": "vibe-pushover"`)) {
		t.Fatalf("unexpected Goose plugin manifest: %s", manifest)
	}
	hookData, err := os.ReadFile(filepath.Join(path, "hooks", "hooks.json"))
	if err != nil {
		t.Fatalf("ReadFile(hooks.json) error = %v", err)
	}
	if !bytes.Contains(hookData, []byte(`"Stop"`)) || !bytes.Contains(hookData, []byte("--agent goose --event turn-complete")) {
		t.Fatalf("unexpected Goose hooks: %s", hookData)
	}
	changed, err := hooks.Install("goose", path, "/opt/bin/vibe-pushover", "")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed Goose plugin")
	}
}

func TestInstallGooseDoesNotWriteManifestWhenHooksAreInvalid(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".agents", "plugins", "vibe-pushover")
	hooksPath := filepath.Join(path, "hooks", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(hooksPath, []byte(`{"hooks":`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("goose", path, "/opt/bin/vibe-pushover", ""); err == nil {
		t.Fatal("Install() accepted invalid Goose hooks")
	}
	if _, err := os.Stat(filepath.Join(path, "plugin.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("plugin manifest exists after failed install: %v", err)
	}
}

func TestInstallCreatesOpenCodePlugin(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "opencode", "plugins", "vibe-pushover.ts")
	if _, err := hooks.Install("opencode", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json"); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{
		`event.type === "session.idle"`,
		`event.type === "permission.asked"`,
		`"--agent", "opencode"`,
		`"--config", configPath`,
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("OpenCode plugin does not contain %q:\n%s", want, data)
		}
	}
	changed, err := hooks.Install("opencode", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed OpenCode plugin")
	}
}

func TestInstallAddsCursorStopHook(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".cursor", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"hooks":{"afterFileEdit":[{"command":"existing"}]}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("cursor", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var got struct {
		Version int                         `json:"version"`
		Hooks   map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.Version != 1 || len(got.Hooks["stop"]) != 1 || len(got.Hooks["afterFileEdit"]) != 1 {
		t.Fatalf("Cursor config = %#v", got)
	}
	if got.Hooks["stop"][0]["command"] != "'/opt/bin/vibe-pushover' notify --agent cursor --event turn-complete --ignore-errors" {
		t.Fatalf("Cursor stop hook = %#v", got.Hooks["stop"][0])
	}
	changed, err := hooks.Install("cursor", path, "/opt/bin/vibe-pushover", "")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed Cursor hooks")
	}
}

func TestInstallAddsKimiHooksAndPreservesConfig(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".kimi-code", "config.toml")
	original := `theme = "dark"

[[hooks]]
event = "StopFailure"
command = "existing-hook"
timeout = 10
`
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	changed, err := hooks.Install("kimi", path, "/opt/bin/vibe-pushover", "/tmp/pushover config.json")
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
	for _, want := range []string{
		original,
		`event = "Stop"`,
		`--agent kimi --event turn-complete --ignore-errors --config '/tmp/pushover config.json'`,
		`event = "PermissionRequest"`,
		`--agent kimi --event approval-required --ignore-errors --config '/tmp/pushover config.json'`,
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("Kimi config does not contain %q:\n%s", want, data)
		}
	}
	if got := bytes.Count(data, []byte("[[hooks]]")); got != 3 {
		t.Fatalf("hook count = %d, want 3", got)
	}
}

func TestInstallKimiHooksIsIdempotent(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	if _, err := hooks.Install("kimi", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	changed, err := hooks.Install("kimi", path, "/opt/bin/vibe-pushover", "")
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
	if !bytes.Equal(after, before) {
		t.Fatal("second Install() rewrote the Kimi config")
	}
}

func TestInstallKimiHooksConvertsInlineHooksAndPreservesEntries(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	original := []byte(`# keep this comment
theme = "dark"
hooks = [
  { event = "StopFailure", command = "echo '] # still a string'", timeout = 10 }, # keep hook
]
`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := hooks.Install("kimi", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Contains(data, []byte("# keep this comment")) {
		t.Fatalf("Install() dropped unrelated comments:\n%s", data)
	}
	var got struct {
		Theme string `toml:"theme"`
		Hooks []struct {
			Event   string `toml:"event"`
			Command string `toml:"command"`
			Timeout int    `toml:"timeout"`
		} `toml:"hooks"`
	}
	if err := toml.Unmarshal(data, &got); err != nil {
		t.Fatalf("generated config is invalid TOML: %v\n%s", err, data)
	}
	if got.Theme != "dark" {
		t.Fatalf("theme = %q, want dark", got.Theme)
	}
	if len(got.Hooks) != 3 {
		t.Fatalf("hook count = %d, want 3", len(got.Hooks))
	}
	if got.Hooks[0].Event != "StopFailure" || got.Hooks[0].Command != "echo '] # still a string'" || got.Hooks[0].Timeout != 10 {
		t.Fatalf("existing hook was not preserved: %#v", got.Hooks[0])
	}
	if !bytes.Contains(data, []byte("# keep hook")) {
		t.Fatalf("inline hook comment was removed:\n%s", data)
	}
	changed, err := hooks.Install("kimi", path, "/opt/bin/vibe-pushover", "")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed inline hooks")
	}
}

func TestInstallKimiHooksConvertsEmptyInlineHooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("hooks = []\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("kimi", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var got struct {
		Hooks []map[string]any `toml:"hooks"`
	}
	if err := toml.Unmarshal(data, &got); err != nil {
		t.Fatalf("generated config is invalid TOML: %v\n%s", err, data)
	}
	if len(got.Hooks) != 2 {
		t.Fatalf("hook count = %d, want 2", len(got.Hooks))
	}
}

func TestInstallKimiHooksRecognizesQuotedArrayTable(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	original := []byte("[[\"hooks\"]]\nevent = \"StopFailure\"\ncommand = \"echo keep\"\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("kimi", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Contains(data, original) || bytes.Count(data, []byte("[[hooks]]")) != 2 {
		t.Fatalf("quoted existing hook was not preserved:\n%s", data)
	}
}

func TestInstallKimiHooksIgnoresHookHeaderInsideMultilineString(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	original := []byte("note = '''\n[[hooks]]\n'''\nhooks = []\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("kimi", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var parsed map[string]any
	if err := toml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("generated config is invalid: %v\n%s", err, data)
	}
	hookList, ok := parsed["hooks"].([]any)
	if !ok || len(hookList) != 2 {
		t.Fatalf("hooks = %#v, want two installed hooks", parsed["hooks"])
	}
}

func TestInstallKimiHooksPreservesConfigSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "kimi.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(target, []byte("theme = \"dark\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	path := filepath.Join(dir, "config.toml")
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	if _, err := hooks.Install("kimi", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat() error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("Install() replaced the Kimi config symlink")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(target) error = %v", err)
	}
	if !bytes.Contains(data, []byte(`event = "Stop"`)) {
		t.Fatalf("symlink target was not updated: %s", data)
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

func TestInstallAddsPiTurnCompleteExtension(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".pi", "agent", "extensions", "vibe-pushover", "index.ts")
	changed, err := hooks.Install("pi", path, "/opt/bin/vibe-pushover", "/tmp/pushover config.json")
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
	for _, want := range []string{
		`pi.on("agent_settled"`,
		`"/opt/bin/vibe-pushover"`,
		`"/tmp/pushover config.json"`,
		`"turn-complete"`,
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("Pi extension does not contain %q: %s", want, data)
		}
	}
}

func TestInstallPiExtensionIsIdempotent(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "index.ts")
	if _, err := hooks.Install("pi", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	changed, err := hooks.Install("pi", path, "/opt/bin/vibe-pushover", "")
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
	if !bytes.Equal(after, before) {
		t.Fatal("second Install() rewrote the Pi extension")
	}
}

func TestInstallPiExtensionRefusesToOverwriteForeignFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "index.ts")
	if err := os.WriteFile(path, []byte("export default function custom() {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("pi", path, "/opt/bin/vibe-pushover", ""); err == nil {
		t.Fatal("Install() error = nil, want foreign-file protection error")
	}
}
