package hooks

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

const gajaePayloadFlag = "--payload-env GJC_NOTIFICATION_JSON"

func installGajaeConfig(path, executable, pushoverConfig string) (bool, error) {
	resolvedPath, err := resolveJSONHookPath(path, "Gajae Code")
	if err != nil {
		return false, err
	}
	document, err := readGajaeConfig(resolvedPath)
	if err != nil {
		return false, err
	}
	completion, created, err := rovoMapping(document.Content[0], "completion", "Gajae Code config")
	if err != nil {
		return false, err
	}
	current, exists, err := yamlScalarValueFor(completion, "notifyCommand", "Gajae Code completion config")
	if err != nil {
		return false, err
	}
	if exists && current != "" && !isOwnedCommandWithFlag(current, "gajae", "turn-complete", gajaePayloadFlag) {
		return false, fmt.Errorf("refusing to replace existing Gajae Code completion.notifyCommand")
	}
	command, err := hookNotifyCommandForOSWithFlags(
		runtime.GOOS, "gajae", "Gajae Code", executable, "turn-complete", pushoverConfig,
		"--payload-env", "GJC_NOTIFICATION_JSON",
	)
	if err != nil {
		return false, err
	}
	updated, err := upsertYAMLScalarFor(completion, "notifyCommand", &yaml.Node{
		Kind: yaml.ScalarNode, Tag: "!!str", Value: command, Style: yaml.DoubleQuotedStyle,
	}, "Gajae Code completion config")
	if err != nil {
		return false, err
	}
	if !created && !updated {
		return false, nil
	}
	if err := writeGajaeConfig(resolvedPath, document); err != nil {
		return false, err
	}
	return true, nil
}

func readGajaeConfig(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && len(bytes.TrimSpace(data)) == 0) {
		return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Gajae Code config: %w", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse Gajae Code config: %w", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("Gajae Code config top-level value must be a mapping")
	}
	return &document, nil
}

func writeGajaeConfig(path string, document *yaml.Node) error {
	data, err := yaml.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode Gajae Code config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create Gajae Code config directory: %w", err)
	}
	if err := writeGeneratedFile(path, data, ".gajae-config-*"); err != nil {
		return fmt.Errorf("write Gajae Code config: %w", err)
	}
	return nil
}
