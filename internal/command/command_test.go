package command_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qiz029/vibe-pushover/internal/command"
	"github.com/qiz029/vibe-pushover/internal/config"
)

func TestSetupCommandInteractivelyStoresCredentials(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin:  bytes.NewBufferString("app-token\nuser-key\n"),
		Stdout: &stdout,
	})
	err := app.Run(context.Background(), []string{
		"vibe-pushover", "setup",
		"--config", path,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := config.Credentials{AppToken: "app-token", UserKey: "user-key"}
	if got != want {
		t.Fatalf("credentials = %#v, want %#v", got, want)
	}
	if strings.Contains(stdout.String(), "app-token") || strings.Contains(stdout.String(), "user-key") {
		t.Fatalf("setup output exposed credentials: %q", stdout.String())
	}
}

func TestSetupCommandRepromptsForEmptyCredential(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin:  bytes.NewBufferString("\napp-token\nuser-key\n"),
		Stdout: &stdout,
	})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "setup", "--config", path}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Value cannot be empty") {
		t.Fatalf("setup output = %q, want empty-value guidance", stdout.String())
	}
	if _, err := config.Load(path); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestSetupCommandStoresInteractiveNotificationProfile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString("app-token\nuser-key\nwatch\n"), Stdout: &bytes.Buffer{},
	})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "setup", "--config", path}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.NotificationProfile != "watch" {
		t.Fatalf("NotificationProfile = %q, want watch", got.NotificationProfile)
	}
}

