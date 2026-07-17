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

func TestSetupCommandStoresUrgentNotificationProfile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString("app-token\nuser-key\nurgent\n\n"), Stdout: &bytes.Buffer{},
	})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "setup", "--config", path}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.NotificationProfile != "urgent" {
		t.Fatalf("NotificationProfile = %q, want urgent", got.NotificationProfile)
	}
}

func TestTestCommandSendsConfiguredApprovalExperience(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", Device: "iphone", NotificationProfile: "watch",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var sent map[string]string
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		sent = map[string]string{
			"device": r.Form.Get("device"), "title": r.Form.Get("title"), "message": r.Form.Get("message"),
			"priority": r.Form.Get("priority"), "sound": r.Form.Get("sound"), "ttl": r.Form.Get("ttl"),
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"status":1,"request":"request-id"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{}, HTTPClient: httpClient,
		Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "test", "--config", path}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sent["device"] != "iphone" || sent["priority"] != "1" || sent["sound"] != "persistent" || sent["ttl"] != "1800" {
		t.Fatalf("sent form = %#v, want configured device and approval delivery style", sent)
	}
	if !strings.Contains(sent["title"], "vibe-pushover") || sent["message"] != "Test notification delivered successfully." {
		t.Fatalf("sent title/body = %q / %q", sent["title"], sent["message"])
	}
	if !strings.Contains(stdout.String(), "approval-required") {
		t.Fatalf("output = %q, want tested event", stdout.String())
	}
}

func TestTestCommandExplainsUrgentCompletionSuppression(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", NotificationProfile: "urgent",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	called := false
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			called = true
			return nil, errors.New("unexpected Pushover request")
		})},
		Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "test", "--config", path, "--event", "turn-complete",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if called {
		t.Fatal("test sent a completion notification suppressed by urgent profile")
	}
	if got := strings.TrimSpace(stdout.String()); got != "Test turn-complete notification suppressed by urgent profile" {
		t.Fatalf("output = %q", got)
	}
}

func TestTestCommandCanForceDeliveryWhileSnoozed(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", SnoozedUntil: "2026-07-17T14:00:00Z",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	requests := 0
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin:  &bytes.Buffer{},
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
		Now:    func() time.Time { return now },
		HTTPClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"status":1,"request":"request-id"}`)),
				Header:     make(http.Header),
			}, nil
		})},
		Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "test", "--config", path,
	}); err != nil {
		t.Fatalf("regular test Run() error = %v", err)
	}
	if requests != 0 || !strings.Contains(stdout.String(), "suppressed while notifications are snoozed") {
		t.Fatalf("regular test requests = %d, output = %q", requests, stdout.String())
	}
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "test", "--config", path, "--force",
	}); err != nil {
		t.Fatalf("forced test Run() error = %v", err)
	}
	if requests != 1 || !strings.Contains(stdout.String(), "Test approval-required notification sent") {
		t.Fatalf("forced test requests = %d, output = %q", requests, stdout.String())
	}
}

func TestTestCommandSupportsCompletionAndAttentionStyles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, event, profile, wantPriority, wantSound, wantTTL string
	}{
		{name: "watch completion", event: "turn-complete", profile: "watch", wantPriority: "0", wantSound: "pushover", wantTTL: "3600"},
		{name: "quiet attention", event: "attention-required", profile: "quiet", wantPriority: "0", wantSound: "none", wantTTL: "1800"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "config.json")
			if err := config.Save(path, config.Credentials{
				AppToken: "app-token", UserKey: "user-key", NotificationProfile: tt.profile,
			}); err != nil {
				t.Fatalf("Save() error = %v", err)
			}
			var priority, sound, ttl, body string
			httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if err := r.ParseForm(); err != nil {
					t.Fatalf("ParseForm() error = %v", err)
				}
				priority, sound, ttl, body = r.Form.Get("priority"), r.Form.Get("sound"), r.Form.Get("ttl"), r.Form.Get("message")
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"status":1,"request":"request-id"}`)),
					Header:     make(http.Header),
				}, nil
			})}
			app := command.New(command.Options{
				Stdin: &bytes.Buffer{}, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, HTTPClient: httpClient,
				Endpoint: "https://pushover.test/messages.json",
			})
			if err := app.Run(context.Background(), []string{
				"vibe-pushover", "test", "--config", path, "--event", tt.event, "--message", "Custom test body.",
			}); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if priority != tt.wantPriority || sound != tt.wantSound || ttl != tt.wantTTL || body != "Custom test body." {
				t.Fatalf("priority=%q sound=%q ttl=%q body=%q", priority, sound, ttl, body)
			}
		})
	}
}

