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

type codeWhaleHookSpec struct {
	name  string
	event string
	kind  string
}

func installCodeWhaleHooks(path, executable, pushoverConfig string) (bool, error) {
	writePath, err := resolveCodeWhaleConfigPath(path)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(writePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("read CodeWhale config: %w", err)
	}
	content := string(data)
	if err := validateCodeWhaleConfig(content); err != nil {
		return false, err
	}

	specs := []codeWhaleHookSpec{
		{name: "vibe-pushover-turn-complete", event: "turn_end", kind: "turn-complete"},
		{name: "vibe-pushover-attention-required", event: "on_error", kind: "attention-required"},
	}
	configuredNames, err := codeWhaleHookNames(content)
	if err != nil {
		return false, err
	}
	changed := false
	for _, spec := range specs {
		_, _, managed, err := codeWhaleManagedBlockBounds(content, spec)
		if err != nil {
			return false, err
		}
		if !managed && configuredNames[spec.name] {
			return false, fmt.Errorf("CodeWhale hook name %q is already owned by another configuration", spec.name)
		}
		command, err := hookNotifyCommandForOS(runtime.GOOS, "codewhale", "CodeWhale", executable, spec.kind, pushoverConfig)
		if err != nil {
			return false, err
		}
		var blockChanged bool
		content, blockChanged, err = upsertCodeWhaleHook(content, spec, renderCodeWhaleHook(spec, command))
		if err != nil {
			return false, err
		}
		changed = changed || blockChanged
	}
	if !changed {
		return false, nil
	}
	if err := validateCodeWhaleConfig(content); err != nil {
		return false, fmt.Errorf("generated CodeWhale config is invalid: %w", err)
	}
	if err := writeCodeWhaleConfig(writePath, []byte(content)); err != nil {
		return false, err
	}
	return true, nil
}

func resolveCodeWhaleConfigPath(path string) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect CodeWhale config: %w", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve CodeWhale config symlink: %w", err)
	}
	return resolved, nil
}

func validateCodeWhaleConfig(content string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	var root map[string]any
	if err := toml.Unmarshal([]byte(content), &root); err != nil {
		return fmt.Errorf("parse CodeWhale config: %w", err)
	}
	return nil
}

func codeWhaleHookNames(content string) (map[string]bool, error) {
	result := map[string]bool{}
	if strings.TrimSpace(content) == "" {
		return result, nil
	}
	var root struct {
		Hooks struct {
			Hooks []struct {
				Name string `toml:"name"`
			} `toml:"hooks"`
		} `toml:"hooks"`
	}
	if err := toml.Unmarshal([]byte(content), &root); err != nil {
		return nil, fmt.Errorf("parse CodeWhale config: %w", err)
	}
	for _, hook := range root.Hooks.Hooks {
		if hook.Name != "" {
			result[hook.Name] = true
		}
	}
	return result, nil
}

func renderCodeWhaleHook(spec codeWhaleHookSpec, command string) string {
	begin, end := codeWhaleHookMarkers(spec)
	return fmt.Sprintf("%s\n[[hooks.hooks]]\nname = %s\nevent = %s\ncommand = %s\ntimeout_secs = 15\ncontinue_on_error = true\n%s\n",
		begin,
		tomlString(spec.name),
		tomlString(spec.event),
		tomlString(command),
		end,
	)
}

func upsertCodeWhaleHook(content string, spec codeWhaleHookSpec, block string) (string, bool, error) {
	start, finish, found, err := codeWhaleManagedBlockBounds(content, spec)
	if err != nil {
		return "", false, err
	}
	if !found {
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		if content != "" && !strings.HasSuffix(content, "\n\n") {
			content += "\n"
		}
		return content + block, true, nil
	}
	if content[start:finish] == block {
		return content, false, nil
	}
	return content[:start] + block + content[finish:], true, nil
}

func codeWhaleHookMarkers(spec codeWhaleHookSpec) (string, string) {
	return "# BEGIN vibe-pushover hook: " + spec.name, "# END vibe-pushover hook: " + spec.name
}

func codeWhaleManagedBlockBounds(content string, spec codeWhaleHookSpec) (int, int, bool, error) {
	begin, end := codeWhaleHookMarkers(spec)
	parser := unstable.Parser{KeepComments: true}
	parser.Reset([]byte(content))
	start, finish := -1, -1
	for parser.NextExpression() {
		expression := parser.Expression()
		if expression.Kind != unstable.Comment {
			continue
		}
		switch strings.TrimSpace(string(expression.Data)) {
		case begin:
			if start >= 0 {
				return 0, 0, false, fmt.Errorf("CodeWhale config contains multiple begin markers for %s", spec.event)
			}
			start = int(expression.Raw.Offset)
		case end:
			if finish >= 0 {
				return 0, 0, false, fmt.Errorf("CodeWhale config contains multiple end markers for %s", spec.event)
			}
			finish = int(expression.Raw.Offset + expression.Raw.Length)
		}
	}
	if err := parser.Error(); err != nil {
		return 0, 0, false, fmt.Errorf("parse CodeWhale hook markers: %w", err)
	}
	if start < 0 && finish < 0 {
		return 0, 0, false, nil
	}
	if start < 0 || finish < 0 || finish < start {
		return 0, 0, false, fmt.Errorf("CodeWhale config contains unmatched vibe-pushover markers for %s", spec.event)
	}
	if finish < len(content) && content[finish] == '\r' {
		finish++
	}
	if finish < len(content) && content[finish] == '\n' {
		finish++
	}
	return start, finish, true, nil
}

func writeCodeWhaleConfig(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create CodeWhale config directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".vibe-pushover-*.toml")
	if err != nil {
		return fmt.Errorf("create temporary CodeWhale config: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("set CodeWhale config permissions: %w", err)
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write CodeWhale config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close CodeWhale config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace CodeWhale config: %w", err)
	}
	return nil
}
