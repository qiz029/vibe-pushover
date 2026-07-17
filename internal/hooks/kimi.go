package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

type kimiHookSpec struct {
	event string
	kind  string
}

func installKimiHooks(path, executable, pushoverConfig string) (bool, error) {
	writePath, err := resolveKimiConfigPath(path)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(writePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("read Kimi config: %w", err)
	}

	content := string(data)
	inlineHooks, err := hasInlineKimiHooks(content)
	if err != nil {
		return false, err
	}
	changed := false
	for _, spec := range []kimiHookSpec{
		{event: "Stop", kind: "turn-complete"},
		{event: "PermissionRequest", kind: "approval-required"},
	} {
		command := shellQuote(executable) + " notify --agent kimi --event " + spec.kind + " --ignore-errors"
		if pushoverConfig != "" {
			command += " --config " + shellQuote(pushoverConfig)
		}
		var blockChanged bool
		if inlineHooks {
			content, blockChanged, err = upsertInlineKimiHook(content, spec, renderInlineKimiHook(spec, command))
		} else {
			content, blockChanged, err = upsertKimiHook(content, spec, renderKimiHook(spec, command))
		}
		if err != nil {
			return false, err
		}
		changed = changed || blockChanged
	}
	if !changed {
		return false, nil
	}
	if err := validateKimiConfig(content); err != nil {
		return false, fmt.Errorf("generated Kimi config is invalid: %w", err)
	}
	if err := writeKimiConfig(writePath, []byte(content)); err != nil {
		return false, err
	}
	return true, nil
}

func resolveKimiConfigPath(path string) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect Kimi config: %w", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve Kimi config symlink: %w", err)
	}
	return resolved, nil
}

func validateKimiConfig(content string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	var root map[string]any
	if err := toml.Unmarshal([]byte(content), &root); err != nil {
		return fmt.Errorf("parse Kimi config: %w", err)
	}
	return nil
}

func hasInlineKimiHooks(content string) (bool, error) {
	if strings.TrimSpace(content) == "" {
		return false, nil
	}
	var root map[string]any
	if err := toml.Unmarshal([]byte(content), &root); err != nil {
		return false, fmt.Errorf("parse Kimi config: %w", err)
	}
	_, hasHooks := root["hooks"]
	return hasHooks && !hasKimiHookTable(content), nil
}

func inlineHooksArrayBounds(data []byte) (int, int, error) {
	parser := unstable.Parser{}
	parser.Reset(data)
	rootScope := true
	for parser.NextExpression() {
		expression := parser.Expression()
		switch expression.Kind {
		case unstable.Table, unstable.ArrayTable:
			rootScope = false
		case unstable.KeyValue:
			if !rootScope {
				continue
			}
			keys := expression.Key()
			if !keys.Next() || string(keys.Node().Data) != "hooks" || keys.Node().Next() != nil {
				continue
			}
			key := keys.Node().Raw
			return inlineHooksArrayDelimiters(data, int(key.Offset+key.Length))
		}
	}
	if err := parser.Error(); err != nil {
		return 0, 0, fmt.Errorf("parse Kimi config syntax: %w", err)
	}
	return 0, 0, errors.New("could not locate top-level inline Kimi hooks assignment")
}

func inlineHooksArrayDelimiters(data []byte, offset int) (int, int, error) {
	i := offset
	for i < len(data) && data[i] != '=' && data[i] != '\n' {
		i++
	}
	if i >= len(data) || data[i] != '=' {
		return 0, 0, errors.New("invalid inline Kimi hooks assignment")
	}
	i++
	for i < len(data) && (data[i] == ' ' || data[i] == '\t') {
		i++
	}
	if i >= len(data) || data[i] != '[' {
		return 0, 0, errors.New("inline Kimi hooks must be an array")
	}
	open := i

	depth := 0
	var quote byte
	triple := false
	for i < len(data) {
		if quote != 0 {
			if quote == '"' && data[i] == '\\' {
				i += 2
				continue
			}
			if triple {
				if i+2 < len(data) && data[i] == quote && data[i+1] == quote && data[i+2] == quote {
					quote = 0
					triple = false
					i += 3
					continue
				}
			} else if data[i] == quote {
				quote = 0
				i++
				continue
			}
			i++
			continue
		}

		switch data[i] {
		case '#':
			for i < len(data) && data[i] != '\n' {
				i++
			}
		case '"', '\'':
			quote = data[i]
			triple = i+2 < len(data) && data[i+1] == quote && data[i+2] == quote
			if triple {
				i += 3
			} else {
				i++
			}
		case '[':
			depth++
			i++
		case ']':
			depth--
			if depth == 0 {
				return open, i, nil
			}
			i++
		default:
			i++
		}
	}
	return 0, 0, errors.New("unterminated inline Kimi hooks array")
}

