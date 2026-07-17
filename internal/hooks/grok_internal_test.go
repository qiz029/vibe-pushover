package hooks

import "testing"

func TestGrokNotifyCommandUsesWindowsShellQuoting(t *testing.T) {
	t.Parallel()

	got, err := grokNotifyCommand(
		"windows",
		`C:\Program Files\vibe-pushover.exe`,
		"turn-complete",
		`C:\Users\Todd\pushover config.json`,
	)
	if err != nil {
		t.Fatalf("grokNotifyCommand() error = %v", err)
	}
	want := `"C:\Program Files\vibe-pushover.exe" notify --agent grok --event turn-complete --ignore-errors --config "C:\Users\Todd\pushover config.json"`
	if got != want {
		t.Fatalf("grokNotifyCommand() = %q, want %q", got, want)
	}
}

func TestGrokWindowsHookUpdateIsIdempotent(t *testing.T) {
	t.Parallel()

	command, err := grokNotifyCommand("windows", `C:\Program Files\vibe-pushover.exe`, "turn-complete", "")
	if err != nil {
		t.Fatalf("grokNotifyCommand() error = %v", err)
	}
	want := hookGroup{Hooks: []hookCommand{{Type: "command", Command: command, Timeout: 10}}}
	entries, changed, err := upsert(nil, "grok", "turn-complete", want)
	if err != nil || !changed {
		t.Fatalf("first upsert() = changed %v, error %v", changed, err)
	}
	entries, changed, err = upsert(entries, "grok", "turn-complete", want)
	if err != nil {
		t.Fatalf("second upsert() error = %v", err)
	}
	if changed || len(entries) != 1 {
		t.Fatalf("second upsert() = changed %v, entries %d; want unchanged single hook", changed, len(entries))
	}
}
