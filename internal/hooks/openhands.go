package hooks

import (
	"errors"
	"fmt"
	"runtime"
)

func installOpenHandsHooks(path, executable, pushoverConfig string) (bool, error) {
	resolvedPath, err := resolveJSONHookPath(path, "OpenHands")
	if err != nil {
		return false, err
	}
	path = resolvedPath
	root, err := readRoot(path)
	if err != nil {
		return false, err
	}

	hookMap := root
	eventName := "stop"
	if hooksValue, ok := root["hooks"]; ok {
		var valid bool
		hookMap, valid = hooksValue.(map[string]any)
		if !valid {
			return false, errors.New("OpenHands config field \"hooks\" must be an object")
		}
		eventName = "Stop"
		if _, ok := hookMap["stop"]; ok {
			eventName = "stop"
		}
	} else if _, ok := root["Stop"]; ok {
		eventName = "Stop"
	}

	command, err := renderOpenHandsCommand(runtime.GOOS, executable, pushoverConfig)
	if err != nil {
		return false, err
	}
	want := hookGroup{Hooks: []hookCommand{{
		Type: "command", Command: command, Timeout: 15,
	}}}
	updated, changed, err := upsert(hookMap[eventName], "openhands", "turn-complete", want)
	if err != nil {
		return false, fmt.Errorf("update OpenHands Stop hook: %w", err)
	}
	if !changed {
		return false, nil
	}
	hookMap[eventName] = updated
	if err := writeJSON(path, root); err != nil {
		return false, err
	}
	return true, nil
}

func renderOpenHandsCommand(goos, executable, pushoverConfig string) (string, error) {
	return hookNotifyCommandForOS(goos, "openhands", "OpenHands", executable, "turn-complete", pushoverConfig)
}