func TestNotifyCommandSendsHookPayload(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var received map[string]string
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm() error = %v", err)
		}
		received = map[string]string{
			"title":    r.Form.Get("title"),
			"message":  r.Form.Get("message"),
			"priority": r.Form.Get("priority"),
			"sound":    r.Form.Get("sound"),
			"ttl":      r.Form.Get("ttl"),
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"status":1,"request":"request-id"}`)),
			Header:     make(http.Header),
		}, nil
	})}

	app := command.New(command.Options{
		Stdin:      bytes.NewBufferString(`{"cwd":"/tmp/demo","tool_name":"Bash","tool_input":{"command":"make deploy"}}`),
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		HTTPClient: httpClient,
		Endpoint:   "https://pushover.test/messages.json",
	})
	err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify",
		"--config", path,
		"--agent", "kimi",
		"--event", "approval-required",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := received
	if got["title"] != "⚠ Kimi needs approval · demo" {
		t.Fatalf("title = %q", got["title"])
	}
	if got["message"] != "Bash\nmake deploy" {
		t.Fatalf("message = %q", got["message"])
	}
	if got["priority"] != "1" {
		t.Fatalf("priority = %q, want 1", got["priority"])
	}
	if got["sound"] != "persistent" {
		t.Fatalf("sound = %q, want persistent", got["sound"])
	}
	if got["ttl"] != "1800" {
		t.Fatalf("ttl = %q, want 1800", got["ttl"])
	}
}

func TestNotifyCommandDeduplicatesImmediateRepeatAcrossProcesses(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := config.Save(configPath, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	requests := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"status":1,"request":"request-id"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	run := func() {
		t.Helper()
		app := command.New(command.Options{
			Stdin:      bytes.NewBufferString(`{"session_id":"session-1","turn_id":"turn-4","cwd":"/tmp/demo","last_assistant_message":"Tests pass."}`),
			Stdout:     &bytes.Buffer{},
			Stderr:     &bytes.Buffer{},
			HTTPClient: httpClient,
			Endpoint:   "https://pushover.test/messages.json",
			DedupePath: filepath.Join(dir, "dedupe.json"),
			Now:        func() time.Time { return now },
		})
		if err := app.Run(context.Background(), []string{
			"vibe-pushover", "notify", "--config", configPath, "--agent", "codex", "--event", "turn-complete",
		}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	}
	run()
	run()
	if requests != 1 {
		t.Fatalf("Pushover requests = %d, want 1 for an immediate duplicate", requests)
	}
}

func TestNotifyCommandSendsSameNotificationAfterDedupeWindow(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := config.Save(configPath, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	requests := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
	})}
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	run := func() {
		t.Helper()
		app := command.New(command.Options{
			Stdin:  bytes.NewBufferString(`{"session_id":"session-1","cwd":"/tmp/demo","last_assistant_message":"Done."}`),
			Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, HTTPClient: httpClient,
			Endpoint: "https://pushover.test/messages.json", DedupePath: filepath.Join(dir, "dedupe.json"),
			Now: func() time.Time { return now },
		})
		if err := app.Run(context.Background(), []string{
			"vibe-pushover", "notify", "--config", configPath, "--agent", "codex", "--event", "turn-complete",
		}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	}
	run()
	now = now.Add(4 * time.Second)
	run()
	if requests != 2 {
		t.Fatalf("Pushover requests = %d, want 2 after the dedupe window", requests)
	}
}

func TestNotifyCommandReleasesDedupeReservationAfterDeliveryFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := config.Save(configPath, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	requests := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return nil, errors.New("temporary network failure")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
	})}
	run := func() error {
		app := command.New(command.Options{
			Stdin:  bytes.NewBufferString(`{"session_id":"session-1","turn_id":"turn-1","cwd":"/tmp/demo"}`),
			Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, HTTPClient: httpClient,
			Endpoint: "https://pushover.test/messages.json", DedupePath: filepath.Join(dir, "dedupe.json"),
		})
		return app.Run(context.Background(), []string{
			"vibe-pushover", "notify", "--config", configPath, "--agent", "codex", "--event", "turn-complete",
		})
	}
	if err := run(); err == nil {
		t.Fatal("first Run() error = nil, want temporary delivery failure")
	}
	if err := run(); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("Pushover requests = %d, want retry after reservation release", requests)
	}
}

func TestNotifyCommandPendingDuplicateRetriesWhenOwnerFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := config.Save(configPath, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var requests atomic.Int32
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		if requests.Add(1) == 1 {
			close(firstStarted)
			<-releaseFirst
			return nil, errors.New("temporary network failure")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
	})}
	run := func() error {
		app := command.New(command.Options{
			Stdin:  bytes.NewBufferString(`{"session_id":"session-1","turn_id":"turn-1","cwd":"/tmp/demo"}`),
			Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, HTTPClient: httpClient,
			Endpoint: "https://pushover.test/messages.json", DedupePath: filepath.Join(dir, "dedupe.json"),
		})
		return app.Run(context.Background(), []string{
			"vibe-pushover", "notify", "--config", configPath, "--agent", "codex", "--event", "turn-complete",
		})
	}
	firstResult := make(chan error, 1)
	secondResult := make(chan error, 1)
	go func() { firstResult <- run() }()
	<-firstStarted
	go func() { secondResult <- run() }()
	select {
	case err := <-secondResult:
		t.Fatalf("pending duplicate returned before owner settled: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseFirst)
	if err := <-firstResult; err == nil {
		t.Fatal("first Run() error = nil, want temporary delivery failure")
	}
	if err := <-secondResult; err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if requests.Load() != 2 {
		t.Fatalf("Pushover requests = %d, want failed owner plus retry", requests.Load())
	}
}

func TestNotifyCommandFailsOpenWhenDedupeStateIsCorrupt(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	dedupePath := filepath.Join(dir, "dedupe.json")
	if err := config.Save(configPath, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := os.WriteFile(dedupePath, []byte("not-json"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	requests := 0
	var stderr bytes.Buffer
	app := command.New(command.Options{
		Stdin:  bytes.NewBufferString(`{"session_id":"session-1","cwd":"/tmp/demo"}`),
		Stdout: &bytes.Buffer{}, Stderr: &stderr,
		HTTPClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
		})},
		Endpoint: "https://pushover.test/messages.json", DedupePath: dedupePath,
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", configPath, "--agent", "codex", "--event", "turn-complete",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("Pushover requests = %d, want fail-open delivery", requests)
	}
	if !strings.Contains(stderr.String(), "dedupe unavailable") {
		t.Fatalf("stderr = %q, want dedupe warning", stderr.String())
	}
}

func TestNotifyCommandDoesNotDeduplicateDifferentNumericTurnIDs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := config.Save(configPath, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	requests := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
	})}
	for _, turnID := range []string{"9007199254740992", "9007199254740993"} {
		app := command.New(command.Options{
			Stdin:  bytes.NewBufferString(`{"session_id":"session-1","turn_id":` + turnID + `,"cwd":"/tmp/demo","last_assistant_message":"Done."}`),
			Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, HTTPClient: httpClient,
			Endpoint: "https://pushover.test/messages.json", DedupePath: filepath.Join(dir, "dedupe.json"),
		})
		if err := app.Run(context.Background(), []string{
			"vibe-pushover", "notify", "--config", configPath, "--agent", "codex", "--event", "turn-complete",
		}); err != nil {
			t.Fatalf("Run(turn %s) error = %v", turnID, err)
		}
	}
	if requests != 2 {
		t.Fatalf("Pushover requests = %d, want 2 for distinct numeric turn IDs", requests)
	}
}

func TestNotifyCommandDeduplicatesSameDestinationAcrossConfigPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPaths := []string{filepath.Join(dir, "first.json"), filepath.Join(dir, "second.json")}
	for _, path := range configPaths {
		if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
			t.Fatalf("Save(%q) error = %v", path, err)
		}
	}
	requests := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
	})}
	for _, path := range configPaths {
		app := command.New(command.Options{
			Stdin:  bytes.NewBufferString(`{"session_id":"session-1","turn_id":"turn-1","cwd":"/tmp/demo"}`),
			Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, HTTPClient: httpClient,
			Endpoint: "https://pushover.test/messages.json", DedupePath: filepath.Join(dir, "dedupe.json"),
		})
		if err := app.Run(context.Background(), []string{
			"vibe-pushover", "notify", "--config", path, "--agent", "codex", "--event", "turn-complete",
		}); err != nil {
			t.Fatalf("Run(%q) error = %v", path, err)
		}
	}
	if requests != 1 {
		t.Fatalf("Pushover requests = %d, want 1 for the same delivery destination", requests)
	}
}

func TestNotifyCommandDoesNotDeduplicateDifferentCredentialConfigs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPaths := []string{filepath.Join(dir, "personal.json"), filepath.Join(dir, "team.json")}
	for index, path := range configPaths {
		if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: fmt.Sprintf("user-%d", index)}); err != nil {
			t.Fatalf("Save(%q) error = %v", path, err)
		}
	}
	requests := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
	})}
	for _, path := range configPaths {
		app := command.New(command.Options{
			Stdin:  bytes.NewBufferString(`{"session_id":"session-1","turn_id":"turn-1","cwd":"/tmp/demo"}`),
			Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, HTTPClient: httpClient,
			Endpoint: "https://pushover.test/messages.json", DedupePath: filepath.Join(dir, "dedupe.json"),
		})
		if err := app.Run(context.Background(), []string{
			"vibe-pushover", "notify", "--config", path, "--agent", "codex", "--event", "turn-complete",
		}); err != nil {
			t.Fatalf("Run(%q) error = %v", path, err)
		}
	}
	if requests != 2 {
		t.Fatalf("Pushover requests = %d, want 2 for different credential configs", requests)
	}
}

func TestNotifyCommandAppliesConfiguredNotificationProfile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", NotificationProfile: "watch",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var priority, sound string
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		priority, sound = r.Form.Get("priority"), r.Form.Get("sound")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"status":1,"request":"request-id"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString(`{"cwd":"/tmp/demo"}`), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		HTTPClient: httpClient, Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path, "--agent", "codex", "--event", "turn-complete",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if priority != "0" || sound != "pushover" {
		t.Fatalf("sent priority=%q sound=%q, want watch profile", priority, sound)
	}
}

func TestNotifyCommandSkipsAuggieNonCompletionStop(t *testing.T) {
	t.Parallel()

	for name, payload := range map[string]string{
		"interrupted":   `{"agent_stop_cause":"interrupted","workspace_roots":["/tmp/demo"]}`,
		"missing cause": `{"workspace_roots":["/tmp/demo"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			called := false
			httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
				called = true
				return nil, errors.New("unexpected Pushover request")
			})}
			app := command.New(command.Options{
				Stdin:      bytes.NewBufferString(payload),
				Stdout:     &bytes.Buffer{},
				Stderr:     &bytes.Buffer{},
				HTTPClient: httpClient,
				Endpoint:   "https://pushover.test/messages.json",
			})
			if err := app.Run(context.Background(), []string{
				"vibe-pushover", "notify", "--agent", "auggie", "--event", "turn-complete", "--skip-non-completion",
			}); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if called {
				t.Fatal("notify sent a Pushover request for a non-completion Auggie stop")
			}
		})
	}
}

