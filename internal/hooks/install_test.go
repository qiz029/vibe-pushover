package hooks_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/qiz029/vibe-pushover/internal/hooks"
	"gopkg.in/yaml.v3"
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

func TestDetectedAgentsFindsEverySupportedConfigHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	markers := []string{
		".aider",
		filepath.Join(".config", "amp"),
		filepath.Join(".gemini", "antigravity-cli"),
		".augment",
		".claude",
		filepath.Join("Documents", "Cline"),
		".codebuddy",
		".codewhale",
		".codex",
		".copilot",
		filepath.Join(".snowflake", "cortex"),
		".cursor",
		".factory",
		".craft",
		".gemini",
		filepath.Join(".config", "goose"),
		".grok",
		".hermes",
		".kimi-code",
		".kiro",
		filepath.Join(".config", "kilo"),
		filepath.Join(".config", "mimocode"),
		".vibe",
		".omp",
		".openhands",
		filepath.Join(".config", "opencode"),
		".pi",
		".qoder",
		".qwen",
		".rovodev",
		".tabnine",
		".trae",
		filepath.Join("Library", "Application Support", "Code", "User"),
		filepath.Join(".codeium", "windsurf"),
		".workbuddy",
		".zcode",
	}
	for _, marker := range markers {
		if err := os.MkdirAll(filepath.Join(home, marker), 0o700); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", marker, err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".gemini", "settings.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatalf("WriteFile(Gemini settings) error = %v", err)
	}

	detected, err := hooks.DetectedAgents()
	if err != nil {
		t.Fatalf("DetectedAgents() error = %v", err)
	}
	want := hooks.Agents()
	if len(detected) != len(want) {
		t.Fatalf("DetectedAgents() returned %d agents, want all %d: %#v", len(detected), len(want), detected)
	}
	for index := range want {
		if detected[index].Name != want[index].Name {
			t.Fatalf("DetectedAgents()[%d] = %q, want %q", index, detected[index].Name, want[index].Name)
		}
	}
}

func TestDetectedAgentsDoesNotInferGeminiFromAntigravity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".gemini", "antigravity-cli"), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	detected, err := hooks.DetectedAgents()
	if err != nil {
		t.Fatalf("DetectedAgents() error = %v", err)
	}
	names := make(map[string]bool, len(detected))
	for _, agent := range detected {
		names[agent.Name] = true
	}
	if !names["antigravity"] {
		t.Fatalf("Antigravity was not detected: %#v", detected)
	}
	if names["gemini"] {
		t.Fatalf("Gemini was inferred from Antigravity: %#v", detected)
	}
}

func TestDetectedAgentsHonorsSupportedConfigOverrides(t *testing.T) {
	home := filepath.Join(t.TempDir(), "empty-home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("MkdirAll(HOME) error = %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	overrideRoot := t.TempDir()
	values := map[string]string{
		"CLINE_DIR":             filepath.Join(overrideRoot, "cline"),
		"CODEWHALE_CONFIG_PATH": filepath.Join(overrideRoot, "codewhale", "config.toml"),
		"COPILOT_HOME":          filepath.Join(overrideRoot, "copilot"),
		"GEMINI_CLI_HOME":       filepath.Join(overrideRoot, "gemini"),
		"GROK_HOME":             filepath.Join(overrideRoot, "grok"),
		"HERMES_HOME":           filepath.Join(overrideRoot, "hermes"),
		"KIMI_CODE_HOME":        filepath.Join(overrideRoot, "kimi"),
		"MIMOCODE_HOME":         filepath.Join(overrideRoot, "mimo"),
		"PI_CODING_AGENT_DIR":   filepath.Join(overrideRoot, "pi"),
		"VIBE_HOME":             filepath.Join(overrideRoot, "mistral"),
	}
	for name, value := range values {
		t.Setenv(name, value)
	}
	for _, name := range []string{"CODEWHALE_HOME", "DEEPSEEK_CONFIG_PATH"} {
		t.Setenv(name, "")
	}
	for name, value := range values {
		marker := value
		if name == "CODEWHALE_CONFIG_PATH" {
			marker = filepath.Dir(value)
		}
		if err := os.MkdirAll(marker, 0o700); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", name, err)
		}
	}

	detected, err := hooks.DetectedAgents()
	if err != nil {
		t.Fatalf("DetectedAgents() error = %v", err)
	}
	names := make(map[string]bool, len(detected))
	for _, agent := range detected {
		names[agent.Name] = true
	}
	for _, want := range []string{"cline", "codewhale", "copilot", "gemini", "grok", "hermes", "kimi", "mimo", "mistral", "vscode"} {
		if !names[want] {
			t.Errorf("DetectedAgents() omitted %q with its supported override: %#v", want, detected)
		}
	}
	for _, ambiguous := range []string{"omp", "pi"} {
		if names[ambiguous] {
			t.Errorf("DetectedAgents() inferred %q from ambiguous PI_CODING_AGENT_DIR: %#v", ambiguous, detected)
		}
	}
}

func TestDefaultPathOMPHonorsAgentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "custom-omp-agent")
	t.Setenv("PI_CODING_AGENT_DIR", dir)
	t.Setenv("HOME", "")

	got, err := hooks.DefaultPath("omp")
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	want := filepath.Join(dir, "extensions", "vibe-pushover", "index.ts")
	if got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestDefaultPathMiMoHonorsMiMoCodeHome(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mimocode-home")
	t.Setenv("MIMOCODE_HOME", dir)
	t.Setenv("HOME", "")

	got, err := hooks.DefaultPath("mimo")
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	want := filepath.Join(dir, "config", "plugins", "vibe-pushover.ts")
	if got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestDefaultPathMiMoRejectsRelativeMiMoCodeHome(t *testing.T) {
	t.Setenv("MIMOCODE_HOME", "relative/home")

	if _, err := hooks.DefaultPath("mimo"); err == nil {
		t.Fatal("DefaultPath() accepted relative MIMOCODE_HOME")
	}
}

func TestDefaultPathKiloRejectsRelativeXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "relative/config")

	if _, err := hooks.DefaultPath("kilo"); err == nil {
		t.Fatal("DefaultPath() accepted relative XDG_CONFIG_HOME for Kilo Code")
	}
}

