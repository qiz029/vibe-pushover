package command_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestAgentsCommandShowsCapabilities(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := command.New(command.Options{Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{}})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "agents"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"auggie", "claude", "codex", "copilot", "cursor", "droid", "gemini", "goose", "kimi", "opencode", "pi", "qwen", "vscode", "windsurf",
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