func TestSetupCommandStoresInteractiveTargetDevices(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString("app-token\nuser-key\nbalanced\niphone,ipad\n"), Stdout: &bytes.Buffer{},
	})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "setup", "--config", path}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Device != "iphone,ipad" {
		t.Fatalf("Device = %q, want iphone,ipad", got.Device)
	}
}

func TestSetupCommandNormalizesAllTargetDevicesToBroadcast(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString("app-token\nuser-key\nbalanced\nall\n"), Stdout: &bytes.Buffer{},
	})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "setup", "--config", path}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Device != "" {
		t.Fatalf("Device = %q, want empty broadcast target", got.Device)
	}
}

func TestDeviceCommandShowsSetsAndClearsTarget(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{Stdin: &bytes.Buffer{}, Stdout: &stdout})

	if err := app.Run(context.Background(), []string{"vibe-pushover", "device", "--config", path}); err != nil {
		t.Fatalf("show Run() error = %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "all" {
		t.Fatalf("show output = %q, want all", got)
	}
	stdout.Reset()
	if err := app.Run(context.Background(), []string{"vibe-pushover", "device", "iphone,ipad", "--config", path}); err != nil {
		t.Fatalf("set Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Device != "iphone,ipad" {
		t.Fatalf("Device after set = %q, want iphone,ipad", got.Device)
	}
	stdout.Reset()
	if err := app.Run(context.Background(), []string{"vibe-pushover", "device", "all", "--config", path}); err != nil {
		t.Fatalf("clear Run() error = %v", err)
	}
	got, err = config.Load(path)
	if err != nil {
		t.Fatalf("Load() after clear error = %v", err)
	}
	if got.Device != "" {
		t.Fatalf("Device after clear = %q, want empty", got.Device)
	}
}

func TestNotifyCommandSendsHookPayload(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key", Device: "iphone"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var received map[string]string
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm() error = %v", err)
		}
		received = map[string]string{
			"device":    r.Form.Get("device"),
			"title":     r.Form.Get("title"),
			"message":   r.Form.Get("message"),
			"priority":  r.Form.Get("priority"),
			"sound":     r.Form.Get("sound"),
			"ttl":       r.Form.Get("ttl"),
			"url":       r.Form.Get("url"),
			"url_title": r.Form.Get("url_title"),
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"status":1,"request":"request-id"}`)),
			Header:     make(http.Header),
		}, nil
	})}

	app := command.New(command.Options{
		Stdin:      bytes.NewBufferString(`{"cwd":"/tmp/demo","tool_name":"Bash","tool_input":{"command":"make deploy"},"session_url":"https://example.com/agent/42"}`),
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
	if got["url"] != "https://example.com/agent/42" || got["url_title"] != "Open agent" {
		t.Fatalf("supplementary action = %q (%q)", got["url"], got["url_title"])
	}
	if got["device"] != "iphone" {
		t.Fatalf("device = %q, want iphone", got["device"])
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

func TestNotifyCommandDoesNotDedupeAcrossTargetDevices(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	requests := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
	})}
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	run := func(device string) {
		t.Helper()
		if err := config.Save(configPath, config.Credentials{AppToken: "app-token", UserKey: "user-key", Device: device}); err != nil {
			t.Fatalf("Save(%q) error = %v", device, err)
		}
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
			t.Fatalf("Run(%q) error = %v", device, err)
		}
	}

	run("iphone")
	run("ipad")
	if requests != 2 {
		t.Fatalf("Pushover requests = %d, want 2 for different target devices", requests)
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

func TestNotifyCommandSuppressesCompletionForUrgentProfile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"app_token":"app-token","user_key":"user-key","notification_profile":"urgent"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	called := false
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("unexpected Pushover request")
	})}
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString(`{"cwd":"/tmp/demo"}`), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		HTTPClient: httpClient, Endpoint: "https://pushover.test/messages.json", DedupePath: filepath.Join(dir, "dedupe.json"),
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path, "--agent", "codex", "--event", "turn-complete",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if called {
		t.Fatal("notify sent a completion notification for urgent profile")
	}
}

func TestNotifyCommandUrgentProfilePreservesActionableDelivery(t *testing.T) {
	t.Parallel()

	for _, event := range []string{"approval-required", "attention-required"} {
		event := event
		t.Run(event, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			if err := config.Save(path, config.Credentials{
				AppToken: "app-token", UserKey: "user-key", NotificationProfile: "urgent",
			}); err != nil {
				t.Fatalf("Save() error = %v", err)
			}
			var requests int
			var priority, sound string
			httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests++
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
				Stdin:  bytes.NewBufferString(`{"cwd":"/tmp/demo","message":"User action required."}`),
				Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, HTTPClient: httpClient,
				Endpoint: "https://pushover.test/messages.json", DedupePath: filepath.Join(dir, "dedupe.json"),
			})
			if err := app.Run(context.Background(), []string{
				"vibe-pushover", "notify", "--config", path, "--agent", "mimo", "--event", event,
			}); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if requests != 1 || priority != "1" || sound != "persistent" {
				t.Fatalf("requests=%d priority=%q sound=%q, want one high-priority persistent notification", requests, priority, sound)
			}
		})
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

func TestNotifyCommandSkipsMistralSubagentTurn(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	requests := 0
	app := command.New(command.Options{
		Stdin:  bytes.NewBufferString(`{"session_id":"child","parent_session_id":"parent","transcript_path":"/tmp/session/agents/reviewer/messages.jsonl","cwd":"/tmp/demo"}`),
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
		})},
		Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path, "--agent", "mistral", "--event", "turn-complete", "--skip-mistral-subagent",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if requests != 0 {
		t.Fatalf("Pushover requests = %d, want none for a subagent turn", requests)
	}
}

func TestNotifyCommandDoesNotSkipMistralForkedTopLevelTurn(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	requests := 0
	app := command.New(command.Options{
		Stdin:  bytes.NewBufferString(`{"session_id":"fork","parent_session_id":"original","transcript_path":"/tmp/session/messages.jsonl","cwd":"/tmp/demo"}`),
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
		})},
		Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path, "--agent", "mistral", "--event", "turn-complete", "--skip-mistral-subagent",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("Pushover requests = %d, want one for a forked top-level turn", requests)
	}
}

func TestNotifyCommandDoesNotSkipMistralUnrelatedAgentsDirectory(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	requests := 0
	app := command.New(command.Options{
		Stdin:  bytes.NewBufferString(`{"session_id":"fork","parent_session_id":"original","transcript_path":"/tmp/agents/archive/session/messages.jsonl","cwd":"/tmp/demo"}`),
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
		})},
		Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path, "--agent", "mistral", "--event", "turn-complete", "--skip-mistral-subagent",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("Pushover requests = %d, want one for an unrelated agents directory", requests)
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
		Stdin:  bytes.NewBufferString(`{"cwd":"/tmp/demo","last_assistant_message":"All tests pass.\nMore detail.","session_url":"https://example.com/agent/42"}`),
		Stdout: &stdout, Stderr: &bytes.Buffer{},
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "preview", "--agent", "gemini", "--event", "turn-complete", "--profile", "watch",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"✓ Gemini finished · demo", "All tests pass.", "Priority: 0", "Sound: pushover", "TTL: 1h0m0s", "Action: Open result (https://example.com/agent/42)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("preview output does not contain %q:\n%s", want, output)
		}
	}
}

func TestPreviewCommandShowsUrgentProfileCompletionIsSuppressed(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString(`{"cwd":"/tmp/demo"}`), Stdout: &stdout, Stderr: &bytes.Buffer{},
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "preview", "--agent", "codex", "--event", "turn-complete", "--profile", "urgent",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "Delivery: suppressed by urgent profile" {
		t.Fatalf("preview output = %q", got)
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
		"aider", "amp", "auggie", "claude", "cline", "codex", "copilot", "cortex", "cursor", "droid", "gemini", "goose", "grok", "hermes", "kimi", "kiro", "mimo", "mistral", "omp", "opencode", "pi", "qoder", "qwen", "trae", "vscode", "windsurf",
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
	if err := app.Run(context.Background(), []string{"vibe-pushover", "profile", "--config", path, "urgent"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.NotificationProfile != "urgent" || got.AppToken != "app-token" || got.UserKey != "user-key" {
		t.Fatalf("updated config = %#v", got)
	}
	if !strings.Contains(stdout.String(), "urgent") {
		t.Fatalf("output = %q", stdout.String())
	}
}

func TestSnoozeCommandTemporarilySuppressesNotifications(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	requests := 0
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin:  bytes.NewBufferString(`{"cwd":"/tmp/demo","last_assistant_message":"Done."}`),
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
		Now:    func() time.Time { return now },
		HTTPClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"status":1,"request":"request-id"}`)),
				Header:     make(http.Header),
			}, nil
		})},
		Endpoint:   "https://pushover.test/messages.json",
		DedupePath: filepath.Join(t.TempDir(), "dedupe.json"),
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "snooze", "--config", path, "45m",
	}); err != nil {
		t.Fatalf("snooze Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.SnoozedUntil != "2026-07-17T12:45:00Z" {
		t.Fatalf("SnoozedUntil = %q", got.SnoozedUntil)
	}
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path,
		"--agent", "codex", "--event", "turn-complete",
	}); err != nil {
		t.Fatalf("notify Run() error = %v", err)
	}
	if requests != 0 {
		t.Fatalf("Pushover requests = %d, want 0 while snoozed", requests)
	}
	if !strings.Contains(stdout.String(), "Notifications snoozed until 2026-07-17 12:45 UTC") {
		t.Fatalf("snooze output = %q", stdout.String())
	}
}

