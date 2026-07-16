package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

var supportedAgents = map[string]struct{}{
	"claude": {},
	"codex":  {},
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
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	switch agent {
	case "codex":
		return filepath.Join(home, ".codex", "hooks.json"), nil
	case "claude":
		return filepath.Join(home, ".claude", "settings.json"), nil
	default:
		return "", fmt.Errorf("unsupported agent %q (supported: codex, claude)", agent)
	}
}

func Install(agent, path, executable string) (bool, error) {
	if _, ok := supportedAgents[agent]; !ok {
		return false, fmt.Errorf("unsupported agent %q (supported: codex, claude)", agent)
	}
	if strings.TrimSpace(executable) == "" {
		return false, errors.New("executable path is required")
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
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse agent config: %w", err)
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
	marker := "notify --agent " + agent + " --event " + event
	for i, raw := range entries {
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, false, err
		}
		if strings.Contains(string(data), marker) {
			wantData, _ := json.Marshal(want)
			var replacement any
			_ = json.Unmarshal(wantData, &replacement)
			if reflect.DeepEqual(raw, replacement) {
				return entries, false, nil
			}
			entries[i] = replacement
			return entries, true, nil
		}
	}
	data, _ := json.Marshal(want)
	var entry any
	_ = json.Unmarshal(data, &entry)
	return append(entries, entry), true, nil
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
