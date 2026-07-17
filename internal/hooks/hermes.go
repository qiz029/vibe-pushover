package hooks

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type hermesHookSpec struct {
	event string
	kind  string
	flag  string
}

func installHermesHooks(path, executable, pushoverConfig string) (bool, error) {
	document, err := readHermesConfig(path)
	if err != nil {
		return false, err
	}
	root := document.Content[0]
	hooks, changed, err := yamlMapping(root, "hooks")
	if err != nil {
		return false, err
	}
	for _, spec := range []hermesHookSpec{
		{event: "post_llm_call", kind: "turn-complete"},
		{event: "pre_approval_request", kind: "approval-required", flag: "--skip-noninteractive-approval"},
	} {
		command := shellQuote(executable) + " notify --agent hermes --event " + spec.kind + " --ignore-errors"
		if spec.flag != "" {
			command += " " + spec.flag
		}
		if pushoverConfig != "" {
			command += " --config " + shellQuote(pushoverConfig)
		}
		eventChanged, err := upsertHermesHook(hooks, spec.event, spec.kind, spec.flag, command)
		if err != nil {
			return false, err
		}
		changed = changed || eventChanged
	}
	if !changed {
		return false, nil
	}
	if err := writeHermesConfig(path, document); err != nil {
		return false, err
	}
	return true, nil
}

func readHermesConfig(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && len(bytes.TrimSpace(data)) == 0) {
		return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Hermes config: %w", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse Hermes config: %w", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("Hermes config top-level value must be a mapping")
	}
	return &document, nil
}

func yamlMapping(parent *yaml.Node, key string) (*yaml.Node, bool, error) {
	found := -1
	for index := 0; index+1 < len(parent.Content); index += 2 {
		if parent.Content[index].Value != key {
			continue
		}
		if found >= 0 {
			return nil, false, fmt.Errorf("Hermes config contains duplicate %q keys", key)
		}
		found = index + 1
	}
	if found >= 0 {
		if parent.Content[found].Kind != yaml.MappingNode {
			return nil, false, fmt.Errorf("Hermes config field %q must be a mapping", key)
		}
		return parent.Content[found], false, nil
	}
	want := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, want,
	)
	return want, true, nil
}

func upsertHermesHook(hooks *yaml.Node, event, kind, flag, command string) (bool, error) {
	sequence, created, err := yamlSequence(hooks, event)
	if err != nil {
		return false, err
	}
	for _, entry := range sequence.Content {
		if entry.Kind != yaml.MappingNode {
			continue
		}
		current, ok, err := yamlScalarValueFor(entry, "command", "Hermes hook")
		if err != nil {
			return false, err
		}
		if !ok {
			continue
		}
		owned := isOwnedCommand(current, "hermes", kind)
		if flag != "" {
			owned = owned || isOwnedCommandWithFlag(current, "hermes", kind, flag)
		}
		if !owned {
			continue
		}
		commandChanged, err := upsertYAMLScalarFor(entry, "command", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: command, Style: yaml.DoubleQuotedStyle}, "Hermes hook")
		if err != nil {
			return false, err
		}
		timeoutChanged, err := upsertYAMLScalarFor(entry, "timeout", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "10"}, "Hermes hook")
		return created || commandChanged || timeoutChanged, err
	}
	sequence.Content = append(sequence.Content, &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "command"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: command, Style: yaml.DoubleQuotedStyle},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "timeout"},
		{Kind: yaml.ScalarNode, Tag: "!!int", Value: "10"},
	}})
	return true, nil
}

func yamlSequence(parent *yaml.Node, key string) (*yaml.Node, bool, error) {
	found := -1
	for index := 0; index+1 < len(parent.Content); index += 2 {
		if parent.Content[index].Value != key {
			continue
		}
		if found >= 0 {
			return nil, false, fmt.Errorf("Hermes config hooks contain duplicate %q keys", key)
		}
		found = index + 1
	}
	if found >= 0 {
		if parent.Content[found].Kind != yaml.SequenceNode {
			return nil, false, fmt.Errorf("Hermes hook %q must be a sequence", key)
		}
		return parent.Content[found], false, nil
	}
	want := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, want,
	)
	return want, true, nil
}

func writeHermesConfig(path string, document *yaml.Node) error {
	data, err := yaml.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode Hermes config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create Hermes config directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".hermes-config-*")
	if err != nil {
		return fmt.Errorf("create temporary Hermes config: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("set Hermes config permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write Hermes config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close Hermes config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace Hermes config: %w", err)
	}
	return nil
}