func TestDefaultPathsForAdditionalAgents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Keep the default Documents fallback deterministic on Linux runners that
	// may have xdg-user-dir installed with host-specific configuration.
	t.Setenv("PATH", "")
	t.Setenv("COPILOT_HOME", "~/copilot-home")
	t.Setenv("GEMINI_CLI_HOME", "~/gemini-home")
	t.Setenv("XDG_CONFIG_HOME", "~/.xdg")

	tests := map[string]string{
		"aider":       filepath.Join(home, ".aider.conf.yml"),
		"amp":         filepath.Join(home, ".config", "amp", "plugins", "vibe-pushover.ts"),
		"antigravity": filepath.Join(home, ".gemini", "antigravity-cli", "plugins", "vibe-pushover"),
		"auggie":      filepath.Join(home, ".augment", "settings.json"),
		"cline":       filepath.Join(home, "Documents", "Cline", "Hooks", "TaskComplete"),
		"codebuddy":   filepath.Join(home, ".codebuddy", "settings.json"),
		"codewhale":   filepath.Join(home, ".codewhale", "config.toml"),
		"copilot":     filepath.Join(home, "copilot-home", "hooks", "vibe-pushover.json"),
		"cortex":      filepath.Join(home, ".snowflake", "cortex", "hooks.json"),
		"droid":       filepath.Join(home, ".factory", "settings.json"),
		"dotcraft":    filepath.Join(home, ".craft", "hooks.json"),
		"gemini":      filepath.Join(home, "gemini-home", ".gemini", "settings.json"),
		"goose":       filepath.Join(home, ".agents", "plugins", "vibe-pushover"),
		"hermes":      filepath.Join(home, ".hermes", "config.yaml"),
		"kiro":        filepath.Join(home, ".kiro", "hooks", "vibe-pushover.json"),
		"kilo":        filepath.Join(home, ".xdg", "kilo", "plugin", "vibe-pushover.ts"),
		"mimo":        filepath.Join(home, ".xdg", "mimocode", "plugins", "vibe-pushover.ts"),
		"opencode":    filepath.Join(home, ".xdg", "opencode", "plugins", "vibe-pushover.ts"),
		"omp":         filepath.Join(home, ".omp", "agent", "extensions", "vibe-pushover", "index.ts"),
		"openhands":   filepath.Join(home, ".openhands", "hooks.json"),
		"qoder":       filepath.Join(home, ".qoder", "settings.json"),
		"qwen":        filepath.Join(home, ".qwen", "settings.json"),
		"rovo":        filepath.Join(home, ".rovodev", "config.yml"),
		"tabnine":     filepath.Join(home, ".tabnine", "agent", "settings.json"),
		"trae":        filepath.Join(home, ".trae", "hooks.json"),
		"vscode":      filepath.Join(home, "copilot-home", "hooks", "vibe-pushover.json"),
		"windsurf":    filepath.Join(home, ".codeium", "windsurf", "hooks.json"),
		"workbuddy":   filepath.Join(home, ".workbuddy", "settings.json"),
		"zcode":       filepath.Join(home, ".zcode", "cli", "config.json"),
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

func TestDefaultPathCodeWhaleUsesLegacyConfigWhenPrimaryIsMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEWHALE_HOME", "")
	t.Setenv("CODEWHALE_CONFIG_PATH", "")
	t.Setenv("DEEPSEEK_CONFIG_PATH", "")
	legacy := filepath.Join(home, ".deepseek", "config.toml")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(legacy, []byte("provider = \"deepseek\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	got, err := hooks.DefaultPath("codewhale")
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	if got != legacy {
		t.Fatalf("DefaultPath() = %q, want legacy %q", got, legacy)
	}
}

func TestDefaultPathCodeWhaleHonorsOfficialOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEWHALE_HOME", "~/whale-home")
	t.Setenv("CODEWHALE_CONFIG_PATH", "~/explicit/config.toml")
	t.Setenv("DEEPSEEK_CONFIG_PATH", "~/legacy-explicit/config.toml")
	got, err := hooks.DefaultPath("codewhale")
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	want := filepath.Join(home, "explicit", "config.toml")
	if got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestInstallConfiguresAiderNotificationCommand(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "home with spaces", ".aider.conf.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	original := "# personal aider settings\nmodel: sonnet\nread:\n  - README.md\n# keep notification note\nnotifications: false\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	changed, err := hooks.Install("aider", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !changed {
		t.Fatal("Install() changed = false, want true")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(config) error = %v", err)
	}
	wrapperPath := filepath.Join(filepath.Dir(path), ".aider", "vibe-pushover-notify.sh")
	for _, want := range []string{
		"# personal aider settings",
		"# keep notification note",
		"model: sonnet",
		"- README.md",
		"notifications: true",
		wrapperPath,
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("Aider config does not contain %q:\n%s", want, data)
		}
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal(config) error = %v", err)
	}
	if parsed["notifications"] != true || parsed["notifications-command"] != "'"+wrapperPath+"'" {
		t.Fatalf("Aider notification settings = %#v", parsed)
	}
	wrapper, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatalf("ReadFile(wrapper) error = %v", err)
	}
	for _, want := range []string{
		"#!/bin/sh",
		"# Generated by vibe-pushover.",
		`{"message":"Aider is ready for input."}`,
		"'/opt/bin/vibe-pushover' notify --agent aider --event turn-complete --ignore-errors --config '/tmp/pushover.json'",
	} {
		if !bytes.Contains(wrapper, []byte(want)) {
			t.Fatalf("Aider wrapper does not contain %q:\n%s", want, wrapper)
		}
	}
	info, err := os.Stat(wrapperPath)
	if err != nil {
		t.Fatalf("Stat(wrapper) error = %v", err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("wrapper mode = %o, want executable", info.Mode().Perm())
	}
	changed, err = hooks.Install("aider", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed Aider integration")
	}
}

func TestInstallAiderAcceptsEmptyConfig(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".aider.conf.yml")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("aider", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Contains(data, []byte("notifications: true")) {
		t.Fatalf("Aider config = %s", data)
	}
}

func TestInstallAiderRefusesUnownedWrapper(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".aider.conf.yml")
	wrapperPath := filepath.Join(dir, ".aider", "vibe-pushover-notify.sh")
	if err := os.MkdirAll(filepath.Dir(wrapperPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	original := []byte("#!/bin/sh\necho personal-script\n")
	if err := os.WriteFile(wrapperPath, original, 0o700); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("aider", path, "/opt/bin/vibe-pushover", ""); err == nil {
		t.Fatal("Install() overwrote an unowned Aider wrapper")
	}
	got, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("unowned wrapper changed: %q", got)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Aider config exists after refused install: %v", err)
	}
}

func TestInstallAiderRefusesExistingNotificationCommand(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".aider.conf.yml")
	original := []byte("model: sonnet\nnotifications: true\nnotifications-command: personal-notifier\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("aider", path, "/opt/bin/vibe-pushover", ""); err == nil {
		t.Fatal("Install() replaced an existing Aider notification command")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("Aider config changed after refused install: %s", got)
	}
	wrapperPath := filepath.Join(filepath.Dir(path), ".aider", "vibe-pushover-notify.sh")
	if _, err := os.Stat(wrapperPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Aider wrapper exists after refused install: %v", err)
	}
}

func TestInstallCreatesAmpLifecyclePlugin(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".config", "amp", "plugins", "vibe-pushover.ts")
	changed, err := hooks.Install("amp", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
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
		"// Generated by vibe-pushover.",
		`amp.on("agent.start"`,
		`amp.on("agent.end"`,
		`event.status === "done"`,
		`ctx.thread.state.subscribe`,
		`state === "awaiting-approval"`,
		`amp.helpers.filePathFromURI(amp.system.workspaceRoot)`,
		`["notify", "--agent", "amp", "--event", eventName, "--ignore-errors"]`,
		`"approval-required"`,
		`"attention-required"`,
		`subscriptions.delete(event.thread.id)`,
		`last_assistant_message: assistantText(event.messages)`,
		`const configPath = "/tmp/pushover.json"`,
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("Amp plugin does not contain %q:\n%s", want, data)
		}
	}
	changed, err = hooks.Install("amp", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed Amp plugin")
	}
}

func TestInstallAmpRefusesUnownedPlugin(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "vibe-pushover.ts")
	original := []byte("// personal plugin\n// Generated by vibe-pushover.\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("amp", path, "/opt/bin/vibe-pushover", ""); err == nil {
		t.Fatal("Install() overwrote an unowned Amp plugin")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("unowned plugin changed: %q", got)
	}
}

