package hooks

import "testing"

func TestRenderOpenHandsCommandQuotesWindowsPaths(t *testing.T) {
	got, err := renderOpenHandsCommand(
		"windows",
		`C:\Program Files\vibe-pushover.exe`,
		`C:\Users\Todd\pushover config.json`,
	)
	if err != nil {
		t.Fatalf("renderOpenHandsCommand() error = %v", err)
	}
	want := `"C:\Program Files\vibe-pushover.exe" notify --agent openhands --event turn-complete --ignore-errors --config "C:\Users\Todd\pushover config.json"`
	if got != want {
		t.Fatalf("renderOpenHandsCommand() = %q, want %q", got, want)
	}
}

func TestRenderOpenHandsCommandRejectsUnsafeWindowsPaths(t *testing.T) {
	for _, path := range []string{
		`C:\Users\%USERNAME%\vibe-pushover.exe`,
		`C:\Users\Todd!\vibe-pushover.exe`,
		"C:\\Users\\Todd\nvibe-pushover.exe",
	} {
		if _, err := renderOpenHandsCommand("windows", path, ""); err == nil {
			t.Fatalf("renderOpenHandsCommand(%q) error = nil", path)
		}
	}
}