func TestNotifyCommandSkipsQwenActiveStop(t *testing.T) {
	t.Parallel()

	called := false
	app := command.New(command.Options{
		Stdin:  bytes.NewBufferString(`{"stop_hook_active":true,"cwd":"/tmp/demo"}`),
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			called = true
			return nil, errors.New("unexpected Pushover request")
		})},
		Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--agent", "qwen", "--event", "turn-complete", "--skip-active-qwen-stop",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if called {
		t.Fatal("notify sent a Pushover request for an active Qwen Stop re-entry")
	}
}

func TestNotifyCommandSkipsGenericActiveStop(t *testing.T) {
	t.Parallel()

	called := false
	app := command.New(command.Options{
		Stdin:  bytes.NewBufferString(`{"stop_hook_active":true,"cwd":"/tmp/demo"}`),
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			called = true
			return nil, errors.New("unexpected Pushover request")
		})},
		Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--agent", "qoder", "--event", "turn-complete", "--skip-active-stop",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if called {
		t.Fatal("notify sent a Pushover request for an active Stop re-entry")
	}
}

func TestNotifyCommandSkipsHermesSmartApproval(t *testing.T) {
	t.Parallel()

	called := false
	app := command.New(command.Options{
		Stdin:  bytes.NewBufferString(`{"extra":{"surface":"smart","command":"rm -rf build"}}`),
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			called = true
			return nil, errors.New("unexpected Pushover request")
		})},
		Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--agent", "hermes", "--event", "approval-required", "--skip-noninteractive-approval",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if called {
		t.Fatal("notify sent a Pushover request for a smart auto-approval decision")
	}
}

