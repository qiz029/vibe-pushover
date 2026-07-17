package hooks

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

type rovoHookSpec struct {
	name  string
	event string
}

func installRovoHooks(path, executable, pushoverConfig string) (bool, error) {
	resolvedPath, err := resolveJSONHookPath(path, "Rovo Dev")
	if err != nil {
		return false, err
	}
	path = resolvedPath
	document, err := readRovoConfig(path)
	if err != nil {
		return false, err
	}
	root := document.Content[0]
	eventHooks, changed, err := rovoMapping(root, "eventHooks", "Rovo Dev config")
	if err != nil {
		return false, err
	}
	events, eventsCreated, err := rovoSequence(eventHooks, "events", "Rovo Dev eventHooks")
	if err != nil {
		return false, err
	}
	changed = changed || eventsCreated

	for _, spec := range []rovoHookSpec{
		{name: "on_complete", event: "turn-complete"},
		{name: "on_error", event: "attention-required"},
		{name: "on_tool_permission", event: "approval-required"},
	} {
		command, err := hookNotifyCommandForOSWithFlags(runtime.GOOS, "rovo", "Rovo Dev", executable, spec.event, pushoverConfig, "--no-input")
		if err != nil {
			return false, err
		}
		eventChanged, err := upsertRovoEvent(events, spec, command)
		if err != nil {
			return false, err
		}
		changed = changed || eventChanged
	}
	if !changed {
		return false, nil
	}
	if err := writeRovoConfig(path, document); err != nil {
		return false, err
	}
	return true, nil
}

func readRovoConfig(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && len(bytes.TrimSpace(data)) == 0) {
		return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Rovo Dev config: %w", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse Rovo Dev config: %w", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("Rovo Dev config top-level value must be a mapping")
	}
	return &document, nil
}

func rovoMapping(parent *yaml.Node, key, context string) (*yaml.Node, bool, error) {
	return rovoChild(parent, key, yaml.MappingNode, "!!map", context)
}

func rovoSequence(parent *yaml.Node, key, context string) (*yaml.Node, bool, error) {
	return rovoChild(parent, key, yaml.SequenceNode, "!!seq", context)
}

func rovoChild(parent *yaml.Node, key string, kind yaml.Kind, tag, context string) (*yaml.Node, bool, error) {
	found := -1
	for index := 0; index+1 < len(parent.Content); index += 2 {
		if parent.Content[index].Value != key {
			continue
		}
		if found >= 0 {
			return nil, false, fmt.Errorf("%s contains duplicate %q keys", context, key)
		}
		found = index + 1
	}
	if found >= 0 {
		if parent.Content[found].Kind != kind {
			return nil, false, fmt.Errorf("%s field %q has the wrong type", context, key)
		}
		return parent.Content[found], false, nil
	}
	want := &yaml.Node{Kind: kind, Tag: tag}
	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, want,
	)
	return want, true, nil
}

func upsertRovoEvent(events *yaml.Node, spec rovoHookSpec, command string) (bool, error) {
	var found *yaml.Node
	eventCreated := false
	for _, entry := range events.Content {
		if entry.Kind != yaml.MappingNode {
			return false, errors.New("Rovo Dev eventHooks.events entries must be mappings")
		}
		name, ok, err := yamlScalarValueFor(entry, "name", "Rovo Dev event")
		if err != nil {
			return false, err
		}
		if ok && name == spec.name {
			if found != nil {
				return false, fmt.Errorf("Rovo Dev config contains duplicate %q events", spec.name)
			}
			found = entry
		}
	}
	if found == nil {
		eventCreated = true
		found = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "name"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: spec.name},
		}}
		events.Content = append(events.Content, found)
	}
	commands, created, err := rovoSequence(found, "commands", "Rovo Dev event")
	if err != nil {
		return false, err
	}
	commandChanged, err := upsertRovoCommand(commands, spec.event, command)
	return eventCreated || created || commandChanged, err
}

func upsertRovoCommand(commands *yaml.Node, event, want string) (bool, error) {
	for _, entry := range commands.Content {
		if entry.Kind != yaml.MappingNode {
			return false, errors.New("Rovo Dev event commands must be mappings")
		}
		current, ok, err := yamlScalarValueFor(entry, "command", "Rovo Dev event command")
		if err != nil {
			return false, err
		}
		if !ok || !isOwnedRovoCommand(current, event) {
			continue
		}
		return upsertYAMLScalarFor(entry, "command", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: want, Style: yaml.DoubleQuotedStyle}, "Rovo Dev event command")
	}
	commands.Content = append(commands.Content, &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "command"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: want, Style: yaml.DoubleQuotedStyle},
	}})
	return true, nil
}

func isOwnedRovoCommand(command, event string) bool {
	for _, quote := range []byte{'\'', '"'} {
		separator := string(quote) + " notify --agent "
		index := bytes.LastIndex([]byte(command), []byte(separator))
		if index <= 0 || !isCanonicalQuotedArgument(command[:index+1], quote) {
			continue
		}
		tail := command[index+2:]
		for _, suffix := range []string{"", " --no-input"} {
			base := "notify --agent rovo --event " + event + " --ignore-errors" + suffix
			if tail == base {
				return true
			}
			config, ok := strings.CutPrefix(tail, base+" --config ")
			if ok && isCanonicalQuotedArgument(config, quote) {
				return true
			}
		}
	}
	return false
}

func isCanonicalQuotedArgument(value string, quote byte) bool {
	if len(value) < 2 || value[0] != quote || value[len(value)-1] != quote {
		return false
	}
	inner := value[1 : len(value)-1]
	if quote == '\'' {
		decoded := strings.ReplaceAll(inner, `'"'"'`, "'")
		return shellQuote(decoded) == value
	}
	if strings.Contains(inner, `"`) {
		return false
	}
	quoted, err := windowsShellQuote(inner)
	return err == nil && quoted == value
}

func writeRovoConfig(path string, document *yaml.Node) error {
	data, err := yaml.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode Rovo Dev config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create Rovo Dev config directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".rovo-config-*")
	if err != nil {
		return fmt.Errorf("create temporary Rovo Dev config: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("set Rovo Dev config permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write Rovo Dev config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close Rovo Dev config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace Rovo Dev config: %w", err)
	}
	return nil
}
