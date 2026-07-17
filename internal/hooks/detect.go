package hooks

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

var detectionExecutables = map[string][]string{
	"aider":         {"aider"},
	"amp":           {"amp"},
	"autohand":      {"autohand"},
	"auggie":        {"auggie"},
	"claude":        {"claude"},
	"claude-router": {"ccr"},
	"cline":         {"cline"},
	"codebuddy":     {"codebuddy"},
	"codex":         {"codex"},
	"copilot":       {"copilot"},
	"gemini":        {"gemini"},
	"gptme":         {"gptme"},
	"junie":         {"junie"},
	"kimi":          {"kimi"},
	"kiro":          {"kiro-cli"},
	"kilo":          {"kilo"},
	"omp":           {"omp"},
	"openhands":     {"openhands"},
	"opencode":      {"opencode"},
	"pi":            {"pi"},
	"qoder":         {"qodercli"},
	"qwen":          {"qwen"},
	"tabnine":       {"tabnine"},
}

var runDetectionExecutables = map[string][]string{
	"continue":       {"cn"},
	"crush":          {"crush"},
	"gitlab-duo":     {"duo"},
	"mini-swe-agent": {"mini"},
	"plandex":        {"plandex", "pdx"},
}

// DetectedRunAgents returns process-wrapper agents with a known CLI on PATH.
func DetectedRunAgents() []AgentInfo {
	detected := make([]AgentInfo, 0, len(runAgentCatalog))
	for _, agent := range runAgentCatalog {
		for _, executable := range runDetectionExecutables[agent.Name] {
			if _, err := exec.LookPath(executable); err == nil {
				detected = append(detected, agent)
				break
			}
		}
	}
	return detected
}

// DetectedAgents returns supported agents whose local configuration home
// exists or whose curated CLI executable is on PATH. It never creates files
// or starts an agent process.
func DetectedAgents() ([]AgentInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find home directory: %w", err)
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	} else {
		configHome, err = expandHome(configHome)
		if err != nil {
			return nil, fmt.Errorf("resolve XDG config home: %w", err)
		}
	}
	markers := map[string][]string{
		"aider":       {filepath.Join(home, ".aider"), filepath.Join(home, ".aider.conf.yml")},
		"amp":         {filepath.Join(home, ".config", "amp")},
		"antigravity": {filepath.Join(home, ".gemini", "antigravity-cli")},
		"autohand":    {filepath.Join(home, ".autohand")},
		"auggie":      {filepath.Join(home, ".augment")},
		"claude":      {filepath.Join(home, ".claude")},
		"cline":       {filepath.Join(home, "Documents", "Cline"), filepath.Join(home, ".cline")},
		"codebuddy":   {filepath.Join(home, ".codebuddy")},
		"codewhale":   {filepath.Join(home, ".codewhale"), filepath.Join(home, ".deepseek")},
		"codex":       {filepath.Join(home, ".codex")},
		"copilot":     {filepath.Join(home, ".copilot")},
		"cortex":      {filepath.Join(home, ".snowflake", "cortex")},
		"cursor":      {filepath.Join(home, ".cursor")},
		"droid":       {filepath.Join(home, ".factory")},
		"dotcraft":    {filepath.Join(home, ".craft")},
		"gajae":       {filepath.Join(home, ".gjc")},
		"gemini":      {filepath.Join(home, ".gemini", "settings.json")},
		"goose":       {filepath.Join(configHome, "goose"), filepath.Join(home, ".local", "share", "goose")},
		"grok":        {filepath.Join(home, ".grok")},
		"gptme":       {filepath.Join(home, ".config", "gptme")},
		"hermes":      {filepath.Join(home, ".hermes")},
		"junie":       {filepath.Join(home, ".junie")},
		"kimi":        {filepath.Join(home, ".kimi-code")},
		"kiro":        {filepath.Join(home, ".kiro")},
		"kilo":        {filepath.Join(configHome, "kilo")},
		"mimo":        {filepath.Join(configHome, "mimocode")},
		"mistral":     {filepath.Join(home, ".vibe")},
		"omp":         {filepath.Join(home, ".omp")},
		"openhands":   {filepath.Join(home, ".openhands")},
		"opencode":    {filepath.Join(configHome, "opencode")},
		"pi":          {filepath.Join(home, ".pi")},
		"qoder":       {filepath.Join(home, ".qoder")},
		"qwen":        {filepath.Join(home, ".qwen")},
		"rovo":        {filepath.Join(home, ".rovodev")},
		"tabnine":     {filepath.Join(home, ".tabnine")},
		"trae":        {filepath.Join(home, ".trae")},
		"vscode":      vscodeDetectionMarkers(home, configHome),
		"windsurf":    {filepath.Join(home, ".codeium", "windsurf")},
		"workbuddy":   {filepath.Join(home, ".workbuddy")},
		"zcode":       {filepath.Join(home, ".zcode")},
	}
	markers["claude-router"] = []string{filepath.Join(home, ".claude-code-router")}
	sharedPiOverride := os.Getenv("PI_CODING_AGENT_DIR") != ""
	ambiguousPiExecutables := sharedPiOverride && hasAgentExecutable("omp") && hasAgentExecutable("pi")
	detected := make([]AgentInfo, 0, len(markers))
	for _, agent := range agentCatalog {
		sharedPiRuntime := agent.Name == "omp" || agent.Name == "pi"
		if hasAgentExecutable(agent.Name) && !(sharedPiRuntime && ambiguousPiExecutables) {
			detected = append(detected, agent)
			continue
		}
		agentMarkers := append([]string(nil), markers[agent.Name]...)
		resolvedPaths, err := DefaultPaths(agent.Name)
		if err != nil {
			return nil, fmt.Errorf("resolve %s detection paths: %w", agent.DisplayName, err)
		}
		ambiguousPiOverride := sharedPiRuntime && sharedPiOverride
		if ambiguousPiOverride {
			// Pi and Oh My Pi share this override and install incompatible files
			// at the same path, so directory existence cannot identify the runtime.
			agentMarkers = nil
		} else if agent.Name != "claude-router" {
			// Claude Code Router shares Claude's settings target, but that file
			// alone does not prove the separate Router CLI is installed.
			agentMarkers = append(agentMarkers, resolvedPaths...)
		}
		overrideMarkers, err := overrideDetectionMarkers(agent.Name, resolvedPaths)
		if err != nil {
			return nil, fmt.Errorf("resolve %s detection overrides: %w", agent.DisplayName, err)
		}
		agentMarkers = append(agentMarkers, overrideMarkers...)
		seen := make(map[string]struct{}, len(agentMarkers))
		for _, marker := range agentMarkers {
			marker = filepath.Clean(marker)
			if _, duplicate := seen[marker]; duplicate {
				continue
			}
			seen[marker] = struct{}{}
			if _, err := os.Stat(marker); err == nil {
				detected = append(detected, agent)
				break
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("inspect %s installation marker %s: %w", agent.DisplayName, marker, err)
			}
		}
	}
	return detected, nil
}