func TestSnoozeCommandShowsStatusAndResumes(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", SnoozedUntil: "2026-07-17T14:00:00Z",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin:  &bytes.Buffer{},
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
		Now: func() time.Time {
			return time.Date(2026, time.July, 17, 6, 0, 0, 0, time.FixedZone("PDT", -7*60*60))
		},
	})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "snooze", "--config", path}); err != nil {
		t.Fatalf("status Run() error = %v", err)
	}
	if err := app.Run(context.Background(), []string{"vibe-pushover", "snooze", "--config", path, "off"}); err != nil {
		t.Fatalf("resume Run() error = %v", err)
	}
	if err := app.Run(context.Background(), []string{"vibe-pushover", "snooze", "--config", path}); err != nil {
		t.Fatalf("active status Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.SnoozedUntil != "" {
		t.Fatalf("SnoozedUntil = %q, want cleared", got.SnoozedUntil)
	}
	for _, want := range []string{
		"Notifications snoozed until 2026-07-17 07:00 PDT",
		"Notifications resumed",
		"Notifications are active",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("snooze output does not contain %q:\n%s", want, stdout.String())
		}
	}
}

func TestSnoozeCommandPreservesSubsecondDeadline(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	app := command.New(command.Options{
		Stdin:  &bytes.Buffer{},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		Now: func() time.Time {
			return time.Date(2026, time.July, 17, 12, 0, 0, 900_000_000, time.UTC)
		},
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "snooze", "--config", path, "1s",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.SnoozedUntil != "2026-07-17T12:00:01.9Z" {
		t.Fatalf("SnoozedUntil = %q", got.SnoozedUntil)
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

func TestNotifyCommandSkipsClineSubagentCompletion(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	requests := 0
	app := command.New(command.Options{
		Stdin:  bytes.NewBufferString(`{"hookName":"agent_end","parent_agent_id":"parent-agent","turn":{"outputText":"Child done."}}`),
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"status":1,"request":"request-id"}`)),
				Header:     make(http.Header),
			}, nil
		})},
		Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path,
		"--agent", "cline", "--event", "turn-complete", "--skip-cline-subagent",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if requests != 0 {
		t.Fatalf("Pushover requests = %d, want 0 for Cline subagent", requests)
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

func TestInstallCommandCreatesSharedClineHookForIDEAndCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
		Executable: "/opt/bin/vibe-pushover",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "install", "--agent", "cline",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	path := filepath.Join(home, "Documents", "Cline", "Hooks", "TaskComplete")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	for _, want := range []string{
		"# Generated by vibe-pushover for Cline.",
		"'/opt/bin/vibe-pushover' notify --agent cline --event turn-complete --ignore-errors",
		`{"cancel":false,"contextModification":"","errorMessage":""}`,
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("Cline hook %q does not contain %q:\n%s", path, want, data)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("Cline hook %q mode = %o, want executable", path, info.Mode().Perm())
	}
	if !strings.Contains(stdout.String(), "Documents/Cline/Hooks/TaskComplete") {
		t.Fatalf("install output does not list the shared Cline hook:\n%s", stdout.String())
	}
	duplicatePath := filepath.Join(home, ".cline", "hooks", "TaskComplete")
	if _, err := os.Stat(duplicatePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("install created a duplicate CLI hook: %v", err)
	}
	stdout.Reset()
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "install", "--agent", "cline",
	}); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "already installed") {
		t.Fatalf("second install output = %q", stdout.String())
	}
	dataAfter, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(after second install) error = %v", err)
	}
	if !bytes.Equal(dataAfter, data) {
		t.Fatalf("second install changed Cline hook:\n%s", dataAfter)
	}
}

func TestInstallCommandRefusesUnownedClineHook(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	hook := filepath.Join(home, "Documents", "Cline", "Hooks", "TaskComplete")
	if err := os.MkdirAll(filepath.Dir(hook), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	original := []byte("#!/bin/sh\necho personal-cline-hook\n")
	if err := os.WriteFile(hook, original, 0o700); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	app := command.New(command.Options{
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		Executable: "/opt/bin/vibe-pushover",
	})
	err := app.Run(context.Background(), []string{
		"vibe-pushover", "install", "--agent", "cline",
	})
	if err == nil || !strings.Contains(err.Error(), "not owned by vibe-pushover") {
		t.Fatalf("Run() error = %v, want ownership refusal", err)
	}
	got, err := os.ReadFile(hook)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("personal Cline hook changed:\n%s", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
