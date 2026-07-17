package hooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type AgentInfo struct {
	Name         string
	DisplayName  string
	Capabilities string
	Resource     string
}

var agentCatalog = []AgentInfo{
	{Name: "aider", DisplayName: "Aider", Capabilities: "completion only (macOS/Linux)", Resource: "config+script"},
	{Name: "amp", DisplayName: "Amp", Capabilities: "completion+approval+attention", Resource: "plugin"},
	{Name: "antigravity", DisplayName: "Antigravity CLI", Capabilities: "completion+attention", Resource: "plugin"},
	{Name: "auggie", DisplayName: "Augment Auggie", Capabilities: "completion only", Resource: "hooks+script"},
	{Name: "claude", DisplayName: "Claude Code", Capabilities: "completion+approval", Resource: "hooks"},
	{Name: "cline", DisplayName: "Cline", Capabilities: "completion only", Resource: "hooks"},
	{Name: "codebuddy", DisplayName: "CodeBuddy Code", Capabilities: "completion+approval+attention", Resource: "hooks (beta)"},
	{Name: "codewhale", DisplayName: "CodeWhale (DeepSeek-TUI)", Capabilities: "completion+attention", Resource: "hooks"},
	{Name: "codex", DisplayName: "Codex CLI", Capabilities: "completion+approval", Resource: "hooks"},
	{Name: "copilot", DisplayName: "GitHub Copilot CLI", Capabilities: "completion+attention", Resource: "hooks"},
	{Name: "cortex", DisplayName: "Snowflake Cortex Code", Capabilities: "completion+approval", Resource: "hooks (preview)"},
	{Name: "cursor", DisplayName: "Cursor", Capabilities: "completion only", Resource: "hooks"},
	{Name: "droid", DisplayName: "Factory Droid", Capabilities: "completion+attention", Resource: "hooks"},
	{Name: "dotcraft", DisplayName: "DotCraft", Capabilities: "completion+approval+stop-hook-attention", Resource: "hooks"},
	{Name: "gajae", DisplayName: "Gajae Code", Capabilities: "completion only", Resource: "config"},
	{Name: "gemini", DisplayName: "Gemini CLI", Capabilities: "completion only", Resource: "hooks"},
	{Name: "goose", DisplayName: "Goose", Capabilities: "completion only", Resource: "plugin"},
	{Name: "grok", DisplayName: "Grok Build", Capabilities: "completion+attention", Resource: "hooks"},
	{Name: "hermes", DisplayName: "Hermes Agent", Capabilities: "completion+approval", Resource: "hooks"},
	{Name: "kimi", DisplayName: "Kimi Code CLI", Capabilities: "completion+approval", Resource: "hooks"},
	{Name: "kiro", DisplayName: "Kiro", Capabilities: "completion only (macOS/Linux)", Resource: "hooks"},
	{Name: "kilo", DisplayName: "Kilo Code", Capabilities: "completion+approval+attention", Resource: "plugin"},
	{Name: "mimo", DisplayName: "MiMo Code", Capabilities: "completion+approval+attention", Resource: "plugin"},
	{Name: "mistral", DisplayName: "Mistral Vibe", Capabilities: "completion only", Resource: "hooks (experimental)"},
	{Name: "omp", DisplayName: "Oh My Pi", Capabilities: "completion+approval", Resource: "extension"},
	{Name: "openhands", DisplayName: "OpenHands CLI", Capabilities: "completion only", Resource: "hooks"},
	{Name: "opencode", DisplayName: "OpenCode", Capabilities: "completion+approval+attention", Resource: "plugin"},
	{Name: "pi", DisplayName: "Pi", Capabilities: "completion only", Resource: "extension"},
	{Name: "qoder", DisplayName: "Qoder", Capabilities: "completion only", Resource: "hooks"},
	{Name: "qwen", DisplayName: "Qwen Code", Capabilities: "completion+approval+attention", Resource: "hooks"},
	{Name: "rovo", DisplayName: "Rovo Dev CLI", Capabilities: "completion+approval+attention", Resource: "event hooks"},
	{Name: "tabnine", DisplayName: "Tabnine CLI", Capabilities: "completion+attention (macOS/Linux)", Resource: "hook scripts"},
	{Name: "trae", DisplayName: "TRAE", Capabilities: "completion+approval", Resource: "hooks"},
	{Name: "vscode", DisplayName: "VS Code Agent", Capabilities: "completion only", Resource: "hooks (preview)"},
	{Name: "windsurf", DisplayName: "Windsurf", Capabilities: "completion only", Resource: "hooks"},
	{Name: "workbuddy", DisplayName: "WorkBuddy", Capabilities: "completion+approval+attention", Resource: "hooks"},
	{Name: "zcode", DisplayName: "ZCode", Capabilities: "completion+approval", Resource: "hooks"},
}

