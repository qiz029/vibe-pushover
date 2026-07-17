package hooks

import (
	"errors"
	"fmt"
	"path/filepath"
)

func installGoosePlugin(path, executable, pushoverConfig string) (bool, error) {
	manifestPath := filepath.Join(path, "plugin.json")
	manifest, manifestChanged, err := prepareGooseManifest(manifestPath)
	if err != nil {
		return false, err
	}

	hooksPath := filepath.Join(path, "hooks", "hooks.json")
	root, err := readRoot(hooksPath)
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
		return false, errors.New("Goose hook config field \"hooks\" must be an object")
	}
	entry := hookGroup{Hooks: []hookCommand{{
		Type:    "command",
		Command: hookNotifyCommand(executable, "goose", "turn-complete", pushoverConfig),
		Timeout: 10,
	}}}
	updated, hooksChanged, err := upsert(hookMap["Stop"], "goose", "turn-complete", entry)
	if err != nil {
		return false, fmt.Errorf("update Goose Stop hook: %w", err)
	}
	if hooksChanged {
		hookMap["Stop"] = updated
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

func prepareGooseManifest(path string) (map[string]any, bool, error) {
	root, err := readRoot(path)
	if err != nil {
		return nil, false, err
	}
	if len(root) > 0 {
		name, _ := root["name"].(string)
		if name != "vibe-pushover" {
			return nil, false, fmt.Errorf("refusing to overwrite Goose plugin not owned by vibe-pushover: %s", path)
		}
	}
	want := map[string]any{
		"name":        "vibe-pushover",
		"version":     "1.0.0",
		"description": "Pushover notifications for Goose coding sessions",
	}
	if root["name"] == want["name"] && root["version"] == want["version"] && root["description"] == want["description"] {
		return root, false, nil
	}
	return want, true, nil
}
