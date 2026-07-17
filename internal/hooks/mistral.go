package hooks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

const (
	mistralHookName  = "vibe-pushover-turn-complete"
	mistralHookBegin = "# BEGIN vibe-pushover hook: post_agent_turn"
	mistralHookEnd   = "# END vibe-pushover hook: post_agent_turn"
)

func installMistralVibeHooks(path, executable, pushoverConfig string) (bool, error) {
	configPath := filepath.Join(filepath.Dir(path), "config.toml")
	hooksWritePath, err := resolveMistralPath(path, "hooks")
	if err != nil {
		return false, err
	}
	configWritePath, err := resolveMistralPath(configPath, "config")
	if err != nil {
		return false, err
	}

	hooksData, hooksExisted, err := readOptionalMistralFile(hooksWritePath, "hooks")
	if err != nil {
		return false, err
	}
	configData, _, err := readOptionalMistralFile(configWritePath, "config")
	if err != nil {
		return false, err
	}

	command, err := renderMistralCommand(runtime.GOOS, executable, pushoverConfig)
	if err != nil {
		return false, err
	}
	hookBlock := renderMistralHook(command)
	updatedHooks, hooksChanged, err := upsertMistralHook(string(hooksData), hookBlock)
	if err != nil {
		return false, err
	}
	updatedConfig, configChanged, err := enableMistralHooks(string(configData))
	if err != nil {
		return false, err
	}
	if !hooksChanged && !configChanged {
		return false, nil
	}

	if hooksChanged {
		if err := writeMistralTOML(hooksWritePath, []byte(updatedHooks), "hooks"); err != nil {
			return false, err
		}
	}
	if configChanged {
		if err := writeMistralTOML(configWritePath, []byte(updatedConfig), "config"); err != nil {
			if hooksChanged {
				rollbackErr := rollbackMistralHooks(hooksWritePath, hooksData, hooksExisted)
				if rollbackErr != nil {
					return false, fmt.Errorf("%w; rollback Mistral Vibe hooks: %v", err, rollbackErr)
				}
			}
			return false, err
		}
	}
	return true, nil
}

func renderMistralCommand(goos, executable, pushoverConfig string) (string, error) {
	quote := func(value string) (string, error) {
		return shellQuote(value), nil
	}
	if goos == "windows" {
		quote = windowsShellQuote
	}
	executableArg, err := quote(executable)
	if err != nil {
		return "", fmt.Errorf("quote Mistral Vibe executable: %w", err)
	}
	command := executableArg + " notify --agent mistral --event turn-complete --ignore-errors --skip-mistral-subagent"
	if pushoverConfig == "" {
		return command, nil
	}
	configArg, err := quote(pushoverConfig)
	if err != nil {
		return "", fmt.Errorf("quote Mistral Vibe config: %w", err)
	}
	return command + " --config " + configArg, nil
}

func windowsShellQuote(value string) (string, error) {
	if strings.ContainsAny(value, "\r\n\x00%!") {
		return "", errors.New("Windows hook paths cannot contain newlines, NUL, %, or !")
	}
	if strings.Contains(value, `"`) {
		return "", errors.New(`Windows hook paths cannot contain "`)
	}
	return `"` + value + `"`, nil
}

func renderMistralHook(command string) string {
	return fmt.Sprintf(
		"%s\n[[hooks]]\nname = %s\ntype = \"post_agent_turn\"\ncommand = %s\ntimeout = 10.0\ndescription = \"Send a Pushover notification when Mistral Vibe finishes a top-level turn.\"\n%s\n",
		mistralHookBegin, tomlString(mistralHookName), tomlString(command), mistralHookEnd,
	)
}

func upsertMistralHook(content, block string) (string, bool, error) {
	if err := validateMistralTOML(content, "hooks"); err != nil {
		return "", false, err
	}
	start, finish, found, err := mistralManagedBlockBounds(content)
	if err != nil {
		return "", false, err
	}
	if !found {
		if err := ensureNoUnownedMistralHook(content); err != nil {
			return "", false, err
		}
		return appendTOMLBlock(content, block), true, nil
	}
	if err := ensureNoUnownedMistralHook(content[:start] + "\n" + content[finish:]); err != nil {
		return "", false, err
	}
	if content[start:finish] == block {
		return content, false, nil
	}
	updated := content[:start] + block + content[finish:]
	if err := validateMistralTOML(updated, "hooks"); err != nil {
		return "", false, err
	}
	return updated, true, nil
}

func mistralManagedBlockBounds(content string) (int, int, bool, error) {
	parser := unstable.Parser{KeepComments: true}
	parser.Reset([]byte(content))
	start, finish := -1, -1
	for parser.NextExpression() {
		expression := parser.Expression()
		if expression.Kind != unstable.Comment {
			continue
		}
		switch strings.TrimSpace(string(expression.Data)) {
		case mistralHookBegin:
			if start >= 0 {
				return 0, 0, false, errors.New("Mistral Vibe hooks contain multiple vibe-pushover begin markers")
			}
			start = int(expression.Raw.Offset)
		case mistralHookEnd:
			if finish >= 0 {
				return 0, 0, false, errors.New("Mistral Vibe hooks contain multiple vibe-pushover end markers")
			}
			finish = int(expression.Raw.Offset + expression.Raw.Length)
		}
	}
	if err := parser.Error(); err != nil {
		return 0, 0, false, fmt.Errorf("parse Mistral Vibe hook markers: %w", err)
	}
	if start < 0 && finish < 0 {
		return 0, 0, false, nil
	}
	if start < 0 || finish < 0 || finish < start {
		return 0, 0, false, errors.New("Mistral Vibe hooks contain unmatched vibe-pushover markers")
	}
	if finish < len(content) && content[finish] == '\r' {
		finish++
	}
	if finish < len(content) && content[finish] == '\n' {
		finish++
	}
	return start, finish, true, nil
}