func Agents() []AgentInfo {
	return append([]AgentInfo(nil), agentCatalog...)
}

type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
	Async   bool   `json:"async,omitempty"`
}

type hookGroup struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}

type hookSpec struct {
	Name    string
	Event   string
	Matcher string
	Timeout int
	Async   bool
	Flag    string
}

func DefaultPath(agent string) (string, error) {
	if agent == "cline" {
		paths, err := defaultClinePaths()
		if err != nil {
			return "", err
		}
		return paths[0], nil
	}
	if agent == "copilot" || agent == "vscode" {
		if configHome := os.Getenv("COPILOT_HOME"); configHome != "" {
			configHome, err := expandHome(configHome)
			if err != nil {
				return "", fmt.Errorf("resolve Copilot hook home: %w", err)
			}
			return filepath.Join(configHome, "hooks", "vibe-pushover.json"), nil
		}
	}
	if agent == "gemini" {
		if configHome := os.Getenv("GEMINI_CLI_HOME"); configHome != "" {
			configHome, err := expandHome(configHome)
			if err != nil {
				return "", fmt.Errorf("resolve Gemini CLI home: %w", err)
			}
			return filepath.Join(configHome, ".gemini", "settings.json"), nil
		}
	}
	if agent == "kimi" {
		if dataDir := os.Getenv("KIMI_CODE_HOME"); dataDir != "" {
			dataDir, err := expandHome(dataDir)
			if err != nil {
				return "", fmt.Errorf("resolve Kimi Code home: %w", err)
			}
			return filepath.Join(dataDir, "config.toml"), nil
		}
	}
	if agent == "codewhale" {
		for _, name := range []string{"CODEWHALE_CONFIG_PATH", "DEEPSEEK_CONFIG_PATH"} {
			if configPath := strings.TrimSpace(os.Getenv(name)); configPath != "" {
				configPath, err := expandHome(configPath)
				if err != nil {
					return "", fmt.Errorf("resolve CodeWhale config path: %w", err)
				}
				return configPath, nil
			}
		}
		if codeWhaleHome := strings.TrimSpace(os.Getenv("CODEWHALE_HOME")); codeWhaleHome != "" {
			codeWhaleHome, err := expandHome(codeWhaleHome)
			if err != nil {
				return "", fmt.Errorf("resolve CodeWhale home: %w", err)
			}
			return filepath.Join(codeWhaleHome, "config.toml"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("find home directory: %w", err)
		}
		primary := filepath.Join(home, ".codewhale", "config.toml")
		if _, err := os.Stat(primary); err == nil {
			return primary, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect CodeWhale config: %w", err)
		}
		legacy := filepath.Join(home, ".deepseek", "config.toml")
		if _, err := os.Stat(legacy); err == nil {
			return legacy, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect legacy CodeWhale config: %w", err)
		}
		return primary, nil
	}
	if agent == "mistral" {
		if vibeHome := os.Getenv("VIBE_HOME"); vibeHome != "" {
			vibeHome, err := expandHome(vibeHome)
			if err != nil {
				return "", fmt.Errorf("resolve Mistral Vibe home: %w", err)
			}
			return filepath.Join(vibeHome, "hooks.toml"), nil
		}
	}
	if agent == "grok" {
		if grokHome := os.Getenv("GROK_HOME"); grokHome != "" {
			grokHome, err := expandHome(grokHome)
			if err != nil {
				return "", fmt.Errorf("resolve Grok Build home: %w", err)
			}
			return filepath.Join(grokHome, "hooks", "vibe-pushover.json"), nil
		}
	}
	if agent == "mimo" {
		if mimoHome := os.Getenv("MIMOCODE_HOME"); mimoHome != "" {
			if !filepath.IsAbs(mimoHome) {
				return "", fmt.Errorf("resolve MiMo Code home: MIMOCODE_HOME must be absolute, got %q", mimoHome)
			}
			return filepath.Join(mimoHome, "config", "plugins", "vibe-pushover.ts"), nil
		}
	}
	if agent == "pi" {
		if agentDir := os.Getenv("PI_CODING_AGENT_DIR"); agentDir != "" {
			agentDir, err := expandHome(agentDir)
			if err != nil {
				return "", fmt.Errorf("resolve Pi agent directory: %w", err)
			}
			return filepath.Join(agentDir, "extensions", "vibe-pushover", "index.ts"), nil
		}
	}
	if agent == "omp" {
		if agentDir := os.Getenv("PI_CODING_AGENT_DIR"); agentDir != "" {
			agentDir, err := expandHome(agentDir)
			if err != nil {
				return "", fmt.Errorf("resolve Oh My Pi agent directory: %w", err)
			}
			return filepath.Join(agentDir, "extensions", "vibe-pushover", "index.ts"), nil
		}
	}
	if agent == "hermes" {
		if hermesHome := os.Getenv("HERMES_HOME"); hermesHome != "" {
			hermesHome, err := expandHome(hermesHome)
			if err != nil {
				return "", fmt.Errorf("resolve Hermes home: %w", err)
			}
			return filepath.Join(hermesHome, "config.yaml"), nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	switch agent {
	case "aider":
		return filepath.Join(home, ".aider.conf.yml"), nil
	case "amp":
		return filepath.Join(home, ".config", "amp", "plugins", "vibe-pushover.ts"), nil
	case "antigravity":
		return filepath.Join(home, ".gemini", "antigravity-cli", "plugins", "vibe-pushover"), nil
	case "auggie":
		return filepath.Join(home, ".augment", "settings.json"), nil
	case "codebuddy":
		return filepath.Join(home, ".codebuddy", "settings.json"), nil
	case "codex":
		return filepath.Join(home, ".codex", "hooks.json"), nil
	case "copilot":
		return filepath.Join(home, ".copilot", "hooks", "vibe-pushover.json"), nil
	case "cortex":
		return filepath.Join(home, ".snowflake", "cortex", "hooks.json"), nil
	case "claude":
		return filepath.Join(home, ".claude", "settings.json"), nil
	case "cursor":
		return filepath.Join(home, ".cursor", "hooks.json"), nil
	case "droid":
		return filepath.Join(home, ".factory", "settings.json"), nil
	case "dotcraft":
		return filepath.Join(home, ".craft", "hooks.json"), nil
	case "gajae":
		if agentDir := strings.TrimSpace(os.Getenv("GJC_CODING_AGENT_DIR")); agentDir != "" {
			agentDir, err = expandHome(agentDir)
			if err != nil {
				return "", fmt.Errorf("resolve Gajae Code agent directory: %w", err)
			}
			return filepath.Join(agentDir, "config.yml"), nil
		}
		if configDir := strings.TrimSpace(os.Getenv("GJC_CONFIG_DIR")); configDir != "" {
			return filepath.Join(home, configDir, "agent", "config.yml"), nil
		}
		return filepath.Join(home, ".gjc", "agent", "config.yml"), nil
	case "gemini":
		return filepath.Join(home, ".gemini", "settings.json"), nil
	case "goose":
		return filepath.Join(home, ".agents", "plugins", "vibe-pushover"), nil
	case "grok":
		return filepath.Join(home, ".grok", "hooks", "vibe-pushover.json"), nil
	case "hermes":
		return filepath.Join(home, ".hermes", "config.yaml"), nil
	case "kimi":
		return filepath.Join(home, ".kimi-code", "config.toml"), nil
	case "kiro":
		return filepath.Join(home, ".kiro", "hooks", "vibe-pushover.json"), nil
	case "kilo":
		return kiloPluginPath(runtime.GOOS, home, os.Getenv)
	case "mistral":
		return filepath.Join(home, ".vibe", "hooks.toml"), nil
	case "omp":
		return filepath.Join(home, ".omp", "agent", "extensions", "vibe-pushover", "index.ts"), nil
	case "openhands":
		return filepath.Join(home, ".openhands", "hooks.json"), nil
	case "mimo":
		return mimoPluginPath(runtime.GOOS, home, os.Getenv)
	case "opencode":
		configDir := os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			configDir = filepath.Join(home, ".config")
		} else {
			configDir, err = expandHome(configDir)
			if err != nil {
				return "", fmt.Errorf("resolve XDG config home: %w", err)
			}
		}
		return filepath.Join(configDir, "opencode", "plugins", "vibe-pushover.ts"), nil
	case "pi":
		return filepath.Join(home, ".pi", "agent", "extensions", "vibe-pushover", "index.ts"), nil
	case "qoder":
		return filepath.Join(home, ".qoder", "settings.json"), nil
	case "qwen":
		return filepath.Join(home, ".qwen", "settings.json"), nil
	case "rovo":
		return filepath.Join(home, ".rovodev", "config.yml"), nil
	case "tabnine":
		return filepath.Join(home, ".tabnine", "agent", "settings.json"), nil
	case "trae":
		return filepath.Join(home, ".trae", "hooks.json"), nil
	case "vscode":
		return filepath.Join(home, ".copilot", "hooks", "vibe-pushover.json"), nil
	case "windsurf":
		return filepath.Join(home, ".codeium", "windsurf", "hooks.json"), nil
	case "workbuddy":
		return filepath.Join(home, ".workbuddy", "settings.json"), nil
	case "zcode":
		return filepath.Join(home, ".zcode", "cli", "config.json"), nil
	default:
		return "", unsupportedAgentError(agent)
	}
}

func DefaultPaths(agent string) ([]string, error) {
	if agent == "cline" {
		return defaultClinePaths()
	}
	path, err := DefaultPath(agent)
	if err != nil {
		return nil, err
	}
	return []string{path}, nil
}

func mimoPluginPath(goos, home string, getenv func(string) string) (string, error) {
	configDir := getenv("XDG_CONFIG_HOME")
	if configDir == "" && goos == "windows" {
		configDir = getenv("LOCALAPPDATA")
		if configDir == "" {
			configDir = filepath.Join(home, "AppData", "Local")
		}
	}
	if configDir == "" {
		configDir = filepath.Join(home, ".config")
	} else if configDir == "~" || strings.HasPrefix(configDir, "~/") || strings.HasPrefix(configDir, `~\`) {
		if configDir == "~" {
			configDir = home
		} else {
			configDir = filepath.Join(home, configDir[2:])
		}
	}
	if !isAbsolutePathForOS(goos, configDir) {
		return "", fmt.Errorf("resolve MiMo Code config home: path must be absolute, got %q", configDir)
	}
	return filepath.Join(configDir, "mimocode", "plugins", "vibe-pushover.ts"), nil
}

func isAbsolutePathForOS(goos, path string) bool {
	if goos != "windows" {
		return filepath.IsAbs(path)
	}
	if filepath.IsAbs(path) || strings.HasPrefix(path, `\\`) {
		return true
	}
	return len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/') &&
		((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z'))
}

func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}

func Install(agent, path, executable, pushoverConfig string) (bool, error) {
	if !isSupportedAgent(agent) {
		return false, unsupportedAgentError(agent)
	}
	if strings.TrimSpace(executable) == "" {
		return false, errors.New("executable path is required")
	}
	if agent == "pi" {
		return installPiExtension(path, executable, pushoverConfig)
	}
	if agent == "aider" {
		return installAiderNotifications(path, executable, pushoverConfig)
	}
	if agent == "amp" {
		return installAmpPlugin(path, executable, pushoverConfig)
	}
	if agent == "antigravity" {
		return installAntigravityPlugin(path, executable, pushoverConfig)
	}
	if agent == "auggie" {
		return installAuggieHooks(path, executable, pushoverConfig)
	}
	if agent == "codewhale" {
		return installCodeWhaleHooks(path, executable, pushoverConfig)
	}
	if agent == "copilot" || agent == "vscode" {
		return installCopilotHooks(path, executable, pushoverConfig)
	}
	if agent == "cursor" {
		return installCursorHooks(path, executable, pushoverConfig)
	}
	if agent == "cline" {
		return installClineHook(path, executable, pushoverConfig)
	}
	if agent == "goose" {
		return installGoosePlugin(path, executable, pushoverConfig)
	}
	if agent == "gajae" {
		return installGajaeConfig(path, executable, pushoverConfig)
	}
	if agent == "hermes" {
		return installHermesHooks(path, executable, pushoverConfig)
	}
	if agent == "opencode" {
		return installOpenCodePlugin(path, executable, pushoverConfig, agent, "OpenCode", false)
	}
	if agent == "mimo" {
		return installOpenCodePlugin(path, executable, pushoverConfig, agent, "MiMo Code", false)
	}
	if agent == "kilo" {
		return installOpenCodePlugin(path, executable, pushoverConfig, agent, "Kilo Code", true)
	}
	if agent == "windsurf" {
		return installWindsurfHooks(path, executable, pushoverConfig)
	}
	if agent == "kimi" {
		return installKimiHooks(path, executable, pushoverConfig)
	}
	if agent == "kiro" {
		return installKiroHooks(path, executable, pushoverConfig)
	}
	if agent == "mistral" {
		return installMistralVibeHooks(path, executable, pushoverConfig)
	}
	if agent == "omp" {
		return installOMPExtension(path, executable, pushoverConfig)
	}
	if agent == "openhands" {
		return installOpenHandsHooks(path, executable, pushoverConfig)
	}
	if agent == "rovo" {
		return installRovoHooks(path, executable, pushoverConfig)
	}
	if agent == "tabnine" {
		return installTabnineHooks(path, executable, pushoverConfig)
	}
	if agent == "zcode" {
		return installZCodeHooks(path, executable, pushoverConfig)
	}
	if agent == "codebuddy" || agent == "dotcraft" || agent == "grok" || agent == "trae" || agent == "workbuddy" {
		var err error
		displayName := "CodeBuddy Code"
		if agent == "dotcraft" {
			displayName = "DotCraft"
		} else if agent == "grok" {
			displayName = "Grok Build"
		} else if agent == "trae" {
			displayName = "TRAE"
		} else if agent == "workbuddy" {
			displayName = "WorkBuddy"
		}
		path, err = resolveJSONHookPath(path, displayName)
		if err != nil {
			return false, err
		}
	}

	root, err := readRoot(path)
	if err != nil {
		return false, err
	}
	changed := false
	if agent == "trae" {
		version, hasVersion := root["version"]
		if hasVersion && !isJSONNumericOne(version) {
			return false, fmt.Errorf("TRAE hook config version must be 1, got %v", version)
		}
		if !hasVersion {
			root["version"] = 1
			changed = true
		}
	}
	hooksValue, ok := root["hooks"]
	if !ok {
		hooksValue = map[string]any{}
		root["hooks"] = hooksValue
	}
	hookMap, ok := hooksValue.(map[string]any)
	if !ok {
		return false, errors.New("agent config field \"hooks\" must be an object")
	}

	for _, spec := range genericHookSpecs(agent) {
		command := ""
		if agent == "codebuddy" || agent == "dotcraft" || agent == "grok" || agent == "trae" || agent == "workbuddy" {
			displayName := "CodeBuddy Code"
			if agent == "dotcraft" {
				displayName = "DotCraft"
			} else if agent == "grok" {
				displayName = "Grok Build"
			} else if agent == "trae" {
				displayName = "TRAE"
			} else if agent == "workbuddy" {
				displayName = "WorkBuddy"
			}
			flags := []string(nil)
			if spec.Flag != "" {
				flags = append(flags, spec.Flag)
			}
			command, err = hookNotifyCommandForOSWithFlags(runtime.GOOS, agent, displayName, executable, spec.Event, pushoverConfig, flags...)
			if err != nil {
				return false, err
			}
		} else {
			command = shellQuote(executable) + " notify --agent " + agent + " --event " + spec.Event + " --ignore-errors"
			if spec.Flag != "" {
				command += " " + spec.Flag
			}
			if pushoverConfig != "" {
				command += " --config " + shellQuote(pushoverConfig)
			}
		}
		entry := hookGroup{Matcher: spec.Matcher, Hooks: []hookCommand{{
			Type:    "command",
			Command: command,
			Timeout: spec.Timeout,
			Async:   spec.Async,
		}}}
		updated, entryChanged, err := upsert(hookMap[spec.Name], agent, spec.Event, entry)
		if err != nil {
			return false, fmt.Errorf("update %s hook: %w", spec.Name, err)
		}
		if entryChanged {
			hookMap[spec.Name] = updated
			changed = true
		}
	}
	if !changed {
		return false, nil
	}
	if err := writeJSON(path, root); err != nil {
		return false, err
	}
	return true, nil
}

func InstallAll(agent string, paths []string, executable, pushoverConfig string) ([]bool, error) {
	if agent == "cline" {
		for _, path := range paths {
			resolved, err := resolveClineHookPath(path)
			if err != nil {
				return nil, err
			}
			if err := validateClineHookOwnership(resolved); err != nil {
				return nil, err
			}
		}
	}
	changed := make([]bool, len(paths))
	for index, path := range paths {
		pathChanged, err := Install(agent, path, executable, pushoverConfig)
		if err != nil {
			return nil, err
		}
		changed[index] = pathChanged
	}
	return changed, nil
}

func isJSONNumericOne(value any) bool {
	number, ok := value.(json.Number)
	if !ok {
		return false
	}
	parsed, err := number.Float64()
	return err == nil && parsed == 1
}

func resolveJSONHookPath(path, agent string) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect %s hook file: %w", agent, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s hook symlink: %w", agent, err)
	}
	return resolved, nil
}

func isSupportedAgent(name string) bool {
	for _, agent := range agentCatalog {
		if agent.Name == name {
			return true
		}
	}
	return false
}

func unsupportedAgentError(name string) error {
	names := make([]string, 0, len(agentCatalog))
	for _, agent := range agentCatalog {
		names = append(names, agent.Name)
	}
	return fmt.Errorf("unsupported agent %q (supported: %s)", name, strings.Join(names, ", "))
}

func genericHookSpecs(agent string) []hookSpec {
	switch agent {
	case "codebuddy", "workbuddy":
		return []hookSpec{
			{Name: "Stop", Event: "turn-complete", Timeout: 10, Flag: "--skip-active-stop"},
			{Name: "StopFailure", Event: "attention-required", Timeout: 10},
			{Name: "PermissionRequest", Event: "approval-required", Timeout: 10},
		}
	case "dotcraft":
		return []hookSpec{
			{Name: "Stop", Event: "turn-complete", Timeout: 10, Async: true, Flag: "--skip-active-stop"},
			{Name: "StopFailure", Event: "attention-required", Timeout: 10, Async: true},
			{Name: "PermissionRequest", Event: "approval-required", Timeout: 10, Async: true},
		}
	case "gemini":
		return []hookSpec{{Name: "AfterAgent", Event: "turn-complete", Timeout: 10000}}
	case "droid":
		return []hookSpec{
			{Name: "Stop", Event: "turn-complete", Timeout: 10},
			{Name: "Notification", Event: "attention-required", Timeout: 10},
		}
	case "qwen":
		return []hookSpec{
			{Name: "Stop", Event: "turn-complete", Timeout: 10000, Async: true, Flag: "--skip-active-qwen-stop"},
			{Name: "PermissionRequest", Event: "approval-required", Timeout: 10000, Async: true},
			{Name: "Notification", Event: "attention-required", Matcher: "idle_prompt", Timeout: 10000, Async: true},
		}
	case "qoder":
		return []hookSpec{{Name: "Stop", Event: "turn-complete", Timeout: 10, Flag: "--skip-active-stop"}}
	case "cortex":
		return []hookSpec{
			{Name: "Stop", Event: "turn-complete", Timeout: 10},
			{Name: "PermissionRequest", Event: "approval-required", Timeout: 10},
		}
	case "grok":
		return []hookSpec{
			{Name: "Stop", Event: "turn-complete", Timeout: 10},
			{Name: "StopFailure", Event: "attention-required", Timeout: 10},
		}
	case "trae":
		return []hookSpec{
			{Name: "Stop", Event: "turn-complete", Timeout: 10},
			{Name: "Notification", Event: "approval-required", Matcher: "permission_prompt", Timeout: 10},
		}
	default:
		return []hookSpec{
			{Name: "Stop", Event: "turn-complete", Timeout: 10, Async: true},
			{Name: "PermissionRequest", Event: "approval-required", Timeout: 10, Async: true},
		}
	}
}

func readRoot(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read agent config: %w", err)
	}
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("parse agent config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("parse agent config: multiple top-level values")
		}
		return nil, fmt.Errorf("parse agent config: %w", err)
	}
	if root == nil {
		return nil, errors.New("parse agent config: top-level value must be an object")
	}
	return root, nil
}

func upsert(value any, agent, event string, want hookGroup) ([]any, bool, error) {
	var entries []any
	if value != nil {
		var ok bool
		entries, ok = value.([]any)
		if !ok {
			return nil, false, errors.New("hook value must be an array")
		}
	}
	for i, raw := range entries {
		group, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		commands, ok := group["hooks"].([]any)
		if !ok {
			continue
		}
		for commandIndex, rawCommand := range commands {
			command, ok := rawCommand.(map[string]any)
			if !ok {
				continue
			}
			current, _ := command["command"].(string)
			owned := isOwnedCommand(current, agent, event) ||
				isOwnedCommandWithFlag(current, agent, event, "--skip-active-stop")
			if agent == "qwen" && event == "turn-complete" {
				owned = owned || isOwnedCommandWithFlag(current, agent, event, "--skip-active-qwen-stop")
			}
			if !owned {
				continue
			}
			wanted := want.Hooks[0]
			matcher, _ := group["matcher"].(string)
			if hookMatches(command, wanted) && matcher == want.Matcher {
				return entries, false, nil
			}
			if matcher != want.Matcher && len(commands) > 1 {
				remaining := make([]any, 0, len(commands)-1)
				remaining = append(remaining, commands[:commandIndex]...)
				remaining = append(remaining, commands[commandIndex+1:]...)
				group["hooks"] = remaining
				entries[i] = group
				return appendHookGroup(entries, want), true, nil
			}
			command["type"] = wanted.Type
			command["command"] = wanted.Command
			command["timeout"] = wanted.Timeout
			if wanted.Async {
				command["async"] = true
			} else {
				delete(command, "async")
			}
			if want.Matcher != "" {
				group["matcher"] = want.Matcher
			} else {
				delete(group, "matcher")
			}
			commands[commandIndex] = command
			group["hooks"] = commands
			entries[i] = group
			return entries, true, nil
		}
	}
	return appendHookGroup(entries, want), true, nil
}

func appendHookGroup(entries []any, want hookGroup) []any {
	data, _ := json.Marshal(want)
	var entry any
	_ = json.Unmarshal(data, &entry)
	return append(entries, entry)
}

func isOwnedCommand(command, agent, event string) bool {
	return isOwnedCommandWithFlag(command, agent, event, "")
}

func isOwnedCommandWithFlag(command, agent, event, flag string) bool {
	if len(command) < 2 || (command[0] != '\'' && command[0] != '"') {
		return false
	}
	quote := command[0]
	separator := string(quote) + " notify --agent "
	separatorIndex := strings.LastIndex(command, separator)
	if separatorIndex <= 0 || !isCanonicalQuotedArgument(command[:separatorIndex+1], quote) {
		return false
	}
	tail := command[separatorIndex+2:]
	base := "notify --agent " + agent + " --event " + event + " --ignore-errors"
	if flag != "" {
		base += " " + flag
	}
	if tail == base {
		return true
	}
	configValue, ok := strings.CutPrefix(tail, base+" --config ")
	return ok && isCanonicalQuotedArgument(configValue, quote)
}

func hookMatches(got map[string]any, want hookCommand) bool {
	gotType, _ := got["type"].(string)
	gotCommand, _ := got["command"].(string)
	gotAsync, _ := got["async"].(bool)
	return gotType == want.Type && gotCommand == want.Command && gotAsync == want.Async && fmt.Sprint(got["timeout"]) == fmt.Sprint(want.Timeout)
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create agent config directory: %w", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode agent config: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".hooks-*")
	if err != nil {
		return fmt.Errorf("create temporary agent config: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("set agent config permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write agent config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close agent config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace agent config: %w", err)
	}
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
