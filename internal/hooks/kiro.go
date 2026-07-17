package hooks

import (
	"errors"
	"fmt"
	"runtime"
)

const kiroHookName = "vibe-pushover-turn-complete"

func installKiroHooks(path, executable, pushoverConfig string) (bool, error) {
	if runtime.GOOS == "windows" {
		return false, errors.New("Kiro hook integration currently requires macOS or Linux")
	}
	root, err := readRoot(path)
	if err != nil {
		return false, err
	}
	version, hasVersion := root["version"]
	if hasVersion && version != "v1" {
		return false, fmt.Errorf("Kiro hook config version must be v1, got %v", version)
	}
	root["version"] = "v1"
	changed := !hasVersion

	hooksValue, ok := root["hooks"]
	if !ok {
		hooksValue = []any{}
		root["hooks"] = hooksValue
	}
	entries, ok := hooksValue.([]any)
	if !ok {
		return false, errors.New("Kiro hook config field \"hooks\" must be an array")
	}

	wantAction := map[string]any{
		"type":    "command",
		"command": hookNotifyCommand(executable, "kiro", "turn-complete", pushoverConfig),
	}
	found := false
	for index, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok || entry["name"] != kiroHookName {
			continue
		}
		found = true
		action, _ := entry["action"].(map[string]any)
		if entry["trigger"] == "Stop" && fmt.Sprint(entry["timeout"]) == "10" && entry["enabled"] == true && action["type"] == "command" && action["command"] == wantAction["command"] {
			break
		}
		entry["trigger"] = "Stop"
		entry["action"] = wantAction
		entry["timeout"] = 10
		entry["enabled"] = true
		entries[index] = entry
		changed = true
		break
	}
	if !found {
		entries = append(entries, map[string]any{
			"name":        kiroHookName,
			"description": "Send a Pushover notification when Kiro completes a turn",
			"trigger":     "Stop",
			"action":      wantAction,
			"timeout":     10,
			"enabled":     true,
		})
		changed = true
	}
	if !changed {
		return false, nil
	}
	root["hooks"] = entries
	if err := writeJSON(path, root); err != nil {
		return false, err
	}
	return true, nil
}