func TestInstallCreatesKiroCompletionHook(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".kiro", "hooks", "vibe-pushover.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"version":"v1","hooks":[{"name":"keep","trigger":"PostFileSave","action":{"type":"command","command":"npm test"},"enabled":true}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	changed, err := hooks.Install("kiro", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
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
		Version string           `json:"version"`
		Hooks   []map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.Version != "v1" || len(got.Hooks) != 2 || got.Hooks[0]["name"] != "keep" {
		t.Fatalf("Kiro config did not preserve existing hooks: %#v", got)
	}
	owned := got.Hooks[1]
	action, _ := owned["action"].(map[string]any)
	wantCommand := "'/opt/bin/vibe-pushover' notify --agent kiro --event turn-complete --ignore-errors --config '/tmp/pushover.json'"
	if owned["name"] != "vibe-pushover-turn-complete" || owned["trigger"] != "Stop" || owned["timeout"] != float64(10) || owned["enabled"] != true || action["type"] != "command" || action["command"] != wantCommand {
		t.Fatalf("Kiro completion hook = %#v", owned)
	}
	changed, err = hooks.Install("kiro", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed Kiro hook")
	}
}

func TestInstallAddsMistralVibeCompletionHook(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), ".vibe")
	hooksPath := filepath.Join(dir, "hooks.toml")
	configPath := filepath.Join(dir, "config.toml")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(hooksPath, []byte("# personal hook\n[[hooks]]\nname = \"lint\"\ntype = \"after_tool\"\nmatch = \"bash\"\ncommand = \"make lint\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(hooks) error = %v", err)
	}
	if err := os.WriteFile(configPath, []byte("# personal config\nactive_model = \"devstral\"\nenable_experimental_hooks = false\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}

	changed, err := hooks.Install("mistral", hooksPath, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !changed {
		t.Fatal("Install() changed = false, want true")
	}
	hooksData, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("ReadFile(hooks) error = %v", err)
	}
	var hooksRoot struct {
		Hooks []struct {
			Name        string  `toml:"name"`
			Type        string  `toml:"type"`
			Command     string  `toml:"command"`
			Timeout     float64 `toml:"timeout"`
			Description string  `toml:"description"`
		} `toml:"hooks"`
	}
	if err := toml.Unmarshal(hooksData, &hooksRoot); err != nil {
		t.Fatalf("Unmarshal(hooks) error = %v\n%s", err, hooksData)
	}
	if len(hooksRoot.Hooks) != 2 || hooksRoot.Hooks[0].Name != "lint" {
		t.Fatalf("Mistral hooks did not preserve existing entries: %#v", hooksRoot.Hooks)
	}
	owned := hooksRoot.Hooks[1]
	wantCommand := "'/opt/bin/vibe-pushover' notify --agent mistral --event turn-complete --ignore-errors --skip-mistral-subagent --config '/tmp/pushover.json'"
	if owned.Name != "vibe-pushover-turn-complete" || owned.Type != "post_agent_turn" || owned.Command != wantCommand || owned.Timeout != 10 || owned.Description == "" {
		t.Fatalf("Mistral completion hook = %#v", owned)
	}
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config) error = %v", err)
	}
	var configRoot map[string]any
	if err := toml.Unmarshal(configData, &configRoot); err != nil {
		t.Fatalf("Unmarshal(config) error = %v\n%s", err, configData)
	}
	if configRoot["enable_experimental_hooks"] != true || configRoot["active_model"] != "devstral" || !bytes.Contains(configData, []byte("# personal config")) {
		t.Fatalf("Mistral config not enabled or not preserved: %#v\n%s", configRoot, configData)
	}
	changed, err = hooks.Install("mistral", hooksPath, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed Mistral Vibe integration")
	}
}

