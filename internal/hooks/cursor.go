package hooks

import (
	"errors"
	"fmt"
)

func installCursorHooks(path, executable, pushoverConfig string) (bool, error) {
	root, err := readRoot(path)
	if err != nil {
		return false, err
	}
	version, hasVersion := root["version"]
	if hasVersion && fmt.Sprint(version) != "1" {
		return false, fmt.Errorf("Cursor hook config version must be 1, got %v", version)
	}
	root["version"] = 1
	changed := !hasVersion
	hooksValue, ok := root["hooks"]
	if !ok {
		hooksValue = map[string]any{}
		root["hooks"] = hooksValue
	}
	hookMap, ok := hooksValue.(map[string]any)
	if !ok {
		return false, errors.New("Cursor hook config field \"hooks\" must be an object")
	}
	want := map[string]any{
		"command": hookNotifyCommand(executable, "cursor", "turn-complete", pushoverConfig),
		"timeout": 10,
	}
	updated, hookChanged, err := upsertCursorHook(hookMap["stop"], want)
	if err != nil {
		return false, err
	}
	if hookChanged {
		hookMap["stop"] = updated
		changed = true
	}
	if !changed {
		return false, nil
	}
	if err := writeJSON(path, root); err != nil {
		return false, err
	}
	return true, nil
}

func upsertCursorHook(value any, want map[string]any) ([]any, bool, error) {
	var entries []any
	if value != nil {
		var ok bool
		entries, ok = value.([]any)
		if !ok {
			return nil, false, errors.New("Cursor stop hook value must be an array")
		}
	}
	for i, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		command, _ := entry["command"].(string)
		if !isOwnedCommand(command, "cursor", "turn-complete") {
			continue
		}
		if command == want["command"] && fmt.Sprint(entry["timeout"]) == fmt.Sprint(want["timeout"]) {
			return entries, false, nil
		}
		entries[i] = want
		return entries, true, nil
	}
	return append(entries, want), true, nil
}
