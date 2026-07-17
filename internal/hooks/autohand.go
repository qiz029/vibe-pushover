package hooks

import (
	"errors"
	"fmt"
)

type autohandHookSpec struct {
	Name  string
	Event string
}

func installAutohandHooks(path, executable, pushoverConfig string) (bool, error) {
	resolved, err := resolveJSONHookPath(path, "Autohand Code")
	if err != nil {
		return false, err
	}
	root, err := readRoot(resolved)
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
		return false, errors.New("Autohand config field \"hooks\" must be an object")
	}

	changed := false
	for _, spec := range []autohandHookSpec{
		{Name: "on_agent_response", Event: "turn-complete"},
		{Name: "on_permission_request", Event: "approval-required"},
		{Name: "on_error", Event: "attention-required"},
	} {
		command := shellQuote(executable) + " notify --agent autohand --event " + spec.Event + " --ignore-errors --no-input"
		if pushoverConfig != "" {
			command += " --config " + shellQuote(pushoverConfig)
		}
		want := map[string]any{"command": command, "timeout": 10000, "async": true}
		updated, entryChanged, err := upsertAutohandHook(hookMap[spec.Name], spec.Event, want)
		if err != nil {
			return false, fmt.Errorf("update Autohand %s hook: %w", spec.Name, err)
		}
		if entryChanged {
			hookMap[spec.Name] = updated
			changed = true
		}
	}
	if !changed {
		return false, nil
	}
	if err := writeJSON(resolved, root); err != nil {
		return false, err
	}
	return true, nil
}

func upsertAutohandHook(value any, event string, want map[string]any) ([]any, bool, error) {
	var entries []any
	if value != nil {
		var ok bool
		entries, ok = value.([]any)
		if !ok {
			return nil, false, errors.New("hook value must be an array")
		}
	}
	for index, rawEntry := range entries {
		current := ""
		entry, isObject := rawEntry.(map[string]any)
		if isObject {
			current, _ = entry["command"].(string)
		} else {
			current, _ = rawEntry.(string)
		}
		if !isOwnedCommandWithFlag(current, "autohand", event, "--no-input") {
			continue
		}
		if isObject && entry["command"] == want["command"] && entry["async"] == true && fmt.Sprint(entry["timeout"]) == "10000" {
			return entries, false, nil
		}
		if !isObject {
			entry = map[string]any{}
		}
		entry["command"] = want["command"]
		entry["timeout"] = want["timeout"]
		entry["async"] = true
		entries[index] = entry
		return entries, true, nil
	}
	return append(entries, want), true, nil
}
