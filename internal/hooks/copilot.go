package hooks

import (
	"errors"
	"fmt"
)

func installCopilotHooks(path, executable, pushoverConfig string, includeCLINotifications bool) (bool, error) {
	root, err := readRoot(path)
	if err != nil {
		return false, err
	}
	version, hasVersion := root["version"]
	if hasVersion && fmt.Sprint(version) != "1" {
		return false, fmt.Errorf("Copilot hook manifest version must be 1, got %v", version)
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
		return false, errors.New("Copilot hook manifest field \"hooks\" must be an object")
	}

	type copilotHookSpec struct {
		hookName string
		agent    string
		event    string
		matcher  string
	}
	specs := []copilotHookSpec{
		{hookName: "agentStop", agent: "copilot-vscode", event: "turn-complete"},
	}
	if includeCLINotifications {
		specs = append(specs,
			copilotHookSpec{hookName: "notification", agent: "copilot", event: "approval-required", matcher: "permission_prompt"},
			copilotHookSpec{hookName: "notification", agent: "copilot", event: "attention-required", matcher: "elicitation_dialog"},
		)
	}
	for _, spec := range specs {
		command := hookNotifyCommand(executable, spec.agent, spec.event, pushoverConfig)
		entry := map[string]any{
			"type":       "command",
			"bash":       command,
			"timeoutSec": 10,
		}
		if spec.matcher != "" {
			entry["matcher"] = spec.matcher
		}
		updated, entryChanged, err := upsertCopilotHook(hookMap[spec.hookName], spec.agent, spec.event, entry)
		if err != nil {
			return false, fmt.Errorf("update Copilot %s hook: %w", spec.hookName, err)
		}
		if entryChanged {
			hookMap[spec.hookName] = updated
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

func upsertCopilotHook(value any, agent, event string, want map[string]any) ([]any, bool, error) {
	var entries []any
	if value != nil {
		var ok bool
		entries, ok = value.([]any)
		if !ok {
			return nil, false, errors.New("hook value must be an array")
		}
	}
	for i, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		current, _ := entry["bash"].(string)
		if !isOwnedCopilotCommand(current, agent, event) {
			continue
		}
		if entry["type"] == want["type"] && entry["bash"] == want["bash"] && entry["matcher"] == want["matcher"] && fmt.Sprint(entry["timeoutSec"]) == fmt.Sprint(want["timeoutSec"]) {
			return entries, false, nil
		}
		entries[i] = want
		return entries, true, nil
	}
	return append(entries, want), true, nil
}

func isOwnedCopilotCommand(command, agent, event string) bool {
	if isOwnedCommand(command, agent, event) {
		return true
	}
	if agent == "copilot-vscode" && event == "turn-complete" {
		return isOwnedCommand(command, "copilot", event) || isOwnedCommand(command, "vscode", event)
	}
	return false
}

func hookNotifyCommand(executable, agent, event, pushoverConfig string) string {
	command := shellQuote(executable) + " notify --agent " + agent + " --event " + event + " --ignore-errors"
	if pushoverConfig != "" {
		command += " --config " + shellQuote(pushoverConfig)
	}
	return command
}