func TestInstallAddsGrokBuildCompletionAndFailureHooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".grok", "hooks", "vibe-pushover.json")
	changed, err := hooks.Install("grok", path, "/opt/bin/vibe-pushover", "/tmp/pushover config.json")
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
		Hooks map[string][]struct {
			Hooks []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
				Timeout int    `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(got.Hooks["Stop"]) != 1 || len(got.Hooks["StopFailure"]) != 1 {
		t.Fatalf("Grok hooks = %#v", got.Hooks)
	}
	completion := got.Hooks["Stop"][0].Hooks[0]
	failure := got.Hooks["StopFailure"][0].Hooks[0]
	if completion.Type != "command" || completion.Timeout != 10 || completion.Command != "'/opt/bin/vibe-pushover' notify --agent grok --event turn-complete --ignore-errors --config '/tmp/pushover config.json'" {
		t.Fatalf("Grok Stop hook = %#v", completion)
	}
	if failure.Type != "command" || failure.Timeout != 10 || failure.Command != "'/opt/bin/vibe-pushover' notify --agent grok --event attention-required --ignore-errors --config '/tmp/pushover config.json'" {
		t.Fatalf("Grok StopFailure hook = %#v", failure)
	}
	changed, err = hooks.Install("grok", path, "/opt/bin/vibe-pushover", "/tmp/pushover config.json")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed Grok Build hooks")
	}
}

func TestInstallGrokBuildPreservesHookSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior is platform-specific")
	}
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "shared-hooks.json")
	path := filepath.Join(dir, ".grok", "hooks", "vibe-pushover.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(target, []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	if _, err := hooks.Install("grok", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat() error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("Install() replaced Grok hook symlink")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(target) error = %v", err)
	}
	if !bytes.Contains(data, []byte(`"Stop"`)) || !bytes.Contains(data, []byte(`"theme": "dark"`)) {
		t.Fatalf("symlink target not updated or sibling config lost: %s", data)
	}
}

func TestInstallMistralVibeAddsFeatureFlagBeforeExistingTables(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), ".vibe")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	hooksPath := filepath.Join(dir, "hooks.toml")
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte("[tools.bash]\npermission = \"ask\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	if _, err := hooks.Install("mistral", hooksPath, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config) error = %v", err)
	}
	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		t.Fatalf("Unmarshal(config) error = %v\n%s", err, data)
	}
	if root["enable_experimental_hooks"] != true {
		t.Fatalf("enable_experimental_hooks = %#v, want true\n%s", root["enable_experimental_hooks"], data)
	}
	tools, _ := root["tools"].(map[string]any)
	bash, _ := tools["bash"].(map[string]any)
	if bash["permission"] != "ask" {
		t.Fatalf("tools.bash was not preserved: %#v", root)
	}
}

func TestInstallMistralVibeRefusesUnownedHook(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), ".vibe")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	hooksPath := filepath.Join(dir, "hooks.toml")
	configPath := filepath.Join(dir, "config.toml")
	originalHooks := []byte("[[hooks]]\nname = \"vibe-pushover-turn-complete\"\ntype = \"post_agent_turn\"\ncommand = \"personal-notifier\"\n")
	originalConfig := []byte("enable_experimental_hooks = false\n")
	if err := os.WriteFile(hooksPath, originalHooks, 0o600); err != nil {
		t.Fatalf("WriteFile(hooks) error = %v", err)
	}
	if err := os.WriteFile(configPath, originalConfig, 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	if _, err := hooks.Install("mistral", hooksPath, "/opt/bin/vibe-pushover", ""); err == nil {
		t.Fatal("Install() replaced an unowned Mistral Vibe hook")
	}
	gotHooks, _ := os.ReadFile(hooksPath)
	gotConfig, _ := os.ReadFile(configPath)
	if !bytes.Equal(gotHooks, originalHooks) || !bytes.Equal(gotConfig, originalConfig) {
		t.Fatalf("Mistral files changed after refused install:\nhooks:\n%s\nconfig:\n%s", gotHooks, gotConfig)
	}
}

func TestInstallMistralVibeRefusesUnownedDuplicateBesideManagedHook(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), ".vibe")
	hooksPath := filepath.Join(dir, "hooks.toml")
	if _, err := hooks.Install("mistral", hooksPath, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	managed, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("ReadFile(hooks) error = %v", err)
	}
	original := append(append([]byte(nil), managed...), []byte("\n[[hooks]]\nname = \"vibe-pushover-turn-complete\"\ntype = \"post_agent_turn\"\ncommand = \"personal-notifier\"\n")...)
	if err := os.WriteFile(hooksPath, original, 0o600); err != nil {
		t.Fatalf("WriteFile(hooks) error = %v", err)
	}
	if _, err := hooks.Install("mistral", hooksPath, "/opt/bin/vibe-pushover", ""); err == nil {
		t.Fatal("Install() accepted an unowned duplicate beside the managed hook")
	}
	got, _ := os.ReadFile(hooksPath)
	if !bytes.Equal(got, original) {
		t.Fatalf("Mistral hooks changed after refused install:\n%s", got)
	}
}

func TestInstallMistralVibePreservesConfigSymlinks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	vibeDir := filepath.Join(dir, ".vibe")
	dotfilesDir := filepath.Join(dir, "dotfiles")
	if err := os.MkdirAll(vibeDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(vibe) error = %v", err)
	}
	if err := os.MkdirAll(dotfilesDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(dotfiles) error = %v", err)
	}
	hooksTarget := filepath.Join(dotfilesDir, "hooks.toml")
	configTarget := filepath.Join(dotfilesDir, "config.toml")
	if err := os.WriteFile(hooksTarget, []byte("# hooks\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(hooks target) error = %v", err)
	}
	if err := os.WriteFile(configTarget, []byte("# config\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(config target) error = %v", err)
	}
	hooksPath := filepath.Join(vibeDir, "hooks.toml")
	configPath := filepath.Join(vibeDir, "config.toml")
	if err := os.Symlink(hooksTarget, hooksPath); err != nil {
		t.Fatalf("Symlink(hooks) error = %v", err)
	}
	if err := os.Symlink(configTarget, configPath); err != nil {
		t.Fatalf("Symlink(config) error = %v", err)
	}
	if _, err := hooks.Install("mistral", hooksPath, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	for _, path := range []string{hooksPath, configPath} {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("Lstat(%q) error = %v", path, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("Install() replaced symlink %q", path)
		}
	}
	hooksData, _ := os.ReadFile(hooksTarget)
	configData, _ := os.ReadFile(configTarget)
	if !bytes.Contains(hooksData, []byte("vibe-pushover-turn-complete")) || !bytes.Contains(configData, []byte("enable_experimental_hooks = true")) {
		t.Fatalf("symlink targets were not updated:\nhooks:\n%s\nconfig:\n%s", hooksData, configData)
	}
}

func TestInstallMistralVibeIgnoresMarkersInsideMultilineString(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), ".vibe")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	hooksPath := filepath.Join(dir, "hooks.toml")
	original := "note = '''\n# BEGIN vibe-pushover hook: post_agent_turn\nnot a managed block\n# END vibe-pushover hook: post_agent_turn\n'''\n"
	if err := os.WriteFile(hooksPath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile(hooks) error = %v", err)
	}
	if _, err := hooks.Install("mistral", hooksPath, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("ReadFile(hooks) error = %v", err)
	}
	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		t.Fatalf("Unmarshal(hooks) error = %v\n%s", err, data)
	}
	if root["note"] != "# BEGIN vibe-pushover hook: post_agent_turn\nnot a managed block\n# END vibe-pushover hook: post_agent_turn\n" {
		t.Fatalf("multiline string changed: %#v", root["note"])
	}
	hookEntries, _ := root["hooks"].([]any)
	if len(hookEntries) != 1 {
		t.Fatalf("installed hooks = %#v, want one managed hook\n%s", root["hooks"], data)
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

func TestDefaultPathMistralHonorsVibeHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("VIBE_HOME", "~/custom-vibe")

	got, err := hooks.DefaultPath("mistral")
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	want := filepath.Join(home, "custom-vibe", "hooks.toml")
	if got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestDefaultPathGrokHonorsGrokHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GROK_HOME", "~/custom-grok")

	got, err := hooks.DefaultPath("grok")
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	want := filepath.Join(home, "custom-grok", "hooks", "vibe-pushover.json")
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
	if !bytes.Contains(data, []byte(`"timeout": 10000`)) {
		t.Fatalf("Gemini hook timeout is not expressed in milliseconds: %s", data)
	}
	changed, err = hooks.Install("gemini", path, "/opt/bin/vibe-pushover", "")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed Gemini hooks")
	}
}

func TestInstallAddsQwenCompletionApprovalAndIdleAttentionHooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".qwen", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"theme":"dark","hooks":{"BeforeToolUse":[{"hooks":[{"type":"command","command":"existing"}]}]}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	changed, err := hooks.Install("qwen", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
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
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for _, event := range []string{"Stop", "PermissionRequest", "Notification"} {
		if len(got.Hooks[event]) != 1 {
			t.Fatalf("%s hook count = %d, want 1", event, len(got.Hooks[event]))
		}
		commands, _ := got.Hooks[event][0]["hooks"].([]any)
		command, _ := commands[0].(map[string]any)
		if command["timeout"] != float64(10000) || command["async"] != true {
			t.Fatalf("%s command = %#v, want 10000ms async hook", event, command)
		}
	}
	if got.Hooks["Notification"][0]["matcher"] != "idle_prompt" {
		t.Fatalf("Notification matcher = %#v, want idle_prompt", got.Hooks["Notification"][0]["matcher"])
	}
	for _, want := range []string{
		"--agent qwen --event turn-complete",
		"--skip-active-qwen-stop",
		"--agent qwen --event approval-required",
		"--agent qwen --event attention-required",
		"--config '/tmp/pushover.json'",
		`"theme": "dark"`,
		`"BeforeToolUse"`,
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("Qwen config does not contain %q:\n%s", want, data)
		}
	}
	changed, err = hooks.Install("qwen", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed Qwen hooks")
	}
}

func TestInstallAddsTraeCompletionAndApprovalHooksWithoutReplacingExistingHooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".trae", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"theme":"dark","hooks":{"Stop":[{"hooks":[{"type":"command","command":"/Users/todd/.vibe-island/bin/vibe-island-bridge --source trae"}]}],"Notification":[{"hooks":[{"type":"command","command":"/Users/todd/.vibe-island/bin/vibe-island-bridge --source trae"}]}]}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	changed, err := hooks.Install("trae", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
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
		Version int                         `json:"version"`
		Theme   string                      `json:"theme"`
		Hooks   map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.Version != 1 || got.Theme != "dark" {
		t.Fatalf("TRAE sibling config changed: version=%d theme=%q", got.Version, got.Theme)
	}
	if len(got.Hooks["Stop"]) != 2 || len(got.Hooks["Notification"]) != 2 {
		t.Fatalf("TRAE hooks = %#v, want existing and vibe-pushover entries", got.Hooks)
	}
	if got.Hooks["Notification"][1]["matcher"] != "permission_prompt" {
		t.Fatalf("TRAE Notification matcher = %#v, want permission_prompt", got.Hooks["Notification"][1]["matcher"])
	}
	for _, want := range []string{
		"vibe-island-bridge --source trae",
		"--agent trae --event turn-complete --ignore-errors",
		"--agent trae --event approval-required --ignore-errors",
		"--config '/tmp/pushover.json'",
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("TRAE config does not contain %q:\n%s", want, data)
		}
	}
	changed, err = hooks.Install("trae", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed TRAE hooks")
	}
}

func TestInstallCreatesVersionedTraeHookConfig(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".trae", "hooks.json")
	if _, err := hooks.Install("trae", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var got struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.Version != 1 {
		t.Fatalf("TRAE hook config version = %d, want 1", got.Version)
	}
}

func TestInstallTraePreservesHookSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior is platform-specific")
	}
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "shared-trae-hooks.json")
	path := filepath.Join(dir, ".trae", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(target, []byte(`{"version":1,"theme":"dark"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	if _, err := hooks.Install("trae", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat() error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("Install() replaced TRAE hook symlink")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(target) error = %v", err)
	}
	if !bytes.Contains(data, []byte(`"Stop"`)) || !bytes.Contains(data, []byte(`"theme": "dark"`)) {
		t.Fatalf("symlink target not updated or sibling config lost: %s", data)
	}
}

func TestInstallTraeSeparatesManagedHookBeforeChangingSharedMatcher(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".trae", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	content := `{"version":1,"hooks":{"Notification":[{"matcher":"*","hooks":[{"type":"command","command":"third-party-notifier"},{"type":"command","command":"'/old/vibe-pushover' notify --agent trae --event approval-required --ignore-errors"}]}]}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("trae", path, "/new/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var got struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	groups := got.Hooks["Notification"]
	if len(groups) != 2 {
		t.Fatalf("Notification group count = %d, want 2: %s", len(groups), data)
	}
	if groups[0].Matcher != "*" || len(groups[0].Hooks) != 1 || groups[0].Hooks[0].Command != "third-party-notifier" {
		t.Fatalf("third-party group changed = %#v", groups[0])
	}
	if groups[1].Matcher != "permission_prompt" || len(groups[1].Hooks) != 1 || !strings.Contains(groups[1].Hooks[0].Command, "/new/vibe-pushover") {
		t.Fatalf("managed group = %#v, want isolated permission_prompt hook", groups[1])
	}
}

func TestInstallTraeValidatesNumericManifestVersion(t *testing.T) {
	t.Parallel()

	t.Run("accepts numeric one", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "hooks.json")
		if err := os.WriteFile(path, []byte(`{"version":1.0}`), 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if _, err := hooks.Install("trae", path, "/opt/bin/vibe-pushover", ""); err != nil {
			t.Fatalf("Install() rejected numeric version 1.0: %v", err)
		}
	})

	t.Run("rejects string one", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "hooks.json")
		original := []byte(`{"version":"1","theme":"dark"}`)
		if err := os.WriteFile(path, original, 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if _, err := hooks.Install("trae", path, "/opt/bin/vibe-pushover", ""); err == nil {
			t.Fatal("Install() accepted string manifest version")
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		if !bytes.Equal(got, original) {
			t.Fatalf("invalid TRAE config changed: %s", got)
		}
	})
}

func TestInstallAddsQoderCompletionHook(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".qoder", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"theme":"dark","hooks":{"Stop":[{"hooks":[{"type":"command","command":"existing"}]}]}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	changed, err := hooks.Install("qoder", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
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
		`"theme": "dark"`,
		`"command": "existing"`,
		"--agent qoder --event turn-complete --ignore-errors --skip-active-stop",
		`"timeout": 10`,
		"--config '/tmp/pushover.json'",
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("Qoder config does not contain %q:\n%s", want, data)
		}
	}
	changed, err = hooks.Install("qoder", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed Qoder hooks")
	}
}

func TestInstallAddsCortexCompletionAndApprovalHooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".snowflake", "cortex", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"theme":"dark","hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"existing"}]}]}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	changed, err := hooks.Install("cortex", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
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
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for _, event := range []string{"Stop", "PermissionRequest"} {
		if len(got.Hooks[event]) != 1 {
			t.Fatalf("%s hook count = %d, want 1", event, len(got.Hooks[event]))
		}
		commands, _ := got.Hooks[event][0]["hooks"].([]any)
		command, _ := commands[0].(map[string]any)
		if command["timeout"] != float64(10) {
			t.Fatalf("%s timeout = %#v, want 10 seconds", event, command["timeout"])
		}
		if _, exists := command["async"]; exists {
			t.Fatalf("%s command unexpectedly contains unsupported async field: %#v", event, command)
		}
	}
	for _, want := range []string{
		`"theme": "dark"`, `"command": "existing"`,
		"--agent cortex --event turn-complete --ignore-errors",
		"--agent cortex --event approval-required --ignore-errors",
		"--config '/tmp/pushover.json'",
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("Cortex config does not contain %q:\n%s", want, data)
		}
	}
	changed, err = hooks.Install("cortex", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed Cortex hooks")
	}
}

func TestInstallAddsOhMyPiCompletionAndApprovalExtension(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".omp", "agent", "extensions", "vibe-pushover", "index.ts")
	changed, err := hooks.Install("omp", path, "/opt/bin/vibe-pushover", "/tmp/pushover config.json")
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
		`pi.on("agent_end"`, `event.willContinue`, `lastAssistantText(event.messages)`,
		`pi.on("tool_approval_requested"`, `tool_name: event.toolName`, `reason: event.reason`,
		`"--agent"`, `"omp"`, `"turn-complete"`, `"approval-required"`,
		`const configPath = "/tmp/pushover config.json"`,
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("Oh My Pi extension does not contain %q:\n%s", want, data)
		}
	}
	changed, err = hooks.Install("omp", path, "/opt/bin/vibe-pushover", "/tmp/pushover config.json")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed Oh My Pi extension")
	}
}

func TestInstallRefusesToOverwriteUnownedOhMyPiExtension(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "index.ts")
	if err := os.WriteFile(path, []byte("export default function custom() {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	changed, err := hooks.Install("omp", path, "/opt/bin/vibe-pushover", "")
	if err == nil || changed {
		t.Fatalf("Install() = changed %v, error %v; want refusal", changed, err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	if string(data) != "export default function custom() {}\n" {
		t.Fatalf("unowned extension was modified: %q", data)
	}
}

func TestInstallAddsHermesCompletionAndApprovalHooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".hermes", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	original := "# personal config\nmodel: nous\nhooks:\n  pre_tool_call:\n    - matcher: terminal\n      command: /usr/local/bin/check-hook\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	changed, err := hooks.Install("hermes", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
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
		"# personal config", "model: nous", "pre_tool_call:", "command: /usr/local/bin/check-hook",
		"post_llm_call:", "--agent hermes --event turn-complete --ignore-errors",
		"pre_approval_request:", "--agent hermes --event approval-required --ignore-errors --skip-noninteractive-approval",
		"timeout: 10", "--config '/tmp/pushover.json'",
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("Hermes config does not contain %q:\n%s", want, data)
		}
	}
	var decoded map[string]any
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("YAML is invalid: %v", err)
	}
	changed, err = hooks.Install("hermes", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed Hermes hooks")
	}
}

func TestInstallHermesReportsHermesForMalformedHook(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".hermes", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("hooks:\n  post_llm_call:\n    - command:\n        nested: invalid\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, err := hooks.Install("hermes", path, "/opt/bin/vibe-pushover", "")
	if err == nil {
		t.Fatal("Install() error = nil, want malformed Hermes hook error")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("Hermes hook")) || bytes.Contains([]byte(err.Error()), []byte("Aider")) {
		t.Fatalf("Install() error = %q, want Hermes-specific context", err)
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
		`const childSessions = new Set();`,
		`const erroredSessions = new Set();`,
		`event.type === "session.created"`,
		`event.type === "session.deleted"`,
		`event.properties?.info?.parentID`,
		`childSessions.has(sessionID)`,
		`childSessions.delete(deletedSessionID)`,
		`event.type === "session.status"`,
		`event.properties?.status?.type !== "idle"`,
		`erroredSessions.delete(event.properties.sessionID)`,
		`event.type === "session.idle"`,
		`event.type === "permission.asked"`,
		`event.type === "session.error"`,
		`erroredSessions.add(sessionID)`,
		`erroredSessions.delete(sessionID)`,
		`error?.data?.message`,
		`"attention-required"`,
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

func TestInstallCreatesKiloCodePlugin(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "kilo", "plugin", "vibe-pushover.ts")
	changed, err := hooks.Install("kilo", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
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
		`const VibePushover = async`,
		`export default { id: "vibe-pushover", server: VibePushover };`,
		`const erroredSessions = new Set();`,
		`event.type === "session.status"`,
		`event.properties?.status?.type !== "idle"`,
		`erroredSessions.delete(event.properties.sessionID)`,
		`event.type === "session.idle"`,
		`event.type === "permission.asked"`,
		`event.type === "session.error"`,
		`"attention-required"`,
		`erroredSessions.add(sessionID)`,
		`erroredSessions.delete(sessionID)`,
		`error?.data?.message`,
		`event.type === "session.error"`,
		`"attention-required"`,
		`childSessions.has(sessionID)`,
		`"--agent", "kilo"`,
		`"--config", configPath`,
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("Kilo Code plugin does not contain %q:\n%s", want, data)
		}
	}
	changed, err = hooks.Install("kilo", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed Kilo Code plugin")
	}
}

func TestInstallAddsDotCraftLifecycleHooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".craft", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	original := `{"hooks":{"PostToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"personal-audit","timeout":3}]}]}}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	changed, err := hooks.Install("dotcraft", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
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
	type dotCraftCommand struct {
		Command string `json:"command"`
	}
	type dotCraftGroup struct {
		Hooks []dotCraftCommand `json:"hooks"`
	}
	var got struct {
		Hooks map[string][]dotCraftGroup `json:"hooks"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(got.Hooks["PostToolUse"]) != 1 || got.Hooks["PostToolUse"][0].Hooks[0].Command != "personal-audit" {
		t.Fatalf("third-party DotCraft hook changed: %#v", got.Hooks["PostToolUse"])
	}
	wants := map[string]string{
		"Stop":              "turn-complete",
		"StopFailure":       "attention-required",
		"PermissionRequest": "approval-required",
	}
	for hookName, event := range wants {
		groups := got.Hooks[hookName]
		if len(groups) != 1 || len(groups[0].Hooks) != 1 {
			t.Fatalf("DotCraft %s hook = %#v", hookName, groups)
		}
		command := groups[0].Hooks[0].Command
		if !strings.Contains(command, "notify --agent dotcraft --event "+event+" --ignore-errors") || !strings.Contains(command, "--config '/tmp/pushover.json'") {
			t.Fatalf("DotCraft %s command = %q", hookName, command)
		}
	}
	if !strings.Contains(got.Hooks["Stop"][0].Hooks[0].Command, "--skip-active-stop") {
		t.Fatalf("DotCraft Stop command does not filter re-entry: %q", got.Hooks["Stop"][0].Hooks[0].Command)
	}
	changed, err = hooks.Install("dotcraft", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed DotCraft hooks")
	}
}

func TestInstallCreatesMiMoCodePlugin(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "mimocode", "plugins", "vibe-pushover.ts")
	changed, err := hooks.Install("mimo", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
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
		`const childSessions = new Set();`,
		`event.type === "session.created"`,
		`event.type === "session.deleted"`,
		`event.properties?.info?.parentID`,
		`childSessions.has(sessionID)`,
		`childSessions.delete(deletedSessionID)`,
		`event.type === "session.idle"`,
		`event.type === "permission.asked"`,
		`"--agent", "mimo"`,
		`"/tmp/pushover.json"`,
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("MiMo Code plugin does not contain %q:\n%s", want, data)
		}
	}
	changed, err = hooks.Install("mimo", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed MiMo Code plugin")
	}
}

func TestInstallMiMoCodePluginRefusesNonOwnedFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "mimocode", "plugins", "vibe-pushover.ts")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	original := []byte("// User-owned MiMo plugin.\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("mimo", path, "/opt/bin/vibe-pushover", ""); err == nil {
		t.Fatal("Install() overwrote a non-owned MiMo Code plugin")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("non-owned MiMo Code plugin changed:\n%s", got)
	}
}

func TestInstallMiMoCodePluginPreservesSymlinkOnUpdate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior is platform-specific")
	}
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "shared-vibe-pushover.ts")
	if _, err := hooks.Install("mimo", target, "/old/vibe-pushover", "/old/config.json"); err != nil {
		t.Fatalf("initial Install() error = %v", err)
	}
	path := filepath.Join(dir, "mimocode", "plugins", "vibe-pushover.ts")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	if _, err := hooks.Install("mimo", path, "/new/vibe-pushover", "/new/config.json"); err != nil {
		t.Fatalf("update Install() error = %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat() error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("Install() replaced MiMo Code plugin symlink")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(target) error = %v", err)
	}
	if !bytes.Contains(data, []byte(`/new/vibe-pushover`)) || !bytes.Contains(data, []byte(`/new/config.json`)) {
		t.Fatalf("symlink target was not updated:\n%s", data)
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

func TestInstallAddsWindsurfPostResponseHook(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".codeium", "windsurf", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":{"post_read_code":[{"command":"existing","show_output":true}]}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	changed, err := hooks.Install("windsurf", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
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
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(got.Hooks["post_cascade_response"]) != 1 || len(got.Hooks["post_read_code"]) != 1 {
		t.Fatalf("Windsurf hooks = %#v", got.Hooks)
	}
	wantCommand := "'/opt/bin/vibe-pushover' notify --agent windsurf --event turn-complete --ignore-errors --config '/tmp/pushover.json'"
	if got.Hooks["post_cascade_response"][0]["command"] != wantCommand || got.Hooks["post_cascade_response"][0]["show_output"] != false {
		t.Fatalf("post_cascade_response hook = %#v", got.Hooks["post_cascade_response"][0])
	}
	changed, err = hooks.Install("windsurf", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed Windsurf hooks")
	}
}

func TestInstallAddsVSCodeAgentStopHook(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".copilot", "hooks", "vibe-pushover.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"hooks":{"userPromptSubmitted":[{"type":"command","bash":"existing"}]}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	changed, err := hooks.Install("vscode", path, "/opt/bin/vibe-pushover", "")
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
		Version int                         `json:"version"`
		Hooks   map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.Version != 1 || len(got.Hooks["agentStop"]) != 1 || len(got.Hooks["userPromptSubmitted"]) != 1 {
		t.Fatalf("VS Code hooks = %#v", got.Hooks)
	}
	stop := got.Hooks["agentStop"][0]
	if stop["type"] != "command" || stop["timeoutSec"] != float64(10) || stop["bash"] != "'/opt/bin/vibe-pushover' notify --agent copilot-vscode --event turn-complete --ignore-errors" {
		t.Fatalf("VS Code Stop hook = %#v", stop)
	}
	changed, err = hooks.Install("vscode", path, "/opt/bin/vibe-pushover", "")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed VS Code hooks")
	}
}

func TestInstallCopilotAndVSCodeShareManifestWithoutDuplicateCompletion(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".copilot", "hooks", "vibe-pushover.json")
	if _, err := hooks.Install("copilot", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install(copilot) error = %v", err)
	}
	changed, err := hooks.Install("vscode", path, "/opt/bin/vibe-pushover", "")
	if err != nil {
		t.Fatalf("Install(vscode) error = %v", err)
	}
	if changed {
		t.Fatal("VS Code install duplicated the shared Copilot manifest")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var got struct {
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(got.Hooks["agentStop"]) != 1 {
		t.Fatalf("shared completion hook count = %d, want 1", len(got.Hooks["agentStop"]))
	}
}

func TestInstallAddsAuggieCompletionHookAndWrapper(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "home with spaces", ".augment", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"model":"fast","hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"/tmp/existing.sh"}]}]}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	changed, err := hooks.Install("auggie", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !changed {
		t.Fatal("Install() changed = false, want true")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(settings) error = %v", err)
	}
	var got struct {
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(got.Hooks["Stop"]) != 1 || len(got.Hooks["SessionStart"]) != 1 || !bytes.Contains(data, []byte(`"model": "fast"`)) {
		t.Fatalf("Auggie settings did not preserve sibling config: %s", data)
	}
	stop := got.Hooks["Stop"][0]
	metadata, _ := stop["metadata"].(map[string]any)
	commands, _ := stop["hooks"].([]any)
	command, _ := commands[0].(map[string]any)
	wrapperPath := filepath.Join(filepath.Dir(path), "hooks", "vibe-pushover.sh")
	if command["type"] != "command" || command["command"] != "'"+wrapperPath+"'" || command["timeout"] != float64(10000) || metadata["includeConversationData"] != true {
		t.Fatalf("Auggie Stop hook = %#v", stop)
	}
	wrapper, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatalf("ReadFile(wrapper) error = %v", err)
	}
	for _, want := range []string{
		"# Generated by vibe-pushover.",
		"'/opt/bin/vibe-pushover' notify --agent auggie --event turn-complete --skip-non-completion --ignore-errors",
		"--config '/tmp/pushover.json'",
	} {
		if !bytes.Contains(wrapper, []byte(want)) {
			t.Fatalf("Auggie wrapper does not contain %q:\n%s", want, wrapper)
		}
	}
	info, err := os.Stat(wrapperPath)
	if err != nil {
		t.Fatalf("Stat(wrapper) error = %v", err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("wrapper mode = %o, want executable", info.Mode().Perm())
	}
	changed, err = hooks.Install("auggie", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed Auggie integration")
	}
}

func TestInstallAuggiePreservesOwnedGroupSiblings(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".augment", "settings.json")
	wrapperPath := filepath.Join(filepath.Dir(path), "hooks", "vibe-pushover.sh")
	original := map[string]any{
		"hooks": map[string]any{
			"Stop": []any{map[string]any{
				"hooks": []any{
					map[string]any{"type": "command", "command": wrapperPath, "timeout": 1, "custom": "keep-command"},
					map[string]any{"type": "command", "command": "/tmp/keep.sh"},
				},
				"metadata": map[string]any{"custom": "keep-metadata"},
				"custom":   "keep-group",
			}},
		},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(wrapperPath), 0o700); err != nil {
		t.Fatalf("MkdirAll(wrapper) error = %v", err)
	}
	if err := os.WriteFile(wrapperPath, []byte("#!/bin/sh\n# Generated by vibe-pushover.\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(wrapper) error = %v", err)
	}

	if _, err := hooks.Install("auggie", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(settings) error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	hookMap := got["hooks"].(map[string]any)
	group := hookMap["Stop"].([]any)[0].(map[string]any)
	commands := group["hooks"].([]any)
	metadata := group["metadata"].(map[string]any)
	owned := commands[0].(map[string]any)
	if len(commands) != 2 || commands[1].(map[string]any)["command"] != "/tmp/keep.sh" {
		t.Fatalf("sibling commands were not preserved: %#v", commands)
	}
	if group["custom"] != "keep-group" || metadata["custom"] != "keep-metadata" || owned["custom"] != "keep-command" {
		t.Fatalf("unknown Auggie fields were not preserved: %#v", group)
	}
	if owned["command"] != "'"+wrapperPath+"'" || owned["timeout"] != float64(10000) || metadata["includeConversationData"] != true {
		t.Fatalf("owned Auggie hook was not updated: %#v", group)
	}
}

func TestInstallAuggieRefusesMarkerOutsideGeneratedPrefix(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".augment", "settings.json")
	wrapperPath := filepath.Join(filepath.Dir(path), "hooks", "vibe-pushover.sh")
	if err := os.MkdirAll(filepath.Dir(wrapperPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	original := []byte("#!/bin/sh\necho '# Generated by vibe-pushover.'\n")
	if err := os.WriteFile(wrapperPath, original, 0o700); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("auggie", path, "/opt/bin/vibe-pushover", ""); err == nil {
		t.Fatal("Install() overwrote a non-owned Auggie wrapper")
	}
	after, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(after, original) {
		t.Fatal("non-owned Auggie wrapper changed")
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

func TestInstallOpenHandsPreservesWrapperHooksAndConfigSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges on Windows")
	}
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "shared-hooks.json")
	path := filepath.Join(dir, "hooks.json")
	original := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"./existing.sh","timeout":60}]}]}}`
	if err := os.WriteFile(target, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	changed, err := hooks.Install("openhands", path, "/opt/bin/vibe-pushover", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !changed {
		t.Fatal("Install() changed = false, want true")
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat() error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("Install() replaced the OpenHands config symlink")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{
		`"command": "./existing.sh"`,
		`"command": "'/opt/bin/vibe-pushover' notify --agent openhands --event turn-complete --ignore-errors --config '/tmp/pushover.json'"`,
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("OpenHands wrapper config does not contain %q:\n%s", want, data)
		}
	}
}

func TestInstallRovoDevHooksIsIdempotentAndPreservesThirdPartyCommands(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".rovodev", "config.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	original := "eventHooks:\n  events:\n    - name: on_complete\n      commands:\n        - command: ./existing-notifier.sh\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if changed, err := hooks.Install("rovo", path, "/opt/bin/vibe-pushover", ""); err != nil || !changed {
		t.Fatalf("first Install() changed=%v error=%v", changed, err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	changed, err := hooks.Install("rovo", path, "/opt/bin/vibe-pushover", "")
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if changed {
		t.Fatal("second Install() changed = true, want false")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(after second install) error = %v", err)
	}
	if !bytes.Equal(after, before) || !bytes.Contains(after, []byte("command: ./existing-notifier.sh")) {
		t.Fatalf("Rovo Dev config changed or lost third-party command:\n%s", after)
	}
}

func TestInstallRovoDevHooksPreservesConfigSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges on Windows")
	}
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "shared-rovo.yml")
	path := filepath.Join(dir, ".rovodev", "config.yml")
	if err := os.WriteFile(target, []byte("agent:\n  modelId: auto\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, err := hooks.Install("rovo", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat() error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("Install() replaced the Rovo Dev config symlink")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(target) error = %v", err)
	}
	if !bytes.Contains(data, []byte("name: on_complete")) {
		t.Fatalf("symlink target was not updated:\n%s", data)
	}
}

func TestInstallRovoDevHooksPreservesThirdPartyCompoundCommand(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".rovodev", "config.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	compound := "prepare && '/opt/bin/vibe-pushover' notify --agent rovo --event turn-complete --ignore-errors --no-input"
	original := "eventHooks:\n  events:\n    - name: on_complete\n      commands:\n        - command: \"" + compound + "\"\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("rovo", path, "/new/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Contains(data, []byte(compound)) {
		t.Fatalf("third-party compound command was overwritten:\n%s", data)
	}
	if !bytes.Contains(data, []byte("'/new/bin/vibe-pushover' notify --agent rovo --event turn-complete")) {
		t.Fatalf("managed Rovo Dev command was not appended:\n%s", data)
	}
}

func TestInstallRovoDevHooksRecognizesQuotedExecutableWithApostrophe(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".rovodev", "config.yml")
	executable := "/opt/Todd's tools/vibe-pushover"
	if changed, err := hooks.Install("rovo", path, executable, ""); err != nil || !changed {
		t.Fatalf("first Install() changed=%v error=%v", changed, err)
	}
	if changed, err := hooks.Install("rovo", path, executable, ""); err != nil || changed {
		t.Fatalf("second Install() changed=%v error=%v", changed, err)
	}
}

func TestInstallCodeBuddyPreservesThirdPartyCompoundCommand(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".codebuddy", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	compound := "'/usr/bin/prepare' && '/old/bin/vibe-pushover' notify --agent codebuddy --event turn-complete --ignore-errors"
	original := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":` + strconv.Quote(compound) + `}]}]}}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("codebuddy", path, "/new/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Contains(data, []byte("'/usr/bin/prepare'")) || !bytes.Contains(data, []byte("'/old/bin/vibe-pushover'")) {
		t.Fatalf("third-party compound command was overwritten:\n%s", data)
	}
	if !bytes.Contains(data, []byte("'/new/bin/vibe-pushover' notify --agent codebuddy --event turn-complete")) {
		t.Fatalf("managed CodeBuddy command was not appended:\n%s", data)
	}
}

func TestInstallZCodeRespectsExplicitlyDisabledHooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".zcode", "cli", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":{"enabled":false,"events":{}}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	changed, err := hooks.Install("zcode", path, "/opt/bin/vibe-pushover", "")
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
	var root struct {
		Hooks struct {
			Enabled bool                        `json:"enabled"`
			Events  map[string][]map[string]any `json:"events"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if root.Hooks.Enabled {
		t.Fatal("Install() enabled explicitly disabled ZCode hooks")
	}
	if len(root.Hooks.Events["Stop"]) != 1 || len(root.Hooks.Events["PermissionRequest"]) != 1 {
		t.Fatalf("ZCode events = %#v", root.Hooks.Events)
	}
}

func TestInstallCodeWhaleRefusesConflictingManagedName(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	original := `[[hooks.hooks]]
name = "vibe-pushover-turn-complete"
event = "turn_end"
command = "./third-party.sh"
`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("codewhale", path, "/opt/bin/vibe-pushover", ""); err == nil {
		t.Fatal("Install() accepted a conflicting CodeWhale hook name")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(after) != original {
		t.Fatalf("refused install changed CodeWhale config:\n%s", after)
	}
}

func TestInstallCodeWhalePreservesConfigSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "codewhale.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(target, []byte("provider = \"deepseek\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	path := filepath.Join(dir, "config.toml")
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, err := hooks.Install("codewhale", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat() error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("CodeWhale config symlink was replaced")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(target) error = %v", err)
	}
	if !bytes.Contains(data, []byte(`event = "turn_end"`)) {
		t.Fatalf("CodeWhale symlink target was not updated:\n%s", data)
	}
}

func TestInstallCodeWhaleIgnoresMarkersInsideMultilineString(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	original := `note = '''
# BEGIN vibe-pushover hook: vibe-pushover-turn-complete
not a managed block
# END vibe-pushover hook: vibe-pushover-turn-complete
'''
`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("codewhale", path, "/opt/bin/vibe-pushover", ""); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		t.Fatalf("Unmarshal() error = %v\n%s", err, data)
	}
	if root["note"] != "# BEGIN vibe-pushover hook: vibe-pushover-turn-complete\nnot a managed block\n# END vibe-pushover hook: vibe-pushover-turn-complete\n" {
		t.Fatalf("multiline string changed: %#v", root["note"])
	}
}

func TestInstallCodeWhaleRefusesDuplicateManagedNameOutsideBlock(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	first := `# BEGIN vibe-pushover hook: vibe-pushover-turn-complete
[[hooks.hooks]]
name = "vibe-pushover-turn-complete"
event = "turn_end"
command = "'/opt/bin/vibe-pushover' notify --agent codewhale --event turn-complete --ignore-errors"
# END vibe-pushover hook: vibe-pushover-turn-complete

[[hooks.hooks]]
name = "vibe-pushover-turn-complete"
event = "turn_end"
command = "./other.sh"
`
	if err := os.WriteFile(path, []byte(first), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := hooks.Install("codewhale", path, "/opt/bin/vibe-pushover", ""); err == nil {
		t.Fatal("Install() accepted a duplicate managed CodeWhale hook name")
	}
}

func TestInstallTabnineHooksIsIdempotent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Tabnine documents POSIX executable hook scripts")
	}
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".tabnine", "agent", "settings.json")
	if changed, err := hooks.Install("tabnine", path, "/opt/bin/vibe-pushover", ""); err != nil || !changed {
		t.Fatalf("first Install() changed=%v error=%v", changed, err)
	}
	if changed, err := hooks.Install("tabnine", path, "/opt/bin/vibe-pushover", ""); err != nil || changed {
		t.Fatalf("second Install() changed=%v error=%v", changed, err)
	}
}

func TestInstallTabnineHooksRefusesUnownedScriptBeforeChangingSettings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Tabnine documents POSIX executable hook scripts")
	}
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".tabnine", "agent", "settings.json")
	hooksDir := filepath.Join(dir, ".tabnine", "hooks")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll(settings) error = %v", err)
	}
	if err := os.MkdirAll(hooksDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(hooks) error = %v", err)
	}
	originalSettings := []byte("{\"hooks\":{\"enabled\":false}}\n")
	if err := os.WriteFile(path, originalSettings, 0o600); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}
	unownedPath := filepath.Join(hooksDir, "after-agent.sh")
	unowned := []byte("#!/bin/sh\necho third-party\n")
	if err := os.WriteFile(unownedPath, unowned, 0o700); err != nil {
		t.Fatalf("WriteFile(unowned hook) error = %v", err)
	}

	changed, err := hooks.Install("tabnine", path, "/opt/bin/vibe-pushover", "")
	if err == nil || changed || !strings.Contains(err.Error(), "not owned") {
		t.Fatalf("Install() changed=%v error=%v, want ownership refusal", changed, err)
	}
	afterSettings, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile(settings) error = %v", readErr)
	}
	afterHook, readErr := os.ReadFile(unownedPath)
	if readErr != nil {
		t.Fatalf("ReadFile(unowned hook) error = %v", readErr)
	}
	if !bytes.Equal(afterSettings, originalSettings) || !bytes.Equal(afterHook, unowned) {
		t.Fatal("refused install changed settings or unowned hook")
	}
	if _, statErr := os.Stat(filepath.Join(hooksDir, "on-error.sh")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("on-error.sh stat error = %v, want not exist", statErr)
	}
}
