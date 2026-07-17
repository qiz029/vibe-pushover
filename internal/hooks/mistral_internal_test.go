package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderMistralCommandUsesHostShellQuoting(t *testing.T) {
	t.Parallel()

	unix, err := renderMistralCommand("darwin", "/Applications/Vibe Pushover/bin", "/tmp/pushover config.json")
	if err != nil {
		t.Fatalf("renderMistralCommand(darwin) error = %v", err)
	}
	if want := "'/Applications/Vibe Pushover/bin' notify --agent mistral --event turn-complete --ignore-errors --skip-mistral-subagent --config '/tmp/pushover config.json'"; unix != want {
		t.Fatalf("darwin command = %q, want %q", unix, want)
	}

	windows, err := renderMistralCommand("windows", `C:\\Program Files\\vibe-pushover.exe`, `C:\\Users\\Todd\\pushover config.json`)
	if err != nil {
		t.Fatalf("renderMistralCommand(windows) error = %v", err)
	}
	if want := `"C:\\Program Files\\vibe-pushover.exe" notify --agent mistral --event turn-complete --ignore-errors --skip-mistral-subagent --config "C:\\Users\\Todd\\pushover config.json"`; windows != want {
		t.Fatalf("windows command = %q, want %q", windows, want)
	}
}

func TestRenderMistralCommandRejectsWindowsExpansionCharacters(t *testing.T) {
	t.Parallel()

	for _, path := range []string{`C:\\Users\\%USERNAME%\\vibe-pushover.exe`, `C:\\Users\\Todd!\\vibe-pushover.exe`} {
		if _, err := renderMistralCommand("windows", path, ""); err == nil {
			t.Fatalf("renderMistralCommand(%q) error = nil, want unsafe path error", path)
		}
	}
}

func TestRollbackMistralHooksRestoresOrRemovesManagedWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.toml")
	if err := os.WriteFile(existing, []byte("new"), 0o600); err != nil {
		t.Fatalf("WriteFile(existing) error = %v", err)
	}
	if err := rollbackMistralHooks(existing, []byte("original"), true); err != nil {
		t.Fatalf("rollbackMistralHooks(existing) error = %v", err)
	}
	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("ReadFile(existing) error = %v", err)
	}
	if string(data) != "original" {
		t.Fatalf("existing contents = %q, want original", data)
	}

	created := filepath.Join(dir, "created.toml")
	if err := os.WriteFile(created, []byte("new"), 0o600); err != nil {
		t.Fatalf("WriteFile(created) error = %v", err)
	}
	if err := rollbackMistralHooks(created, nil, false); err != nil {
		t.Fatalf("rollbackMistralHooks(created) error = %v", err)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Fatalf("Stat(created) error = %v, want not exist", err)
	}
}

func TestWindowsShellQuoteErrorExplainsConstraint(t *testing.T) {
	t.Parallel()

	_, err := windowsShellQuote(`C:\\Users\\%USERNAME%\\vibe-pushover.exe`)
	if err == nil || !strings.Contains(err.Error(), "%") {
		t.Fatalf("windowsShellQuote() error = %v, want percent explanation", err)
	}
}
