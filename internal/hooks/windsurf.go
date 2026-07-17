package hooks

import "errors"

func installWindsurfHooks(path, executable, pushoverConfig string) (bool, error) {
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
		return false, errors.New("Windsurf hook config field \"hooks\" must be an object")
	}
	want := map[string]any{
		"command":     hookNotifyCommand(executable, "windsurf", "turn-complete", pushoverConfig),
		"show_output": false,
	}
	updated, changed, err := upsertWindsurfHook(hookMap["post_cascade_response"], want)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	hookMap["post_cascade_response"] = updated
	if err := writeJSON(path, root); err != nil {
		return false, err
	}
	return true, nil
}

func upsertWindsurfHook(value any, want map[string]any) ([]any, bool, error) {
	var entries []any
	if value != nil {
		var ok bool
		entries, ok = value.([]any)
		if !ok {
			return nil, false, errors.New("Windsurf post_cascade_response hook value must be an array")
		}
	}
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		command, _ := entry["command"].(string)
		if !isOwnedCommand(command, "windsurf", "turn-complete") {
			continue
		}
		showOutput, _ := entry["show_output"].(bool)
		if command == want["command"] && !showOutput {
			return entries, false, nil
		}
		entry["command"] = want["command"]
		entry["show_output"] = false
		return entries, true, nil
	}
	return append(entries, want), true, nil
}
