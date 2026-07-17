package hooks

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
)

const antigravityHookName = "vibe-pushover"

func installAntigravityPlugin(path, executable, pushoverConfig string) (bool, error) {
	manifestPath := filepath.Join(path, "plugin.json")
	manifest, manifestChanged, err := prepareAntigravityManifest(manifestPath)
	if err != nil {
		return false, err
	}

	hooksPath := filepath.Join(path, "hooks.json")
	root, err := readRoot(hooksPath)
	if err != nil {
		return false, err
	}
	hookDefinition, ok := root[antigravityHookName].(map[string]any)
	if root[antigravityHookName] != nil && !ok {
		return false, errors.New("Antigravity vibe-pushover hook definition must be an object")
	}
	if !ok {
		hookDefinition = map[string]any{}
		root[antigravityHookName] = hookDefinition
	}

	completionCommand, err := hookNotifyCommandForOSWithFlags(runtime.GOOS, "antigravity", "Antigravity CLI", executable, "turn-complete", pushoverConfig, "--skip-antigravity-noncompletion")
	if err != nil {
		return false, err
	}
	attentionCommand, err := hookNotifyCommandForOSWithFlags(runtime.GOOS, "antigravity", "Antigravity CLI", executable, "attention-required", pushoverConfig, "--only-antigravity-failure")
	if err != nil {
		return false, err
	}
	stops, completionChanged, err := upsertAntigravityStop(hookDefinition["Stop"], "turn-complete", "--skip-antigravity-noncompletion", completionCommand)
	if err != nil {
		return false, err
	}
	stops, attentionChanged, err := upsertAntigravityStop(stops, "attention-required", "--only-antigravity-failure", attentionCommand)
	if err != nil {
		return false, err
	}
	hooksChanged := completionChanged || attentionChanged
	if hooksChanged {
		hookDefinition["Stop"] = stops
		if err := writeJSON(hooksPath, root); err != nil {
			return false, err
		}
	}
	if manifestChanged {
		if err := writeJSON(manifestPath, manifest); err != nil {
			return false, err
		}
	}
	return manifestChanged || hooksChanged, nil
}

func prepareAntigravityManifest(path string) (map[string]any, bool, error) {
	root, err := readRoot(path)
	if err != nil {
		return nil, false, err
	}
	if len(root) > 0 {
		name, _ := root["name"].(string)
		if name != antigravityHookName {
			return nil, false, fmt.Errorf("refusing to overwrite Antigravity plugin not owned by vibe-pushover: %s", path)
		}
	}
	if root["name"] == antigravityHookName {
		return root, false, nil
	}
	root["name"] = antigravityHookName
	return root, true, nil
}

func upsertAntigravityStop(value any, event, flag, wantCommand string) ([]any, bool, error) {
	var stops []any
	if value != nil {
		var ok bool
		stops, ok = value.([]any)
		if !ok {
			return nil, false, errors.New("Antigravity Stop hook must be an array")
		}
	}
	for index, raw := range stops {
		handler, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		current, _ := handler["command"].(string)
		owned := isOwnedCommandWithFlag(current, "antigravity", event, flag)
		if event == "turn-complete" {
			owned = owned || isOwnedCommand(current, "antigravity", event)
		}
		if !owned {
			continue
		}
		if handler["type"] == "command" && current == wantCommand && fmt.Sprint(handler["timeout"]) == "10" {
			return stops, false, nil
		}
		handler["type"] = "command"
		handler["command"] = wantCommand
		handler["timeout"] = 10
		stops[index] = handler
		return stops, true, nil
	}
	return append(stops, map[string]any{
		"type": "command", "command": wantCommand, "timeout": 10,
	}), true, nil
}
