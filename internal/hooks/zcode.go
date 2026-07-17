package hooks

import (
	"errors"
	"fmt"
	"runtime"
)

func installZCodeHooks(path, executable, pushoverConfig string) (bool, error) {
	path, err := resolveJSONHookPath(path, "ZCode")
	if err != nil {
		return false, err
	}
	root, err := readRoot(path)
	if err != nil {
		return false, err
	}

	changed := false
	hooksValue, ok := root["hooks"]
	if !ok {
		hooksValue = map[string]any{}
		root["hooks"] = hooksValue
		changed = true
	}
	hookConfig, ok := hooksValue.(map[string]any)
	if !ok {
		return false, errors.New("ZCode config field \"hooks\" must be an object")
	}
	if _, ok := hookConfig["enabled"]; !ok {
		hookConfig["enabled"] = true
		changed = true
	}

	eventsValue, ok := hookConfig["events"]
	if !ok {
		eventsValue = map[string]any{}
		hookConfig["events"] = eventsValue
		changed = true
	}
	events, ok := eventsValue.(map[string]any)
	if !ok {
		return false, errors.New("ZCode config field \"hooks.events\" must be an object")
	}

	for _, spec := range genericHookSpecs("zcode") {
		command, err := hookNotifyCommandForOS(runtime.GOOS, "zcode", "ZCode", executable, spec.Event, pushoverConfig)
		if err != nil {
			return false, err
		}
		entry := hookGroup{Matcher: spec.Matcher, Hooks: []hookCommand{{
			Type:    "command",
			Command: command,
			Timeout: spec.Timeout,
			Async:   spec.Async,
		}}}
		updated, entryChanged, err := upsert(events[spec.Name], "zcode", spec.Event, entry)
		if err != nil {
			return false, fmt.Errorf("update ZCode %s hook: %w", spec.Name, err)
		}
		if entryChanged {
			events[spec.Name] = updated
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
