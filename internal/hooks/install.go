package hooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var supportedAgents = map[string]struct{}{
	"claude": {},
	"codex":  {},
	"pi":     {},
}

type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
	Async   bool   `json:"async,omitempty"`
}

type hookGroup struct {
	Hooks []hookCommand `json:"hooks"`
}

func DefaultPath(agent string) (string, error) {
	if agent == "pi" {
		if agentDir := os.Getenv("PI_CODING_AGENT_DIR"); agentDir != "" {
			agentDir, err := expandHome(agentDir)
			if err != nil {
				return "", fmt.Errorf("resolve Pi agent directory: %w", err)
			}
			return filepath.Join(agentDir, "extensions", "vibe-pushover", "index.ts"), nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	switch agent {
	case "codex":
		return filepath.Join(home, ".codex", "hooks.json"), nil
	case "claude":
		return filepath.Join(home, ".claude", "settings.json"), nil
	case "pi":
		return filepath.Join(home, ".pi", "agent", "extensions", "vibe-pushover", "index.ts"), nil
	default:
		return "", fmt.Errorf("unsupported agent %q (supported: codex, claude, pi)", agent)
	}
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
	if _, ok := supportedAgents[agent]; !ok {
		return false, fmt.Errorf("unsupported agent %q (supported: codex, claude, pi)", agent)
	}
	if strings.TrimSpace(executable) == "" {
		return false, errors.New("executable path is required")
	}
	if agent == "pi" {
		return installPiExtension(path, executable, pushoverConfig)
	}

	root, err := readRoot(path)
	if err != nil {
		return false, err
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

	changed := false
	for hookName, event := range map[string]string{
		"Stop":              "turn-complete",
		"PermissionRequest": "approval-required",
	} {
		command := shellQuote(executable) + " notify --agent " + agent + " --event " + event + " --ignore-errors"
		if pushoverConfig != "" {
			command += " --config " + shellQuote(pushoverConfig)
		}
		entry := hookGroup{Hooks: []hookCommand{{
			Type:    "command",
			Command: command,
			Timeout: 10,
			Async:   true,
		}}}
		updated, entryChanged, err := upsert(hookMap[hookName], agent, event, entry)
		if err != nil {
			return false, fmt.Errorf("update %s hook: %w", hookName, err)
		}
		if entryChanged {
			hookMap[hookName] = updated
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
			if !isOwnedCommand(current, agent, event) {
				continue
			}
			wanted := want.Hooks[0]
			if hookMatches(command, wanted) {
				return entries, false, nil
			}
			command["type"] = wanted.Type
			command["command"] = wanted.Command
			command["timeout"] = wanted.Timeout
			command["async"] = wanted.Async
			commands[commandIndex] = command
			group["hooks"] = commands
			entries[i] = group
			return entries, true, nil
		}
	}
	data, _ := json.Marshal(want)
	var entry any
	_ = json.Unmarshal(data, &entry)
	return append(entries, entry), true, nil
}

func isOwnedCommand(command, agent, event string) bool {
	const separator = "' notify --agent "
	separatorIndex := strings.LastIndex(command, separator)
	if separatorIndex <= 0 || !strings.HasPrefix(command, "'") {
		return false
	}
	tail := command[separatorIndex+2:]
	base := "notify --agent " + agent + " --event " + event + " --ignore-errors"
	if tail == base {
		return true
	}
	configValue, ok := strings.CutPrefix(tail, base+" --config ")
	return ok && len(configValue) >= 2 && strings.HasPrefix(configValue, "'") && strings.HasSuffix(configValue, "'")
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