func ensureNoUnownedMistralHook(content string) error {
	var root struct {
		Hooks []struct {
			Name string `toml:"name"`
		} `toml:"hooks"`
	}
	if strings.TrimSpace(content) != "" {
		if err := toml.Unmarshal([]byte(content), &root); err != nil {
			return fmt.Errorf("parse Mistral Vibe hooks: %w", err)
		}
	}
	for _, hook := range root.Hooks {
		if hook.Name == mistralHookName {
			return fmt.Errorf("Mistral Vibe hook %q already exists and is not owned by vibe-pushover", mistralHookName)
		}
	}
	return nil
}

func enableMistralHooks(content string) (string, bool, error) {
	if err := validateMistralTOML(content, "config"); err != nil {
		return "", false, err
	}
	if strings.TrimSpace(content) != "" {
		var root map[string]any
		if err := toml.Unmarshal([]byte(content), &root); err != nil {
			return "", false, fmt.Errorf("parse Mistral Vibe config: %w", err)
		}
		if value, ok := root["enable_experimental_hooks"]; ok {
			if _, ok := value.(bool); !ok {
				return "", false, errors.New("Mistral Vibe config field enable_experimental_hooks must be a boolean")
			}
		}
	}

	data := []byte(content)
	parser := unstable.Parser{}
	parser.Reset(data)
	rootScope := true
	firstTable := -1
	for parser.NextExpression() {
		expression := parser.Expression()
		switch expression.Kind {
		case unstable.Table, unstable.ArrayTable:
			rootScope = false
			if firstTable < 0 {
				firstTable = int(expression.Raw.Offset)
			}
		case unstable.KeyValue:
			if !rootScope {
				continue
			}
			keys := expression.Key()
			if !keys.Next() || string(keys.Node().Data) != "enable_experimental_hooks" || keys.Node().Next() != nil {
				continue
			}
			key := keys.Node().Raw
			start, end, err := tomlBooleanValueBounds(data, int(key.Offset+key.Length))
			if err != nil {
				return "", false, err
			}
			if string(data[start:end]) == "true" {
				return content, false, nil
			}
			updated := string(data[:start]) + "true" + string(data[end:])
			if err := validateMistralTOML(updated, "config"); err != nil {
				return "", false, err
			}
			return updated, true, nil
		}
	}
	if err := parser.Error(); err != nil {
		return "", false, fmt.Errorf("parse Mistral Vibe config syntax: %w", err)
	}

	assignment := "enable_experimental_hooks = true\n"
	if firstTable >= 0 {
		prefix := string(data[:firstTable])
		if prefix != "" && !strings.HasSuffix(prefix, "\n") {
			prefix += "\n"
		}
		if prefix != "" && !strings.HasSuffix(prefix, "\n\n") {
			prefix += "\n"
		}
		updated := prefix + assignment + "\n" + string(data[firstTable:])
		return updated, true, validateMistralTOML(updated, "config")
	}
	updated := appendTOMLBlock(content, assignment)
	return updated, true, validateMistralTOML(updated, "config")
}

func tomlBooleanValueBounds(data []byte, offset int) (int, int, error) {
	i := offset
	for i < len(data) && data[i] != '=' && data[i] != '\n' {
		i++
	}
	if i >= len(data) || data[i] != '=' {
		return 0, 0, errors.New("Mistral Vibe config has an invalid enable_experimental_hooks assignment")
	}
	i++
	for i < len(data) && (data[i] == ' ' || data[i] == '\t') {
		i++
	}
	switch {
	case i+4 <= len(data) && string(data[i:i+4]) == "true":
		return i, i + 4, nil
	case i+5 <= len(data) && string(data[i:i+5]) == "false":
		return i, i + 5, nil
	default:
		return 0, 0, errors.New("Mistral Vibe config field enable_experimental_hooks must be a boolean")
	}
}

func appendTOMLBlock(content, block string) string {
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if content != "" && !strings.HasSuffix(content, "\n\n") {
		content += "\n"
	}
	return content + block
}

func validateMistralTOML(content, kind string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	var root map[string]any
	if err := toml.Unmarshal([]byte(content), &root); err != nil {
		return fmt.Errorf("parse Mistral Vibe %s: %w", kind, err)
	}
	return nil
}

func resolveMistralPath(path, kind string) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect Mistral Vibe %s: %w", kind, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve Mistral Vibe %s symlink: %w", kind, err)
	}
	return resolved, nil
}

func readOptionalMistralFile(path, kind string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read Mistral Vibe %s: %w", kind, err)
	}
	return data, true, nil
}

func rollbackMistralHooks(path string, content []byte, existed bool) error {
	if !existed {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	return writeMistralTOML(path, content, "hooks rollback")
}

func writeMistralTOML(path string, content []byte, kind string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create Mistral Vibe %s directory: %w", kind, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".vibe-pushover-*.toml")
	if err != nil {
		return fmt.Errorf("create temporary Mistral Vibe %s: %w", kind, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("set Mistral Vibe %s permissions: %w", kind, err)
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write Mistral Vibe %s: %w", kind, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close Mistral Vibe %s: %w", kind, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace Mistral Vibe %s: %w", kind, err)
	}
	return nil
}