func TestPreviewCommandShowsNotificationWithoutCredentials(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin:  bytes.NewBufferString(`{"cwd":"/tmp/demo","last_assistant_message":"All tests pass.\nMore detail."}`),
		Stdout: &stdout, Stderr: &bytes.Buffer{},
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "preview", "--agent", "gemini", "--event", "turn-complete", "--profile", "watch",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"✓ Gemini finished · demo", "All tests pass.", "Priority: 0", "Sound: pushover", "TTL: 1h0m0s",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("preview output does not contain %q:\n%s", want, output)
		}
	}
}

func TestPreviewUsesProcessDirectoryWhenHookPayloadHasNoWorkspace(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin:  &bytes.Buffer{},
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "preview", "--agent", "kiro", "--event", "turn-complete",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "✓ Kiro finished · " + filepath.Base(cwd)
	if !strings.Contains(stdout.String(), want) {
		t.Fatalf("preview output does not contain %q:\n%s", want, stdout.String())
	}
}

func TestPreviewUsesProcessDirectoryWhenHookWorkspaceIsEmpty(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin:  bytes.NewBufferString(`{"cwd":"","workspace_roots":[]}`),
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "preview", "--agent", "kiro", "--event", "turn-complete",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "✓ Kiro finished · " + filepath.Base(cwd)
	if !strings.Contains(stdout.String(), want) {
		t.Fatalf("preview output does not contain %q:\n%s", want, stdout.String())
	}
}

