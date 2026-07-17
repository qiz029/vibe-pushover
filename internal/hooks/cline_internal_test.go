package hooks

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClineWindowsHookUsesPowerShellAndEscapesPaths(t *testing.T) {
	t.Parallel()

	got := clineHookScriptForOS(
		"windows",
		`C:\Program Files\vibe-pushover.exe`,
		`C:\Users\O'Brien\pushover.json`,
	)
	for _, want := range []string{
		clineHookMarker,
		`$payload = [Console]::In.ReadToEnd()`,
		`$OutputEncoding = New-Object System.Text.UTF8Encoding $false`,
		`$payload | & 'C:\Program Files\vibe-pushover.exe' notify --agent cline --event turn-complete --ignore-errors --skip-cline-subagent --config 'C:\Users\O''Brien\pushover.json' | Out-Null`,
		`{"cancel":false,"contextModification":"","errorMessage":""}`,
		"exit 0",
	} {
		if !bytes.Contains(got, []byte(want)) {
			t.Fatalf("Windows Cline hook does not contain %q:\n%s", want, got)
		}
	}
	if bytes.Contains(got, []byte("#!/bin/sh")) {
		t.Fatalf("Windows Cline hook contains a Unix shebang:\n%s", got)
	}
}

func TestInstalledClineHookForwardsPayloadAndReturnsControlJSON(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix hook execution test")
	}
	t.Parallel()

	dir := t.TempDir()
	binary := filepath.Join(dir, "fake vibe-pushover")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\ncat > \"$CAPTURE_PATH\"\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(fake binary) error = %v", err)
	}
	hook := filepath.Join(dir, "hooks", "TaskComplete")
	changed, err := Install("cline", hook, binary, "")
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !changed {
		t.Fatal("Install() changed = false, want true")
	}

	payload := `{"hookName":"TaskComplete","taskId":"task-42"}`
	capture := filepath.Join(dir, "captured.json")
	command := exec.Command(hook)
	command.Env = append(os.Environ(), "CAPTURE_PATH="+capture)
	command.Stdin = strings.NewReader(payload)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("Cline hook execution error = %v", err)
	}
	if strings.TrimSpace(string(output)) != `{"cancel":false,"contextModification":"","errorMessage":""}` {
		t.Fatalf("Cline hook stdout = %q", output)
	}
	got, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("ReadFile(capture) error = %v", err)
	}
	if string(got) != payload {
		t.Fatalf("forwarded payload = %q, want %q", got, payload)
	}
}

func TestInstallClinePreservesOwnedHookSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior requires Windows developer mode or elevation")
	}
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "managed", "TaskComplete")
	if _, err := Install("cline", target, "/opt/bin/vibe-pushover-old", ""); err != nil {
		t.Fatalf("initial Install() error = %v", err)
	}
	link := filepath.Join(dir, "hooks", "TaskComplete")
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	changed, err := Install("cline", link, "/opt/bin/vibe-pushover-new", "/tmp/pushover.json")
	if err != nil {
		t.Fatalf("Install(symlink) error = %v", err)
	}
	if !changed {
		t.Fatal("Install(symlink) changed = false, want true")
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat(link) error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("Install() replaced the Cline hook symlink")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(target) error = %v", err)
	}
	for _, want := range []string{"/opt/bin/vibe-pushover-new", "--config '/tmp/pushover.json'"} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("updated symlink target does not contain %q:\n%s", want, data)
		}
	}
}

func TestClineDefaultPathsCoverRedirectedWindowsDocumentsAndCLI(t *testing.T) {
	t.Parallel()

	home := `C:\Users\Todd`
	documents := `C:\Users\Todd\OneDrive\Documents`
	paths := clineDefaultPaths("windows", home, "", func(name string, args ...string) (string, error) {
		if name != "powershell" || len(args) == 0 {
			t.Fatalf("documents resolver command = %q %#v", name, args)
		}
		return documents + "\r\n", nil
	})
	want := []string{
		filepath.Join(documents, "Cline", "Hooks", "TaskComplete.ps1"),
		filepath.Join(home, ".cline", "hooks", "TaskComplete.ps1"),
	}
	if len(paths) != len(want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
	for index := range want {
		if paths[index] != want[index] {
			t.Fatalf("paths[%d] = %q, want %q", index, paths[index], want[index])
		}
	}
}

func TestClineDefaultPathsAvoidDuplicateWhenDocumentsUseFallback(t *testing.T) {
	t.Parallel()

	home := "/home/todd"
	paths := clineDefaultPaths("linux", home, "", func(name string, _ ...string) (string, error) {
		if name != "xdg-user-dir" {
			t.Fatalf("documents resolver command = %q", name)
		}
		return filepath.Join(home, "Documents") + "\n", nil
	})
	want := filepath.Join(home, "Documents", "Cline", "Hooks", "TaskComplete")
	if len(paths) != 1 || paths[0] != want {
		t.Fatalf("paths = %#v, want [%q]", paths, want)
	}
}

func TestInstallAllClinePathsPreflightsOwnership(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := filepath.Join(dir, "documents", "Cline", "Hooks", "TaskComplete")
	second := filepath.Join(dir, ".cline", "hooks", "TaskComplete")
	if err := os.MkdirAll(filepath.Dir(second), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	original := []byte("#!/bin/sh\necho personal-cline-hook\n")
	if err := os.WriteFile(second, original, 0o700); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := InstallAll("cline", []string{first, second}, "/opt/bin/vibe-pushover", ""); err == nil {
		t.Fatal("InstallAll() accepted an unowned Cline hook")
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("InstallAll() wrote first path before preflight completed: %v", err)
	}
	got, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("ReadFile(second) error = %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("InstallAll() changed personal hook:\n%s", got)
	}
}