func hasKimiHookTable(content string) bool {
	parser := unstable.Parser{}
	parser.Reset([]byte(content))
	for parser.NextExpression() {
		expression := parser.Expression()
		if expression.Kind != unstable.ArrayTable {
			continue
		}
		keys := expression.Key()
		if keys.Next() && string(keys.Node().Data) == "hooks" && keys.Node().Next() == nil {
			return true
		}
	}
	return false
}

func renderKimiHook(spec kimiHookSpec, command string) string {
	begin, end := kimiHookMarkers(spec)
	return fmt.Sprintf("%s\n[[hooks]]\nevent = %s\ncommand = %s\ntimeout = 15\n%s\n",
		begin,
		tomlString(spec.event),
		tomlString(command),
		end,
	)
}

func renderInlineKimiHook(spec kimiHookSpec, command string) string {
	begin, end := kimiHookMarkers(spec)
	return fmt.Sprintf("%s\n  { event = %s, command = %s, timeout = 15 },\n  %s\n",
		begin,
		tomlString(spec.event),
		tomlString(command),
		end,
	)
}

func upsertInlineKimiHook(content string, spec kimiHookSpec, block string) (string, bool, error) {
	begin, end := kimiHookMarkers(spec)
	start := strings.Index(content, begin)
	if start >= 0 {
		endOffset := strings.Index(content[start+len(begin):], end)
		if endOffset < 0 {
			return "", false, fmt.Errorf("Kimi config contains an unterminated vibe-pushover hook for %s", spec.event)
		}
		finish := start + len(begin) + endOffset + len(end)
		if finish < len(content) && content[finish] == '\n' {
			finish++
		}
		if content[start:finish] == block {
			return content, false, nil
		}
		return content[:start] + block + content[finish:], true, nil
	}
	if strings.Contains(content, end) {
		return "", false, fmt.Errorf("Kimi config contains an unmatched vibe-pushover marker for %s", spec.event)
	}
	open, close, err := inlineHooksArrayBounds([]byte(content))
	if err != nil {
		return "", false, err
	}
	content, close = addInlineArraySeparator(content, open, close)
	prefix := ""
	if close > 0 && content[close-1] != '\n' {
		prefix = "\n"
	}
	return content[:close] + prefix + block + content[close:], true, nil
}

func addInlineArraySeparator(content string, open, close int) (string, int) {
	last := -1
	inComment := false
	var quote byte
	for i := open + 1; i < close; i++ {
		c := content[i]
		if inComment {
			if c == '\n' {
				inComment = false
			}
			continue
		}
		if quote != 0 {
			if quote == '"' && c == '\\' {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '#':
			inComment = true
		case '"', '\'':
			quote = c
			last = i
		case ' ', '\t', '\r', '\n':
		default:
			last = i
		}
	}
	if last < 0 || content[last] == ',' {
		return content, close
	}
	content = content[:last+1] + "," + content[last+1:]
	return content, close + 1
}

func tomlString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func upsertKimiHook(content string, spec kimiHookSpec, block string) (string, bool, error) {
	begin, end := kimiHookMarkers(spec)
	start := strings.Index(content, begin)
	if start < 0 {
		if strings.Contains(content, end) {
			return "", false, fmt.Errorf("Kimi config contains an unmatched vibe-pushover marker for %s", spec.event)
		}
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		if content != "" && !strings.HasSuffix(content, "\n\n") {
			content += "\n"
		}
		return content + block, true, nil
	}

	endOffset := strings.Index(content[start+len(begin):], end)
	if endOffset < 0 {
		return "", false, fmt.Errorf("Kimi config contains an unterminated vibe-pushover hook for %s", spec.event)
	}
	finish := start + len(begin) + endOffset + len(end)
	if finish < len(content) && content[finish] == '\n' {
		finish++
	}
	if content[start:finish] == block {
		return content, false, nil
	}
	return content[:start] + block + content[finish:], true, nil
}

func kimiHookMarkers(spec kimiHookSpec) (string, string) {
	id := spec.event + " " + spec.kind
	return "# BEGIN vibe-pushover hook: " + id, "# END vibe-pushover hook: " + id
}

func writeKimiConfig(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create Kimi config directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".vibe-pushover-*.toml")
	if err != nil {
		return fmt.Errorf("create temporary Kimi config: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("set Kimi config permissions: %w", err)
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write Kimi config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close Kimi config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace Kimi config: %w", err)
	}
	return nil
}
