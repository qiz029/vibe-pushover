package hooks

import "testing"

func TestTraeNotifyCommandUsesWindowsShellQuoting(t *testing.T) {
	t.Parallel()

	got, err := hookNotifyCommandForOS(
		"windows",
		"trae",
		"TRAE",
		`C:\Program Files\vibe-pushover.exe`,
		"approval-required",
		`C:\Users\Todd\pushover config.json`,
	)
	if err != nil {
		t.Fatalf("hookNotifyCommandForOS() error = %v", err)
	}
	want := `"C:\Program Files\vibe-pushover.exe" notify --agent trae --event approval-required --ignore-errors --config "C:\Users\Todd\pushover config.json"`
	if got != want {
		t.Fatalf("hookNotifyCommandForOS() = %q, want %q", got, want)
	}
}