func hasAgentExecutable(agent string) bool {
	for _, executable := range detectionExecutables[agent] {
		if _, err := exec.LookPath(executable); err == nil {
			return true
		}
	}
	return false
}

func overrideDetectionMarkers(agent string, resolvedPaths []string) ([]string, error) {
	levels := 0
	markers := make([]string, 0, len(resolvedPaths)+1)
	switch agent {
	case "autohand":
		if os.Getenv("AUTOHAND_CONFIG") != "" {
			levels = 1
		}
	case "cline":
		// Cline's Documents directory can be redirected independently of HOME.
		levels = 2
		if clineDir := os.Getenv("CLINE_DIR"); clineDir != "" {
			clineDir, err := expandHome(clineDir)
			if err != nil {
				return nil, err
			}
			markers = append(markers, clineDir)
		}
	case "codewhale":
		if firstEnvironmentValue("CODEWHALE_CONFIG_PATH", "DEEPSEEK_CONFIG_PATH", "CODEWHALE_HOME") != "" {
			levels = 1
		}
	case "copilot", "vscode":
		if os.Getenv("COPILOT_HOME") != "" {
			levels = 2
		}
	case "craft":
		// Craft automations are stored one level below each workspace.
		levels = 1
	case "gemini":
		if os.Getenv("GEMINI_CLI_HOME") != "" {
			levels = 2
		}
	case "grok":
		if os.Getenv("GROK_HOME") != "" {
			levels = 2
		}
	case "hermes":
		if os.Getenv("HERMES_HOME") != "" {
			levels = 1
		}
	case "kimi":
		if os.Getenv("KIMI_CODE_HOME") != "" {
			levels = 1
		}
	case "kilo":
		levels = 2
	case "gajae":
		levels = 1
	case "mimo":
		// MiMo uses LOCALAPPDATA on Windows and XDG_CONFIG_HOME elsewhere.
		levels = 2
		if os.Getenv("MIMOCODE_HOME") != "" {
			levels = 3
		}
	case "mistral":
		if os.Getenv("VIBE_HOME") != "" {
			levels = 1
		}
	}
	if levels == 0 {
		return markers, nil
	}
	for _, path := range resolvedPaths {
		root := path
		for range levels {
			root = filepath.Dir(root)
		}
		markers = append(markers, root)
	}
	return markers, nil
}

func firstEnvironmentValue(names ...string) string {
	for _, name := range names {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}
	return ""
}

func vscodeDetectionMarkers(home, configHome string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{filepath.Join(home, "Library", "Application Support", "Code", "User")}
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return []string{filepath.Join(appData, "Code", "User")}
		}
		return []string{filepath.Join(home, "AppData", "Roaming", "Code", "User")}
	default:
		return []string{filepath.Join(configHome, "Code", "User")}
	}
}
