package hooks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type craftAutomationSpec struct {
	Name    string
	Event   string
	Matcher string
	Flag    string
}

func defaultCraftPaths() ([]string, error) {
	configDir := strings.TrimSpace(os.Getenv("CRAFT_CONFIG_DIR"))
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("find home directory: %w", err)
		}
		configDir = filepath.Join(home, ".craft-agent")
	} else {
		var err error
		configDir, err = expandHome(configDir)
		if err != nil {
			return nil, fmt.Errorf("resolve Craft Agents config directory: %w", err)
		}
		if !filepath.IsAbs(configDir) {
			return nil, fmt.Errorf("resolve Craft Agents config directory: CRAFT_CONFIG_DIR must be absolute, got %q", configDir)
		}
	}
	workspaceRoot := filepath.Join(configDir, "workspaces")
	entries, err := os.ReadDir(workspaceRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list Craft Agents workspaces: %w", err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		workspacePath := filepath.Join(workspaceRoot, entry.Name())
		info, err := os.Stat(workspacePath)
		if err != nil {
			return nil, fmt.Errorf("inspect Craft Agents workspace %s: %w", workspacePath, err)
		}
		if info.IsDir() {
			paths = append(paths, filepath.Join(workspacePath, "automations.json"))
		}
	}
	return paths, nil
}

func installCraftAutomations(path, executable, pushoverConfig string) (bool, error) {
	resolved, err := resolveJSONHookPath(path, "Craft Agents")
	if err != nil {
		return false, err
	}
	root, err := readRoot(resolved)
	if err != nil {
		return false, err
	}
	changed := false
	if version, ok := root["version"]; ok {
		if !isJSONNumeric(version, 2) {
			return false, fmt.Errorf("Craft Agents automation config version must be 2, got %v", version)
		}
	} else {
		root["version"] = 2
		changed = true
	}
	automationsValue, ok := root["automations"]
	if !ok {
		automationsValue = map[string]any{}
		root["automations"] = automationsValue
		changed = true
	}
	automations, ok := automationsValue.(map[string]any)
	if !ok {
		return false, errors.New("Craft Agents config field \"automations\" must be an object")
	}
	for _, spec := range []craftAutomationSpec{
		{Name: "Stop", Event: "turn-complete", Flag: "--skip-active-stop"},
		{Name: "Notification", Event: "approval-required", Matcher: "permission_prompt"},
		{Name: "Notification", Event: "attention-required", Matcher: "idle_prompt"},
	} {
		command := shellQuote(executable) + " notify --agent craft --event " + spec.Event + " --ignore-errors --payload-env CRAFT_EVENT_DATA"
		if spec.Flag != "" {
			command += " " + spec.Flag
		}
		if pushoverConfig != "" {
			command += " --config " + shellQuote(pushoverConfig)
		}
		updated, entryChanged, err := upsertCraftAutomation(automations[spec.Name], spec, command)
		if err != nil {
			return false, fmt.Errorf("update Craft Agents %s automation: %w", spec.Name, err)
		}
		if entryChanged {
			automations[spec.Name] = updated
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

func upsertCraftAutomation(value any, spec craftAutomationSpec, command string) ([]any, bool, error) {
	var entries []any
	if value != nil {
		var ok bool
		entries, ok = value.([]any)
		if !ok {
			return nil, false, errors.New("automation value must be an array")
		}
	}
	changed := false
	for entryIndex, rawEntry := range entries {
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			continue
		}
		actions, ok := entry["actions"].([]any)
		if !ok {
			continue
		}
		var ownedAction map[string]any
		otherActions := make([]any, 0, len(actions))
		for _, rawAction := range actions {
			action, ok := rawAction.(map[string]any)
			if !ok || !isOwnedCraftCommand(fmt.Sprint(action["command"]), spec.Event) {
				otherActions = append(otherActions, rawAction)
				continue
			}
			if ownedAction == nil {
				ownedAction = action
			}
		}
		if ownedAction == nil {
			continue
		}
		if len(otherActions) > 0 {
			// permissionMode applies to every action in a matcher group. Move the
			// notifier out before granting it allow-all so sibling commands keep
			// their existing security policy.
			entry["actions"] = otherActions
			entries[entryIndex] = entry
			changed = true
			continue
		}

		matcher, _ := entry["matcher"].(string)
		permissionMode, _ := entry["permissionMode"].(string)
		matches := len(actions) == 1 && matcher == spec.Matcher && permissionMode == "allow-all" &&
			ownedAction["type"] == "command" && ownedAction["command"] == command && fmt.Sprint(ownedAction["timeout"]) == "10000"
		if matches {
			return entries, changed, nil
		}
		if spec.Matcher == "" {
			delete(entry, "matcher")
		} else {
			entry["matcher"] = spec.Matcher
		}
		entry["permissionMode"] = "allow-all"
		ownedAction["type"] = "command"
		ownedAction["command"] = command
		ownedAction["timeout"] = 10000
		entry["actions"] = []any{ownedAction}
		entries[entryIndex] = entry
		return entries, true, nil
	}
	entry := map[string]any{
		"permissionMode": "allow-all",
		"actions": []any{map[string]any{
			"type": "command", "command": command, "timeout": 10000,
		}},
	}
	if spec.Matcher != "" {
		entry["matcher"] = spec.Matcher
	}
	return append(entries, entry), true, nil
}

func isOwnedCraftCommand(command, event string) bool {
	if len(command) < 2 || (command[0] != '\'' && command[0] != '"') {
		return false
	}
	quote := command[0]
	separator := string(quote) + " notify --agent craft --event "
	separatorIndex := strings.LastIndex(command, separator)
	if separatorIndex <= 0 || !isCanonicalQuotedArgument(command[:separatorIndex+1], quote) {
		return false
	}
	tail := command[separatorIndex+2:]
	base := "notify --agent craft --event " + event + " --ignore-errors --payload-env CRAFT_EVENT_DATA"
	rest, ok := strings.CutPrefix(tail, base)
	if !ok {
		return false
	}
	if strings.HasPrefix(rest, " --skip-active-stop") {
		rest = strings.TrimPrefix(rest, " --skip-active-stop")
	}
	if rest == "" {
		return true
	}
	configValue, ok := strings.CutPrefix(rest, " --config ")
	return ok && isCanonicalQuotedArgument(configValue, quote)
}
