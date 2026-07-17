package hooks

import (
	"path/filepath"
	"testing"
)

func TestMiMoPluginPathUsesLocalAppDataOnWindows(t *testing.T) {
	t.Parallel()

	values := map[string]string{"LOCALAPPDATA": `C:\Users\Todd\AppData\Local`}
	got, err := mimoPluginPath("windows", `C:\Users\Todd`, func(key string) string { return values[key] })
	if err != nil {
		t.Fatalf("mimoPluginPath() error = %v", err)
	}
	want := filepath.Join(values["LOCALAPPDATA"], "mimocode", "plugins", "vibe-pushover.ts")
	if got != want {
		t.Fatalf("mimoPluginPath() = %q, want %q", got, want)
	}
}