func TestAgentsCommandShowsCapabilities(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := command.New(command.Options{Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{}})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "agents"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"aider", "amp", "auggie", "claude", "codex", "copilot", "cortex", "cursor", "droid", "gemini", "goose", "hermes", "kimi", "kiro", "omp", "opencode", "pi", "qoder", "qwen", "vscode", "windsurf",
		"completion+approval", "completion+approval+attention", "completion+attention", "completion only",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("agents output does not contain %q:\n%s", want, output)
		}
	}
}

func TestProfileCommandUpdatesExistingConfig(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{}})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "profile", "--config", path, "quiet"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.NotificationProfile != "quiet" || got.AppToken != "app-token" || got.UserKey != "user-key" {
		t.Fatalf("updated config = %#v", got)
	}
	if !strings.Contains(stdout.String(), "quiet") {
		t.Fatalf("output = %q", stdout.String())
	}
}

func TestNotifyCommandSendsKimiStopFallback(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var title, message string
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		title = r.Form.Get("title")
		message = r.Form.Get("message")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"status":1,"request":"request-id"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	app := command.New(command.Options{
		Stdin:      bytes.NewBufferString(`{"hook_event_name":"Stop","cwd":"/tmp/demo","stop_hook_active":false}`),
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		HTTPClient: httpClient,
		Endpoint:   "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify",
		"--config", path,
		"--agent", "kimi",
		"--event", "turn-complete",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if title != "✓ Kimi finished · demo" {
		t.Fatalf("title = %q", title)
	}
	if message != "Turn completed." {
		t.Fatalf("message = %q, want fallback body", message)
	}
}

func TestNotifyCommandAcceptsLegacySkipActiveStopFlag(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	requests := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"status":1,"request":"request-id"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	app := command.New(command.Options{
		Stdin:      bytes.NewBufferString(`{"hook_event_name":"Stop","stop_hook_active":true}`),
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		HTTPClient: httpClient,
		Endpoint:   "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path, "--agent", "kimi",
		"--event", "turn-complete", "--skip-active-stop",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestInstallCommandWiresCustomPushoverConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	agentConfig := filepath.Join(dir, "hooks.json")
	pushoverConfig := filepath.Join(dir, "pushover config.json")
	app := command.New(command.Options{
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		Executable: "/opt/bin/vibe-pushover",
	})
	err := app.Run(context.Background(), []string{
		"vibe-pushover", "install",
		"--agent", "codex",
		"--agent-config", agentConfig,
		"--config", pushoverConfig,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	data, err := os.ReadFile(agentConfig)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Contains(data, []byte(`--config '`+pushoverConfig+`'`)) {
		t.Fatalf("installed hook does not use custom Pushover config: %s", data)
	}
}

func TestInstallCommandCreatesPiExtension(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	extensionPath := filepath.Join(dir, "extensions", "vibe-pushover", "index.ts")
	app := command.New(command.Options{
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		Executable: "/opt/bin/vibe-pushover",
	})
	err := app.Run(context.Background(), []string{
		"vibe-pushover", "install",
		"--agent", "pi",
		"--agent-config", extensionPath,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	data, err := os.ReadFile(extensionPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Contains(data, []byte(`pi.on("agent_settled"`)) {
		t.Fatalf("Pi extension does not register agent_settled: %s", data)
	}
}

func TestInstallCommandCreatesKimiHooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	app := command.New(command.Options{
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		Executable: "/opt/bin/vibe-pushover",
	})
	err := app.Run(context.Background(), []string{
		"vibe-pushover", "install",
		"--agent", "KIMI",
		"--agent-config", path,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{`event = "Stop"`, `event = "PermissionRequest"`, `--agent kimi`} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("Kimi config does not contain %q: %s", want, data)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
