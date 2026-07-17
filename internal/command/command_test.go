package command_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qiz029/vibe-pushover/internal/command"
	"github.com/qiz029/vibe-pushover/internal/config"
	"github.com/urfave/cli/v3"
)

func clearAgentDetectionOverrides(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"AUTOHAND_CONFIG", "CLINE_DIR", "CODEWHALE_CONFIG_PATH", "DEEPSEEK_CONFIG_PATH", "CODEWHALE_HOME",
		"COPILOT_HOME", "CRAFT_CONFIG_DIR", "GEMINI_CLI_HOME", "GJC_CODING_AGENT_DIR", "GJC_CONFIG_DIR",
		"GROK_HOME", "HERMES_HOME", "KIMI_CODE_HOME", "MIMOCODE_HOME", "PI_CODING_AGENT_DIR", "VIBE_HOME",
	} {
		t.Setenv(name, "")
	}
}

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
	if !reflect.DeepEqual(got, want) {
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

func TestSetupCommandStoresOnCallNotificationProfile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString("app-token\nuser-key\non-call\n\n\n"), Stdout: &stdout,
	})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "setup", "--config", path}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.NotificationProfile != "on-call" {
		t.Fatalf("NotificationProfile = %q, want on-call", got.NotificationProfile)
	}
	if want := "Approval and attention notifications will repeat every 1m0s for up to 15m0s or until acknowledged."; !strings.Contains(stdout.String(), want) {
		t.Fatalf("setup output does not contain %q:\n%s", want, stdout.String())
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

func TestTestCommandWarnsThatOnCallNotificationRepeatsUntilAcknowledged(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", NotificationProfile: "on-call",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"status":1,"request":"request-id","receipt":"receipt-id"}`)),
				Header:     make(http.Header),
			}, nil
		})},
		Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "test", "--config", path}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{
		"Test approval-required notification sent",
		"Emergency notification repeats every 1m0s for up to 15m0s or until acknowledged.",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("test output does not contain %q:\n%s", want, stdout.String())
		}
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

func TestTestCommandCanForceCompletionDeliveryDuringFocus(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", FocusUntil: "2026-07-17T14:00:00Z",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	requests := 0
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{}, Now: func() time.Time { return now },
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
	args := []string{"vibe-pushover", "test", "--config", path, "--event", "turn-complete"}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("regular test Run() error = %v", err)
	}
	if requests != 0 || !strings.Contains(stdout.String(), "suppressed while focus mode is active") {
		t.Fatalf("regular test requests = %d, output = %q", requests, stdout.String())
	}
	if err := app.Run(context.Background(), append(args, "--force")); err != nil {
		t.Fatalf("forced test Run() error = %v", err)
	}
	if requests != 1 || !strings.Contains(stdout.String(), "Test turn-complete notification sent") {
		t.Fatalf("forced test requests = %d, output = %q", requests, stdout.String())
	}
}

func TestTestCommandReportsSilenceRuleSuppressionAndSupportsForce(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key",
		SilenceRules: []config.SilenceRule{{Agent: "vibe-pushover", Event: "turn-complete"}},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	requests := 0
	stdout := &bytes.Buffer{}
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: stdout, Stderr: &bytes.Buffer{},
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
	args := []string{"vibe-pushover", "test", "--config", path, "--event", "turn-complete"}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("regular test Run() error = %v", err)
	}
	if requests != 0 || !strings.Contains(stdout.String(), "suppressed by a matching silence rule") {
		t.Fatalf("regular test requests = %d, output = %q", requests, stdout.String())
	}
	if err := app.Run(context.Background(), append(args, "--force")); err != nil {
		t.Fatalf("forced test Run() error = %v", err)
	}
	if requests != 1 || !strings.Contains(stdout.String(), "Test turn-complete notification sent") {
		t.Fatalf("forced test requests = %d, output = %q", requests, stdout.String())
	}
}

func TestTestCommandRejectsInvalidEventBeforeSilenceSuppression(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key",
		SilenceRules: []config.SilenceRule{{Agent: "vibe-pushover", Event: "all"}},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	app := command.New(command.Options{Stdin: &bytes.Buffer{}, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	err := app.Run(context.Background(), []string{
		"vibe-pushover", "test", "--config", path, "--event", "typo",
	})
	if err == nil || !strings.Contains(err.Error(), "event must be") {
		t.Fatalf("Run() error = %v, want event validation error", err)
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

func TestTestCommandAppliesConfiguredMinimalDetail(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", NotificationDetail: "minimal",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var body string
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			body = r.Form.Get("message")
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
		})},
		Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "test", "--config", path, "--event", "approval-required", "--message", "Sensitive test detail.",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if body != "Approval requested." {
		t.Fatalf("test notification body = %q, want minimal body", body)
	}
}

func TestTestCommandUsesConfiguredEventSound(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", TurnCompleteSound: "magic",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var sound string
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			sound = r.Form.Get("sound")
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
		})},
		Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "test", "--config", path, "--event", "turn-complete",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sound != "magic" {
		t.Fatalf("sound = %q, want magic", sound)
	}
}

func TestConfiguredEventSoundCanUseAccountDefaultButQuietStaysSilent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, preference, profile, wantSound string
	}{
		{name: "account default", preference: "default", wantSound: ""},
		{name: "quiet wins", preference: "magic", profile: "quiet", wantSound: "none"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "config.json")
			if err := config.Save(path, config.Credentials{
				AppToken: "app-token", UserKey: "user-key", TurnCompleteSound: tt.preference, NotificationProfile: tt.profile,
			}); err != nil {
				t.Fatalf("Save() error = %v", err)
			}
			var sound string
			app := command.New(command.Options{
				Stdin: &bytes.Buffer{}, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
				HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					if err := r.ParseForm(); err != nil {
						t.Fatalf("ParseForm() error = %v", err)
					}
					sound = r.Form.Get("sound")
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
				})},
				Endpoint: "https://pushover.test/messages.json",
			})
			if err := app.Run(context.Background(), []string{
				"vibe-pushover", "test", "--config", path, "--event", "turn-complete",
			}); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if sound != tt.wantSound {
				t.Fatalf("sound = %q, want %q", sound, tt.wantSound)
			}
		})
	}
}

func TestSetupCommandStoresInteractiveTargetDevices(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString("app-token\nuser-key\nbalanced\nsummary\niphone,ipad\n"), Stdout: &bytes.Buffer{},
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

func TestSetupCommandStoresInteractiveNotificationDetail(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString("app-token\nuser-key\nbalanced\nminimal\nall\n"), Stdout: &bytes.Buffer{},
	})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "setup", "--config", path}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.NotificationDetail != "minimal" {
		t.Fatalf("NotificationDetail = %q, want minimal", got.NotificationDetail)
	}
}

func TestSetupCommandStoresInteractivePrivateNotificationDetail(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString("app-token\nuser-key\nbalanced\nprivate\nall\n"), Stdout: &bytes.Buffer{},
	})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "setup", "--config", path}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.NotificationDetail != "private" {
		t.Fatalf("NotificationDetail = %q, want private", got.NotificationDetail)
	}
}

func TestSetupCommandNormalizesAllTargetDevicesToBroadcast(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString("app-token\nuser-key\nbalanced\nsummary\nall\n"), Stdout: &bytes.Buffer{},
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

func TestDetailCommandShowsSetsAndResetsNotificationDetail(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{}})

	if err := app.Run(context.Background(), []string{"vibe-pushover", "detail", "--config", path}); err != nil {
		t.Fatalf("show Run() error = %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "summary" {
		t.Fatalf("show output = %q, want summary", got)
	}
	stdout.Reset()
	if err := app.Run(context.Background(), []string{"vibe-pushover", "detail", "minimal", "--config", path}); err != nil {
		t.Fatalf("set Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.NotificationDetail != "minimal" {
		t.Fatalf("NotificationDetail after set = %q, want minimal", got.NotificationDetail)
	}
	stdout.Reset()
	if err := app.Run(context.Background(), []string{"vibe-pushover", "detail", "summary", "--config", path}); err != nil {
		t.Fatalf("reset Run() error = %v", err)
	}
	got, err = config.Load(path)
	if err != nil {
		t.Fatalf("Load() after reset error = %v", err)
	}
	if got.NotificationDetail != "" {
		t.Fatalf("NotificationDetail after reset = %q, want empty default", got.NotificationDetail)
	}
}

func TestStatusCommandSummarizesDeliveryControlsWithoutSecrets(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken:            "secret-app-token",
		UserKey:             "secret-user-key",
		Device:              "iphone,ipad",
		NotificationProfile: "watch",
		NotificationDetail:  "minimal",
		SnoozedUntil:        "2026-07-18T07:00:00Z",
		FocusUntil:          "2026-07-18T08:00:00Z",
		QuietHoursStart:     "22:00",
		QuietHoursEnd:       "08:00",
		TurnCompleteSound:   "magic",
		ApprovalSound:       "default",
		SilenceRules: []config.SilenceRule{
			{Agent: "codex", Event: "turn-complete"},
			{Project: "private", Event: "all"},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{},
		Now: func() time.Time { return time.Date(2026, time.July, 17, 23, 0, 0, 0, time.FixedZone("PDT", -7*60*60)) },
	})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "status", "--config", path}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := strings.Join([]string{
		"Profile: watch",
		"Detail: minimal",
		"Device target: iphone,ipad",
		"End-to-end encryption: off",
		"Snooze: active until 2026-07-18 00:00 PDT",
		"Focus: active until 2026-07-18 01:00 PDT",
		"Quiet hours: 22:00-08:00 (active now)",
		"Silence rules: 2",
		"Sounds: turn-complete=magic approval-required=default attention-required=persistent",
	}, "\n")
	if got := strings.TrimSpace(stdout.String()); got != want {
		t.Fatalf("status output =\n%s\nwant =\n%s", got, want)
	}
	if strings.Contains(stdout.String(), "secret-app-token") || strings.Contains(stdout.String(), "secret-user-key") {
		t.Fatalf("status output leaked credentials: %s", stdout.String())
	}
}

func TestStatusCommandShowsDefaultActiveDeliveryState(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{}})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "status", "--config", path}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{
		"Profile: balanced", "Detail: summary", "Device target: all", "End-to-end encryption: off", "Snooze: off", "Focus: off", "Quiet hours: off", "Silence rules: 0",
		"Sounds: turn-complete=none approval-required=persistent attention-required=persistent",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("status output does not contain %q:\n%s", want, stdout.String())
		}
	}
}

func TestEncryptionCommandGeneratesKeyAndStatusDoesNotLeakIt(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin:  &bytes.Buffer{},
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
		Random: bytes.NewReader(bytes.Repeat([]byte{0x42}, 32)),
	})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "encryption", "enable", "--config", path}); err != nil {
		t.Fatalf("enable Run() error = %v", err)
	}
	key := strings.Repeat("42", 32)
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.EncryptionKey != key {
		t.Fatalf("EncryptionKey = %q, want generated key", got.EncryptionKey)
	}
	if output := stdout.String(); !strings.Contains(output, key) || !strings.Contains(output, "shown once") || !strings.Contains(output, "every target iOS/Android device") {
		t.Fatalf("enable output lacks one-time setup guidance:\n%s", output)
	}

	stdout.Reset()
	if err := app.Run(context.Background(), []string{"vibe-pushover", "status", "--config", path}); err != nil {
		t.Fatalf("status Run() error = %v", err)
	}
	if output := stdout.String(); !strings.Contains(output, "End-to-end encryption: on") || strings.Contains(output, key) {
		t.Fatalf("status output = %q, want enabled state without key", output)
	}
}

func TestEncryptionCommandInteractivelySetsExistingKeyWithoutEcho(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	key := strings.Repeat("AB", 32)
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin: strings.NewReader(key + "\n"), Stdout: &stdout, Stderr: &bytes.Buffer{},
	})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "encryption", "set", "--config", path}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.EncryptionKey != strings.ToLower(key) {
		t.Fatalf("EncryptionKey = %q, want normalized supplied key", got.EncryptionKey)
	}
	if strings.Contains(stdout.String(), key) || strings.Contains(stdout.String(), strings.ToLower(key)) {
		t.Fatalf("encryption set output leaked key: %q", stdout.String())
	}
}

func TestEncryptionCommandRotatesAndDisablesKey(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", EncryptionKey: strings.Repeat("42", 32),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{},
		Random: bytes.NewReader(bytes.Repeat([]byte{0x24}, 32)),
	})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "encryption", "rotate", "--config", path}); err != nil {
		t.Fatalf("rotate Run() error = %v", err)
	}
	rotated := strings.Repeat("24", 32)
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load(rotated) error = %v", err)
	}
	if got.EncryptionKey != rotated || !strings.Contains(stdout.String(), rotated) {
		t.Fatalf("rotate result key=%q output=%q", got.EncryptionKey, stdout.String())
	}

	stdout.Reset()
	if err := app.Run(context.Background(), []string{"vibe-pushover", "encryption", "disable", "--config", path}); err != nil {
		t.Fatalf("disable Run() error = %v", err)
	}
	got, err = config.Load(path)
	if err != nil {
		t.Fatalf("Load(disabled) error = %v", err)
	}
	if got.EncryptionKey != "" || !strings.Contains(stdout.String(), "disabled") {
		t.Fatalf("disable result key=%q output=%q", got.EncryptionKey, stdout.String())
	}
}

func TestEncryptionCommandRollsBackRotationWhenKeyCannotBeDisplayed(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	originalKey := strings.Repeat("42", 32)
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", EncryptionKey: originalKey,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: errorWriter{}, Stderr: &bytes.Buffer{},
		Random: bytes.NewReader(bytes.Repeat([]byte{0x24}, 32)),
	})
	err := app.Run(context.Background(), []string{"vibe-pushover", "encryption", "rotate", "--config", path})
	if err == nil || !strings.Contains(err.Error(), "display Pushover encryption key") {
		t.Fatalf("Run() error = %v, want display failure", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.EncryptionKey != originalKey {
		t.Fatalf("EncryptionKey = %q, want original key after output failure", got.EncryptionKey)
	}
}

func TestStatusCommandShowsProfileAdjustedEffectiveSounds(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", NotificationProfile: "quiet",
		TurnCompleteSound: "magic", ApprovalSound: "persistent", AttentionSound: "incoming",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{}})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "status", "--config", path}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if want := "Sounds: turn-complete=none approval-required=none attention-required=none"; !strings.Contains(stdout.String(), want) {
		t.Fatalf("status output does not contain %q:\n%s", want, stdout.String())
	}
}

func TestStatusCommandShowsProfileSuppressedEvents(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", NotificationProfile: "urgent",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{}})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "status", "--config", path}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if want := "Sounds: turn-complete=suppressed approval-required=persistent attention-required=persistent"; !strings.Contains(stdout.String(), want) {
		t.Fatalf("status output does not contain %q:\n%s", want, stdout.String())
	}
}

func TestStatusCommandShowsOnCallEmergencyRetrySchedule(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", NotificationProfile: "on-call",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{}})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "status", "--config", path}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if want := "Emergency retries: every 1m0s for up to 15m0s or until acknowledged"; !strings.Contains(stdout.String(), want) {
		t.Fatalf("status output does not contain %q:\n%s", want, stdout.String())
	}
}

func TestSoundCommandShowsSetsDefaultsAndResetsEventSounds(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{}})

	if err := app.Run(context.Background(), []string{"vibe-pushover", "sound", "--config", path}); err != nil {
		t.Fatalf("show Run() error = %v", err)
	}
	for _, want := range []string{"turn-complete: none", "approval-required: persistent", "attention-required: persistent"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("sound output does not contain %q:\n%s", want, stdout.String())
		}
	}

	stdout.Reset()
	if err := app.Run(context.Background(), []string{"vibe-pushover", "sound", "turn-complete", "magic", "--config", path}); err != nil {
		t.Fatalf("set Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.TurnCompleteSound != "magic" || !strings.Contains(stdout.String(), "turn-complete sound set to magic") {
		t.Fatalf("TurnCompleteSound=%q output=%q", got.TurnCompleteSound, stdout.String())
	}

	stdout.Reset()
	if err := app.Run(context.Background(), []string{"vibe-pushover", "sound", "turn-complete", "default", "--config", path}); err != nil {
		t.Fatalf("default Run() error = %v", err)
	}
	got, err = config.Load(path)
	if err != nil {
		t.Fatalf("Load(default) error = %v", err)
	}
	if got.TurnCompleteSound != "default" {
		t.Fatalf("TurnCompleteSound after default = %q", got.TurnCompleteSound)
	}

	stdout.Reset()
	if err := app.Run(context.Background(), []string{"vibe-pushover", "sound", "turn-complete", "reset", "--config", path}); err != nil {
		t.Fatalf("reset Run() error = %v", err)
	}
	got, err = config.Load(path)
	if err != nil {
		t.Fatalf("Load(reset) error = %v", err)
	}
	if got.TurnCompleteSound != "" || !strings.Contains(stdout.String(), "turn-complete sound reset to none") {
		t.Fatalf("TurnCompleteSound after reset=%q output=%q", got.TurnCompleteSound, stdout.String())
	}
}

func TestSoundCommandRejectsEmptySoundName(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	app := command.New(command.Options{Stdin: &bytes.Buffer{}, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	err := app.Run(context.Background(), []string{"vibe-pushover", "sound", "turn-complete", " ", "--config", path})
	if err == nil || !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("Run() error = %v, want empty sound rejection", err)
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
			"monospace": r.Form.Get("monospace"),
			"ttl":       r.Form.Get("ttl"),
			"timestamp": r.Form.Get("timestamp"),
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
		Stdin:      bytes.NewBufferString(`{"cwd":"/tmp/demo","timestamp":1752761234567,"tool_name":"Bash","tool_input":{"command":"make deploy"},"session_url":"https://example.com/agent/42"}`),
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
	if got["monospace"] != "1" {
		t.Fatalf("monospace = %q, want approval command formatting", got["monospace"])
	}
	if got["ttl"] != "1800" {
		t.Fatalf("ttl = %q, want 1800", got["ttl"])
	}
	if got["timestamp"] != "1752761234" {
		t.Fatalf("timestamp = %q, want hook event time", got["timestamp"])
	}
	if got["url"] != "https://example.com/agent/42" || got["url_title"] != "Open agent" {
		t.Fatalf("supplementary action = %q (%q)", got["url"], got["url_title"])
	}
	if got["device"] != "iphone" {
		t.Fatalf("device = %q, want iphone", got["device"])
	}
}

func TestNotifyCommandUsesConfiguredEndToEndEncryption(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", EncryptionKey: strings.Repeat("42", 32),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var encrypted, title, message string
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		encrypted, title, message = r.Form.Get("encrypted"), r.Form.Get("title"), r.Form.Get("message")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"status":1,"request":"request-id"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	app := command.New(command.Options{
		Stdin:  bytes.NewBufferString(`{"cwd":"/tmp/demo","last_assistant_message":"Implemented E2EE."}`),
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, HTTPClient: httpClient,
		Endpoint: "https://pushover.test/messages.json", DedupePath: filepath.Join(dir, "dedupe.json"),
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path, "--agent", "codex", "--event", "turn-complete",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if encrypted != "1" || title == "" || title == "✓ Codex finished · demo" || message == "" || message == "Implemented E2EE." {
		t.Fatalf("encrypted=%q title=%q message=%q, want encrypted sensitive fields", encrypted, title, message)
	}
}

func TestNotifyCommandMinimalDetailHidesHookPayloadContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", NotificationDetail: "minimal",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	t.Setenv("CRAFT_EVENT_DATA", `{"cwd":"/tmp/private-project","tool_name":"Bash","tool_input":{"command":"deploy secret-service"}}`)
	var title, body string
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			title, body = r.Form.Get("title"), r.Form.Get("message")
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
		})},
		Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path, "--agent", "craft", "--event", "approval-required",
		"--payload-env", "CRAFT_EVENT_DATA",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if title != "⚠ Craft Agents needs approval · private-project" || body != "Approval requested." {
		t.Fatalf("title/body = %q / %q", title, body)
	}
	if strings.Contains(body, "deploy") || strings.Contains(body, "secret-service") {
		t.Fatalf("minimal detail leaked hook payload: %q", body)
	}
}

func TestNotifyCommandPrivateDetailHidesProjectAndSupplementaryAction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", NotificationDetail: "private",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	t.Setenv("AUTOHAND_EVENT", `{"cwd":"/tmp/private-project","response":"Sensitive response","url":"https://example.com/private-session"}`)
	var title, body, rawURL, urlTitle string
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			title = r.Form.Get("title")
			body = r.Form.Get("message")
			rawURL = r.Form.Get("url")
			urlTitle = r.Form.Get("url_title")
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
		})},
		Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path, "--agent", "autohand", "--event", "turn-complete",
		"--payload-env", "AUTOHAND_EVENT",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if title != "✓ Autohand Code finished" || body != "Turn completed." {
		t.Fatalf("private title/body = %q / %q", title, body)
	}
	if rawURL != "" || urlTitle != "" {
		t.Fatalf("private notification kept action %q / %q", rawURL, urlTitle)
	}
}

func TestNotifyNoInputDoesNotReadInheritedAgentTerminal(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	requests := 0
	app := command.New(command.Options{
		Stdin: errorReader{}, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
		})},
		Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path, "--agent", "rovo", "--event", "turn-complete", "--no-input",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("Pushover requests = %d, want 1", requests)
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

func TestNotifyCommandOnCallProfileSendsEmergencyBlocker(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", NotificationProfile: "on-call",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var sent map[string]string
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		sent = map[string]string{
			"priority": r.Form.Get("priority"), "sound": r.Form.Get("sound"),
			"ttl": r.Form.Get("ttl"), "retry": r.Form.Get("retry"), "expire": r.Form.Get("expire"),
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"status":1,"request":"request-id","receipt":"receipt-id"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	app := command.New(command.Options{
		Stdin:  bytes.NewBufferString(`{"cwd":"/tmp/demo","tool_name":"Bash"}`),
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, HTTPClient: httpClient,
		Endpoint: "https://pushover.test/messages.json", DedupePath: filepath.Join(dir, "dedupe.json"),
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path, "--agent", "codex", "--event", "approval-required",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := map[string]string{"priority": "2", "sound": "persistent", "ttl": "", "retry": "60", "expire": "900"}
	if !reflect.DeepEqual(sent, want) {
		t.Fatalf("sent form = %#v, want %#v", sent, want)
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
		Stdout: &stdout, Stderr: &bytes.Buffer{}, DefaultConfigPath: filepath.Join(t.TempDir(), "missing.json"),
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

func TestPreviewCommandShowsMonospaceApprovalCommand(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin:  bytes.NewBufferString(`{"cwd":"/tmp/demo","tool_name":"Bash","tool_input":{"command":"make deploy"}}`),
		Stdout: &stdout, Stderr: &bytes.Buffer{}, DefaultConfigPath: filepath.Join(t.TempDir(), "missing.json"),
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "preview", "--agent", "codex", "--event", "approval-required",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if output := stdout.String(); !strings.Contains(output, "Body: Bash\nmake deploy") || !strings.Contains(output, "Formatting: monospace") {
		t.Fatalf("preview output does not explain command formatting:\n%s", output)
	}
}

func TestPreviewCommandUsesDefaultConfigWhenAvailable(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", NotificationProfile: "watch", TurnCompleteSound: "magic",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString(`{"cwd":"/tmp/demo"}`), Stdout: &stdout, Stderr: &bytes.Buffer{}, DefaultConfigPath: path,
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "preview", "--agent", "codex", "--event", "turn-complete",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{"Priority: 0", "Sound: magic"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("preview output does not contain %q:\n%s", want, stdout.String())
		}
	}
}

func TestPreviewCommandUsesConfiguredProfileAndEventSound(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", NotificationProfile: "watch", TurnCompleteSound: "magic",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString(`{"workspacePaths":["/tmp/demo"]}`), Stdout: &stdout, Stderr: &bytes.Buffer{},
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "preview", "--agent", "antigravity", "--event", "turn-complete", "--config", path,
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{"✓ Antigravity finished · demo", "Priority: 0", "Sound: magic"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("preview output does not contain %q:\n%s", want, stdout.String())
		}
	}
}

func TestPreviewCommandUsesConfiguredMinimalDetail(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", NotificationDetail: "minimal",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin:  bytes.NewBufferString(`{"cwd":"/tmp/demo","last_assistant_message":"Sensitive implementation summary."}`),
		Stdout: &stdout, Stderr: &bytes.Buffer{},
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "preview", "--agent", "craft", "--event", "turn-complete", "--config", path,
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Body: Turn completed.") || strings.Contains(stdout.String(), "Sensitive implementation summary") {
		t.Fatalf("minimal preview output leaked hook detail:\n%s", stdout.String())
	}
}

func TestPreviewCommandExplicitQuietProfileOverridesConfiguredSound(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", NotificationProfile: "watch", TurnCompleteSound: "magic",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString(`{"cwd":"/tmp/demo"}`), Stdout: &stdout, Stderr: &bytes.Buffer{}, DefaultConfigPath: filepath.Join(t.TempDir(), "missing.json"),
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "preview", "--agent", "codex", "--event", "turn-complete",
		"--config", path, "--profile", "quiet",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Sound: none") {
		t.Fatalf("explicit quiet profile did not override configured sound:\n%s", stdout.String())
	}
}

func TestPreviewCommandShowsUrgentProfileCompletionIsSuppressed(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString(`{"cwd":"/tmp/demo"}`), Stdout: &stdout, Stderr: &bytes.Buffer{}, DefaultConfigPath: filepath.Join(t.TempDir(), "missing.json"),
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

func TestPreviewCommandShowsOnCallEmergencySchedule(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString(`{"cwd":"/tmp/demo","tool_name":"Bash"}`), Stdout: &stdout, Stderr: &bytes.Buffer{},
		DefaultConfigPath: filepath.Join(t.TempDir(), "missing.json"),
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "preview", "--agent", "codex", "--event", "approval-required", "--profile", "on-call",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{"Priority: 2", "Sound: persistent", "TTL: 0s", "Retry: 1m0s", "Expire: 15m0s"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("preview output does not contain %q:\n%s", want, stdout.String())
		}
	}
}

func TestPreviewCommandExplainsMatchingSilenceRule(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key",
		SilenceRules: []config.SilenceRule{{Agent: "codex", Project: "demo", Event: "turn-complete"}},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	stdout := &bytes.Buffer{}
	app := command.New(command.Options{
		Stdin:  bytes.NewBufferString(`{"cwd":"/tmp/demo","last_assistant_message":"Done."}`),
		Stdout: stdout, Stderr: &bytes.Buffer{},
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "preview", "--agent", "codex", "--event", "turn-complete", "--config", path,
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "Delivery: suppressed by a matching silence rule" {
		t.Fatalf("preview output = %q", got)
	}
}

func TestPreviewCommandExplainsActiveSnooze(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", SnoozedUntil: "2026-07-17T14:00:00Z",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	stdout := &bytes.Buffer{}
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString(`{"cwd":"/tmp/demo"}`), Stdout: stdout, Stderr: &bytes.Buffer{},
		Now: func() time.Time { return time.Date(2026, time.July, 17, 6, 0, 0, 0, time.FixedZone("PDT", -7*60*60)) },
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "preview", "--agent", "codex", "--event", "turn-complete", "--config", path,
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "Delivery: snoozed until 2026-07-17 07:00 PDT" {
		t.Fatalf("preview output = %q", got)
	}
}

func TestPreviewCommandExplainsActiveFocusForCompletion(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", FocusUntil: "2026-07-17T14:00:00Z",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	stdout := &bytes.Buffer{}
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString(`{"cwd":"/tmp/demo"}`), Stdout: stdout, Stderr: &bytes.Buffer{},
		Now: func() time.Time { return time.Date(2026, time.July, 17, 6, 0, 0, 0, time.FixedZone("PDT", -7*60*60)) },
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "preview", "--agent", "codex", "--event", "turn-complete", "--config", path,
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "Delivery: completion suppressed by focus mode until 2026-07-17 07:00 PDT" {
		t.Fatalf("preview output = %q", got)
	}
}

func TestPreviewCommandExplainsActiveQuietHoursForCompletion(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", QuietHoursStart: "22:00", QuietHoursEnd: "08:00",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	stdout := &bytes.Buffer{}
	app := command.New(command.Options{
		Stdin: bytes.NewBufferString(`{"cwd":"/tmp/demo"}`), Stdout: stdout, Stderr: &bytes.Buffer{},
		Now: func() time.Time { return time.Date(2026, time.July, 17, 23, 0, 0, 0, time.Local) },
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "preview", "--agent", "codex", "--event", "turn-complete", "--config", path,
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "Delivery: completion suppressed by quiet hours (22:00-08:00)" {
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
		Stdin:             &bytes.Buffer{},
		Stdout:            &stdout,
		Stderr:            &bytes.Buffer{},
		DefaultConfigPath: filepath.Join(t.TempDir(), "missing.json"),
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

func TestPreviewUsesOpenHandsWorkingDirectoryWithoutProcessDirectoryOverride(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin:             bytes.NewBufferString(`{"working_dir":"/tmp/openhands-demo"}`),
		Stdout:            &stdout,
		Stderr:            &bytes.Buffer{},
		DefaultConfigPath: filepath.Join(t.TempDir(), "missing.json"),
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "preview", "--agent", "openhands", "--event", "turn-complete",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if want := "✓ OpenHands finished · openhands-demo"; !strings.Contains(stdout.String(), want) {
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
		Stdin:             bytes.NewBufferString(`{"cwd":"","workspace_roots":[]}`),
		Stdout:            &stdout,
		Stderr:            &bytes.Buffer{},
		DefaultConfigPath: filepath.Join(t.TempDir(), "missing.json"),
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
		"aider", "amp", "antigravity", "autohand", "auggie", "claude", "claude-router", "cline", "codebuddy", "coderabbit", "codewhale", "codex", "continue", "copilot", "craft", "crush", "cortex", "cursor", "droid", "gemini", "gitlab-duo", "goose", "grok", "gptme", "hermes", "junie", "kimi", "kiro", "mimo", "mini-swe-agent", "mistral", "omp", "openhands", "opencode", "opendev", "pi", "plandex", "qoder", "qwen", "rovo", "swe-agent", "tabnine", "trae", "vscode", "windsurf", "workbuddy", "zcode",
		"completion+approval", "completion+approval+attention", "completion+attention", "completion only", "session exit+failure", "run wrapper",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("agents output does not contain %q:\n%s", want, output)
		}
	}
	if !strings.Contains(output, "AGENT           CAPABILITIES") || !strings.Contains(output, "mini-swe-agent  session exit+failure") {
		t.Fatalf("agents output columns are not aligned for long wrapper names:\n%s", output)
	}
}

func TestAgentsCommandCanShowOnlyDetectedAgents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("AUTOHAND_CONFIG", "")
	for _, marker := range []string{".codex", ".zcode"} {
		if err := os.MkdirAll(filepath.Join(home, marker), 0o700); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", marker, err)
		}
	}

	var stdout bytes.Buffer
	app := command.New(command.Options{Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{}})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "agents", "--detected"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{"codex", "zcode"} {
		if !strings.Contains(output, want) {
			t.Fatalf("detected agents output does not contain %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "claude") {
		t.Fatalf("detected agents output unexpectedly contains Claude:\n%s", output)
	}
}

func TestAgentsCommandDetectsRunWrapperAgents(t *testing.T) {
	home := t.TempDir()
	binDir := t.TempDir()
	executable := filepath.Join(binDir, "cn")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	if err := os.WriteFile(executable, nil, 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", executable, err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("PATH", binDir)
	if runtime.GOOS == "windows" {
		t.Setenv("PATHEXT", ".EXE")
	}
	clearAgentDetectionOverrides(t)

	var stdout bytes.Buffer
	app := command.New(command.Options{Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{}})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "agents", "--detected"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if output := stdout.String(); !strings.Contains(output, "continue") || !strings.Contains(output, "run wrapper") {
		t.Fatalf("detected agents output does not include Continue wrapper:\n%s", output)
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

func TestProfileCommandWarnsWhenEnablingOnCall(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{}})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "profile", "--config", path, "on-call"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{
		"Notification profile set to on-call",
		"Approval and attention notifications will repeat every 1m0s for up to 15m0s or until acknowledged.",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("profile output does not contain %q:\n%s", want, stdout.String())
		}
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

func TestFocusCommandSuppressesCompletionsButKeepsBlockerNotifications(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	requests := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"status":1,"request":"request-id"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	newApp := func(stdin string) *cli.Command {
		return command.New(command.Options{
			Stdin: bytes.NewBufferString(stdin), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
			Now: func() time.Time { return now }, HTTPClient: httpClient,
			Endpoint: "https://pushover.test/messages.json", DedupePath: filepath.Join(t.TempDir(), "dedupe.json"),
		})
	}
	if err := newApp("").Run(context.Background(), []string{
		"vibe-pushover", "focus", "--config", path, "45m",
	}); err != nil {
		t.Fatalf("focus Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.FocusUntil != "2026-07-17T12:45:00Z" {
		t.Fatalf("FocusUntil = %q", got.FocusUntil)
	}
	if err := newApp(`{"cwd":"/tmp/demo","last_assistant_message":"Done."}`).Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path, "--agent", "codex", "--event", "turn-complete",
	}); err != nil {
		t.Fatalf("completion notify Run() error = %v", err)
	}
	if err := newApp(`{"cwd":"/tmp/demo","tool_name":"shell","command":"git push"}`).Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path, "--agent", "codex", "--event", "approval-required",
	}); err != nil {
		t.Fatalf("approval notify Run() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("Pushover requests = %d, want only the blocker notification", requests)
	}
}

func TestFocusCommandShowsStatusAndResumesCompletions(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", FocusUntil: "2026-07-17T14:00:00Z",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{},
		Now: func() time.Time {
			return time.Date(2026, time.July, 17, 6, 0, 0, 0, time.FixedZone("PDT", -7*60*60))
		},
	})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "focus", "--config", path}); err != nil {
		t.Fatalf("status Run() error = %v", err)
	}
	if err := app.Run(context.Background(), []string{"vibe-pushover", "focus", "--config", path, "off"}); err != nil {
		t.Fatalf("off Run() error = %v", err)
	}
	if err := app.Run(context.Background(), []string{"vibe-pushover", "focus", "--config", path}); err != nil {
		t.Fatalf("off status Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.FocusUntil != "" {
		t.Fatalf("FocusUntil = %q, want cleared", got.FocusUntil)
	}
	for _, want := range []string{
		"Focus mode active until 2026-07-17 07:00 PDT; blocker notifications remain active",
		"Focus mode disabled; completion notifications resumed",
		"Focus mode is off",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("focus output does not contain %q:\n%s", want, stdout.String())
		}
	}
}

func TestQuietHoursSuppressCompletionsButKeepBlockerNotifications(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	now := time.Date(2026, time.July, 17, 23, 0, 0, 0, time.FixedZone("PDT", -7*60*60))
	requests := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"status":1,"request":"request-id"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	newApp := func(stdin string) *cli.Command {
		return command.New(command.Options{
			Stdin: bytes.NewBufferString(stdin), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
			Now: func() time.Time { return now }, HTTPClient: httpClient,
			Endpoint: "https://pushover.test/messages.json", DedupePath: filepath.Join(t.TempDir(), "dedupe.json"),
		})
	}
	if err := newApp("").Run(context.Background(), []string{
		"vibe-pushover", "quiet-hours", "--config", path, "22:00", "08:00",
	}); err != nil {
		t.Fatalf("quiet-hours Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.QuietHoursStart != "22:00" || got.QuietHoursEnd != "08:00" {
		t.Fatalf("quiet hours = %q-%q", got.QuietHoursStart, got.QuietHoursEnd)
	}
	if err := newApp(`{"cwd":"/tmp/demo","last_assistant_message":"Done."}`).Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path, "--agent", "codex", "--event", "turn-complete",
	}); err != nil {
		t.Fatalf("completion notify Run() error = %v", err)
	}
	if err := newApp(`{"cwd":"/tmp/demo","tool_name":"shell","tool_input":{"command":"git push"}}`).Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path, "--agent", "codex", "--event", "approval-required",
	}); err != nil {
		t.Fatalf("approval notify Run() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("Pushover requests = %d, want only the blocker notification", requests)
	}
}

func TestSilenceRulesSuppressMatchingCompletionsButKeepOtherNotifications(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	requests := 0
	stdout := &bytes.Buffer{}
	stdin := &bytes.Buffer{}
	app := command.New(command.Options{
		Stdin: stdin, Stdout: stdout, Stderr: &bytes.Buffer{}, DedupePath: filepath.Join(dir, "dedupe.json"),
		HTTPClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
		})},
		Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "silence", "add", "--config", path, "--agent", "codex", "--project", "demo",
	}); err != nil {
		t.Fatalf("silence add Run() error = %v", err)
	}

	stdin.WriteString(`{"cwd":"/tmp/demo","last_assistant_message":"Done."}`)
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path, "--agent", "codex", "--event", "turn-complete",
	}); err != nil {
		t.Fatalf("matching completion Run() error = %v", err)
	}
	stdin.WriteString(`{"cwd":"/tmp/demo","message":"Approve command."}`)
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path, "--agent", "codex", "--event", "approval-required",
	}); err != nil {
		t.Fatalf("matching approval Run() error = %v", err)
	}
	stdin.WriteString(`{"cwd":"/tmp/other","last_assistant_message":"Done elsewhere."}`)
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path, "--agent", "codex", "--event", "turn-complete",
	}); err != nil {
		t.Fatalf("other project completion Run() error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want approval plus non-matching completion", requests)
	}
	if !strings.Contains(stdout.String(), "Silence rule 1 added") {
		t.Fatalf("silence output = %q", stdout.String())
	}
}

func TestSilenceCommandListsRemovesAndClearsRules(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key",
		SilenceRules: []config.SilenceRule{
			{Agent: "codex", Event: "turn-complete"},
			{Project: "private-repo", Event: "all"},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	stdout := &bytes.Buffer{}
	app := command.New(command.Options{Stdin: &bytes.Buffer{}, Stdout: stdout, Stderr: &bytes.Buffer{}})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "silence", "--config", path}); err != nil {
		t.Fatalf("list Run() error = %v", err)
	}
	for _, want := range []string{"1: event=turn-complete agent=codex project=*", "2: event=all agent=* project=private-repo"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("list output does not contain %q: %q", want, stdout.String())
		}
	}
	stdout.Reset()
	if err := app.Run(context.Background(), []string{"vibe-pushover", "silence", "remove", "--config", path, "1"}); err != nil {
		t.Fatalf("remove Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got.SilenceRules) != 1 || got.SilenceRules[0].Project != "private-repo" {
		t.Fatalf("rules after remove = %#v", got.SilenceRules)
	}
	stdout.Reset()
	if err := app.Run(context.Background(), []string{"vibe-pushover", "silence", "clear", "--config", path}); err != nil {
		t.Fatalf("clear Run() error = %v", err)
	}
	got, err = config.Load(path)
	if err != nil {
		t.Fatalf("Load() after clear error = %v", err)
	}
	if len(got.SilenceRules) != 0 || !strings.Contains(stdout.String(), "All silence rules cleared") {
		t.Fatalf("rules/output after clear = %#v / %q", got.SilenceRules, stdout.String())
	}
}

func TestSilenceAddIsIdempotent(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	stdout := &bytes.Buffer{}
	app := command.New(command.Options{Stdin: &bytes.Buffer{}, Stdout: stdout, Stderr: &bytes.Buffer{}})
	args := []string{"vibe-pushover", "silence", "add", "--config", path, "--agent", "Codex", "--project", "Demo"}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got.SilenceRules) != 1 || !strings.Contains(stdout.String(), "already exists") {
		t.Fatalf("rules/output = %#v / %q", got.SilenceRules, stdout.String())
	}
}

func TestTestCommandReportsQuietHoursSuppressionAndSupportsForce(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", QuietHoursStart: "22:00", QuietHoursEnd: "08:00",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	requests := 0
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{},
		Now: func() time.Time {
			return time.Date(2026, time.July, 17, 23, 0, 0, 0, time.FixedZone("PDT", -7*60*60))
		},
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
		"vibe-pushover", "test", "--config", path, "--event", "turn-complete",
	}); err != nil {
		t.Fatalf("test Run() error = %v", err)
	}
	if requests != 0 || !strings.Contains(stdout.String(), "suppressed during quiet hours 22:00-08:00") {
		t.Fatalf("quiet-hours test requests = %d, output = %q", requests, stdout.String())
	}
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "test", "--config", path, "--event", "turn-complete", "--force",
	}); err != nil {
		t.Fatalf("forced test Run() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("forced Pushover requests = %d, want 1", requests)
	}
}

func TestQuietHoursCommandShowsStatusAndDisablesSchedule(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{
		AppToken: "app-token", UserKey: "user-key", QuietHoursStart: "22:00", QuietHoursEnd: "08:00",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{}})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "quiet-hours", "--config", path}); err != nil {
		t.Fatalf("status Run() error = %v", err)
	}
	if err := app.Run(context.Background(), []string{"vibe-pushover", "quiet-hours", "--config", path, "off"}); err != nil {
		t.Fatalf("off Run() error = %v", err)
	}
	if err := app.Run(context.Background(), []string{"vibe-pushover", "quiet-hours", "--config", path}); err != nil {
		t.Fatalf("off status Run() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.QuietHoursStart != "" || got.QuietHoursEnd != "" {
		t.Fatalf("quiet hours were not cleared: %#v", got)
	}
	for _, want := range []string{
		"Quiet hours active daily from 22:00 to 08:00; blocker notifications remain active",
		"Quiet hours disabled; completion notifications resumed",
		"Quiet hours are off",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("quiet-hours output does not contain %q:\n%s", want, stdout.String())
		}
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

func TestNotifyCommandReadsPayloadFromEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	t.Setenv("GJC_NOTIFICATION_JSON", `{"cwd":"/tmp/gajae-demo","lastAssistantMessage":"All Gajae tests pass."}`)
	var title, body string
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			title, body = r.Form.Get("title"), r.Form.Get("message")
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
		})},
		Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--config", path, "--agent", "gajae", "--event", "turn-complete",
		"--payload-env", "GJC_NOTIFICATION_JSON",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if title != "✓ Gajae Code finished · gajae-demo" || body != "All Gajae tests pass." {
		t.Fatalf("title/body = %q / %q", title, body)
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

func TestInstallCommandAcceptsCCRAsClaudeCodeRouterAlias(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "settings.json")
	stdout := &bytes.Buffer{}
	app := command.New(command.Options{
		Stdout: stdout, Stderr: &bytes.Buffer{}, Executable: "/opt/bin/vibe-pushover",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "install", "--agent", "ccr", "--agent-config", path,
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Contains(data, []byte("--agent claude --event turn-complete")) {
		t.Fatalf("CCR alias did not install shared Claude hook: %s", data)
	}
	if !strings.Contains(stdout.String(), "claude-router") {
		t.Fatalf("install output did not identify Claude Code Router: %q", stdout.String())
	}
}

func TestInstallCommandReportsDotCraftTrustStep(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	app := command.New(command.Options{
		Stdout: stdout, Stderr: &bytes.Buffer{}, Executable: "/opt/bin/vibe-pushover",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "install", "--agent", "dotcraft",
		"--agent-config", filepath.Join(t.TempDir(), "hooks.json"),
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{"DotCraft", "Settings", "Hooks", "trust"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("install output does not contain %q: %q", want, stdout.String())
		}
	}
}

func TestInstallCommandConfiguresDetectedAgents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("AUTOHAND_CONFIG", "")
	t.Setenv("CRAFT_CONFIG_DIR", "")
	for _, path := range []string{
		filepath.Join(home, ".autohand"),
		filepath.Join(home, ".codex"),
		filepath.Join(home, ".craft-agent", "workspaces", "work"),
		filepath.Join(home, ".zcode", "cli"),
	} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", path, err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".zcode", "cli", "config.json"), []byte(`{"provider":{"type":"custom"}}`), 0o600); err != nil {
		t.Fatalf("WriteFile(ZCode config) error = %v", err)
	}

	stdout := &bytes.Buffer{}
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: stdout, Stderr: &bytes.Buffer{}, Executable: "/opt/bin/vibe-pushover",
	})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "install", "--detected"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{"Installed autohand hooks", "Installed codex hooks", "Installed craft automations", "Installed zcode hooks"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("install output does not contain %q: %q", want, stdout.String())
		}
	}
	for _, installed := range []struct {
		path    string
		command string
	}{
		{filepath.Join(home, ".autohand", "config.json"), "notify --agent autohand --event turn-complete"},
		{filepath.Join(home, ".codex", "hooks.json"), "notify --agent codex --event turn-complete"},
		{filepath.Join(home, ".craft-agent", "workspaces", "work", "automations.json"), "notify --agent craft --event turn-complete"},
		{filepath.Join(home, ".zcode", "cli", "config.json"), "notify --agent zcode --event turn-complete"},
	} {
		data, err := os.ReadFile(installed.path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", installed.path, err)
		}
		if !bytes.Contains(data, []byte(installed.command)) {
			t.Fatalf("%s does not contain %q:\n%s", installed.path, installed.command, data)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("undetected Claude config was created: %v", err)
	}
}

func TestInstallCommandConfiguresInstalledCLIBeforeItsFirstRun(t *testing.T) {
	clearAgentDetectionOverrides(t)
	home := filepath.Join(t.TempDir(), "empty-home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("MkdirAll(HOME) error = %v", err)
	}
	binDir := t.TempDir()
	name := "codex"
	if runtime.GOOS == "windows" {
		name += ".exe"
		t.Setenv("PATHEXT", ".EXE")
	}
	if err := os.WriteFile(filepath.Join(binDir, name), nil, 0o755); err != nil {
		t.Fatalf("WriteFile(codex) error = %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("PATH", binDir)

	stdout := &bytes.Buffer{}
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: stdout, Stderr: &bytes.Buffer{}, Executable: "/opt/bin/vibe-pushover",
	})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "install", "--detected"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Installed codex hooks") {
		t.Fatalf("install output = %q, want Codex installation", stdout.String())
	}
	data, err := os.ReadFile(filepath.Join(home, ".codex", "hooks.json"))
	if err != nil {
		t.Fatalf("ReadFile(Codex hooks) error = %v", err)
	}
	if !bytes.Contains(data, []byte("notify --agent codex --event turn-complete")) {
		t.Fatalf("Codex hooks do not contain vibe-pushover command:\n%s", data)
	}
}

func TestInstallCommandSkipsAmbiguousSharedPiOverride(t *testing.T) {
	clearAgentDetectionOverrides(t)
	home := filepath.Join(t.TempDir(), "empty-home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("MkdirAll(HOME) error = %v", err)
	}
	binDir := t.TempDir()
	for _, name := range []string{"pi", "omp"} {
		path := filepath.Join(binDir, name)
		if runtime.GOOS == "windows" {
			path += ".exe"
			t.Setenv("PATHEXT", ".EXE")
		}
		if err := os.WriteFile(path, nil, 0o755); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}
	sharedHome := filepath.Join(t.TempDir(), "shared-agent-home")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("PATH", binDir)
	t.Setenv("PI_CODING_AGENT_DIR", sharedHome)

	stdout := &bytes.Buffer{}
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: stdout, Stderr: &bytes.Buffer{}, Executable: "/opt/bin/vibe-pushover",
	})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "install", "--detected"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := stdout.String(), "No supported agent installations detected\n"; got != want {
		t.Fatalf("install output = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(sharedHome, "extensions", "vibe-pushover", "index.ts")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ambiguous Pi/OMP extension was created: %v", err)
	}
}

func TestInstallCommandConfiguresEveryCraftWorkspace(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "craft-config")
	t.Setenv("CRAFT_CONFIG_DIR", configDir)
	for _, name := range []string{"personal", "work"} {
		if err := os.MkdirAll(filepath.Join(configDir, "workspaces", name), 0o700); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", name, err)
		}
	}
	stdout := &bytes.Buffer{}
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: stdout, Stderr: &bytes.Buffer{}, Executable: "/opt/bin/vibe-pushover",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "install", "--agent", "craft", "--config", filepath.Join(t.TempDir(), "pushover.json"),
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, name := range []string{"personal", "work"} {
		path := filepath.Join(configDir, "workspaces", name, "automations.json")
		if !strings.Contains(stdout.String(), "Installed craft automations in "+path) {
			t.Fatalf("install output does not include %q:\n%s", path, stdout.String())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		}
		for _, want := range []string{
			"notify --agent craft --event turn-complete",
			"notify --agent craft --event approval-required",
			"notify --agent craft --event attention-required",
		} {
			if !bytes.Contains(data, []byte(want)) {
				t.Fatalf("%s does not contain %q:\n%s", path, want, data)
			}
		}
	}
}

func TestInstallCommandCraftRequiresAWorkspaceOrExplicitConfig(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "empty-craft-config")
	t.Setenv("CRAFT_CONFIG_DIR", configDir)
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Executable: "/opt/bin/vibe-pushover",
	})
	err := app.Run(context.Background(), []string{"vibe-pushover", "install", "--agent", "craft"})
	if err == nil || !strings.Contains(err.Error(), "no Craft Agents workspaces found; create a workspace or pass --agent-config") {
		t.Fatalf("Run() error = %v", err)
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

func TestInstallCommandCreatesCodeWhaleLifecycleHooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	original := `# Keep this comment
provider = "deepseek"

[[hooks.hooks]]
name = "third-party"
event = "session_start"
command = "./prepare.sh"
`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	app := command.New(command.Options{
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		Executable: "/opt/bin/vibe-pushover",
	})
	args := []string{
		"vibe-pushover", "install",
		"--agent", "codewhale",
		"--agent-config", path,
	}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{
		"# Keep this comment",
		`name = "third-party"`,
		`event = "turn_end"`,
		`event = "on_error"`,
		`--agent codewhale --event turn-complete --ignore-errors --skip-codewhale-noncompletion`,
		`--agent codewhale --event attention-required`,
	} {
		if !bytes.Contains(first, []byte(want)) {
			t.Fatalf("CodeWhale config does not contain %q:\n%s", want, first)
		}
	}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("second ReadFile() error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("CodeWhale install is not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestNotifyCommandSkipsFailedCodeWhaleTurnEnd(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := config.Save(configPath, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	requests := 0
	app := command.New(command.Options{
		Stdin:  bytes.NewBufferString(`{"event":"turn_end","status":"failed","error":"provider error"}`),
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, DedupePath: filepath.Join(dir, "dedupe.json"),
		HTTPClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
		})},
		Endpoint: "https://pushover.test/messages.json",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "notify", "--agent", "codewhale", "--event", "turn-complete",
		"--skip-codewhale-noncompletion", "--config", configPath,
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if requests != 0 {
		t.Fatalf("Pushover requests = %d, want 0", requests)
	}
}

func TestInstallCommandAcceptsDeepSeekAlias(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	app := command.New(command.Options{
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Executable: "/opt/bin/vibe-pushover",
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "install", "--agent", "DeepSeek", "--agent-config", path,
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Contains(data, []byte("--agent codewhale --event turn-complete")) {
		t.Fatalf("DeepSeek alias did not install canonical CodeWhale hooks:\n%s", data)
	}
}

func TestInstallCommandCreatesOpenHandsCompletionHook(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".openhands", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	original := `{"pre_tool_use":[{"matcher":"terminal","hooks":[{"command":"./protect.sh"}]}]}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	var stdout bytes.Buffer
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{}, Executable: "/opt/bin/vibe-pushover",
	})
	args := []string{
		"vibe-pushover", "install", "--agent", "openhands", "--agent-config", path,
	}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if _, ok := root["pre_tool_use"]; !ok {
		t.Fatalf("OpenHands config lost existing pre_tool_use hook:\n%s", data)
	}
	stop, ok := root["stop"].([]any)
	if !ok || len(stop) != 1 {
		t.Fatalf("OpenHands stop hooks = %#v", root["stop"])
	}
	encoded := string(data)
	for _, want := range []string{
		`"command": "'/opt/bin/vibe-pushover' notify --agent openhands --event turn-complete --ignore-errors"`,
	} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("OpenHands config does not contain %q:\n%s", want, data)
		}
	}
	if strings.Contains(encoded, `"async": true`) {
		t.Fatalf("OpenHands completion hook is asynchronous and may be killed during headless session teardown:\n%s", data)
	}
	if strings.Contains(encoded, "approval-required") {
		t.Fatalf("OpenHands config installed an unsupported approval hook:\n%s", data)
	}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	dataAfter, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(after second install) error = %v", err)
	}
	if !bytes.Equal(dataAfter, data) {
		t.Fatalf("second install changed OpenHands config:\n%s", dataAfter)
	}
}

func TestInstallCommandCreatesRovoDevLifecycleHooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".rovodev", "config.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	original := "# keep this comment\nagent:\n  modelId: auto\neventHooks:\n  events:\n    - name: on_complete\n      commands:\n        - command: ./existing-notifier.sh\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Executable: "/opt/bin/vibe-pushover",
	})
	args := []string{"vibe-pushover", "install", "--agent", "rovo", "--agent-config", path}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{
		"# keep this comment",
		"modelId: auto",
		"command: ./existing-notifier.sh",
		"name: on_complete",
		"name: on_error",
		"name: on_tool_permission",
		"notify --agent rovo --event turn-complete --ignore-errors --no-input",
		"notify --agent rovo --event attention-required --ignore-errors --no-input",
		"notify --agent rovo --event approval-required --ignore-errors --no-input",
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("Rovo Dev config does not contain %q:\n%s", want, data)
		}
	}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(after second install) error = %v", err)
	}
	if !bytes.Equal(after, data) {
		t.Fatalf("second install changed Rovo Dev config:\n%s", after)
	}
}

func TestInstallCommandCreatesCodeBuddyLifecycleHooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".codebuddy", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	original := `{"theme":"dark","hooks":{"Stop":[{"hooks":[{"type":"command","command":"./existing-notifier.sh"}]}]}}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Executable: "/opt/bin/vibe-pushover",
	})
	args := []string{"vibe-pushover", "install", "--agent", "codebuddy", "--agent-config", path}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{
		`"theme": "dark"`,
		`"command": "./existing-notifier.sh"`,
		`"Stop"`,
		`"StopFailure"`,
		`"PermissionRequest"`,
		"notify --agent codebuddy --event turn-complete --ignore-errors --skip-active-stop",
		"notify --agent codebuddy --event attention-required --ignore-errors",
		"notify --agent codebuddy --event approval-required --ignore-errors",
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("CodeBuddy settings do not contain %q:\n%s", want, data)
		}
	}
	before := append([]byte(nil), data...)
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(after second install) error = %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("second install changed CodeBuddy settings:\n%s", after)
	}
}

func TestInstallCommandCreatesZCodeLifecycleHooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".zcode", "cli", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	original := `{"provider":{"type":"custom"},"hooks":{"enabled":true,"timeoutMs":45000,"maxOutputBytes":16384,"events":{"PostToolUse":[{"matcher":"Write","hooks":[{"type":"command","command":"./format.sh","async":false}]}]}}}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	stdout := &bytes.Buffer{}
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: stdout, Stderr: &bytes.Buffer{}, Executable: "/opt/bin/vibe-pushover",
	})
	args := []string{"vibe-pushover", "install", "--agent", "zcode", "--agent-config", path}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{
		`"type": "custom"`,
		`"timeoutMs": 45000`,
		`"maxOutputBytes": 16384`,
		`"command": "./format.sh"`,
		`"Stop"`,
		`"PermissionRequest"`,
		"notify --agent zcode --event turn-complete --ignore-errors",
		"notify --agent zcode --event approval-required --ignore-errors",
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("ZCode config does not contain %q:\n%s", want, data)
		}
	}
	if !strings.Contains(stdout.String(), "Installed zcode hooks") {
		t.Fatalf("install output = %q", stdout.String())
	}

	before := append([]byte(nil), data...)
	stdout.Reset()
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(after second install) error = %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("second install changed ZCode config:\n%s", after)
	}
	if !strings.Contains(stdout.String(), "zcode hooks already installed") {
		t.Fatalf("second install output = %q", stdout.String())
	}
}

func TestInstallCommandCreatesWorkBuddyLifecycleHooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".workbuddy", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Executable: "/opt/bin/vibe-pushover",
	})
	args := []string{"vibe-pushover", "install", "--agent", "workbuddy", "--agent-config", path}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{
		`"theme": "dark"`,
		`"Stop"`,
		`"StopFailure"`,
		`"PermissionRequest"`,
		"notify --agent workbuddy --event turn-complete --ignore-errors --skip-active-stop",
		"notify --agent workbuddy --event attention-required --ignore-errors",
		"notify --agent workbuddy --event approval-required --ignore-errors",
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("WorkBuddy settings do not contain %q:\n%s", want, data)
		}
	}
	before := append([]byte(nil), data...)
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(after second install) error = %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("second install changed WorkBuddy settings:\n%s", after)
	}
}

func TestInstallCommandCreatesAntigravityCompletionPlugin(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".gemini", "antigravity-cli", "plugins", "vibe-pushover")
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Executable: "/opt/bin/vibe-pushover",
	})
	args := []string{"vibe-pushover", "install", "--agent", "antigravity", "--agent-config", path}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	manifest, err := os.ReadFile(filepath.Join(path, "plugin.json"))
	if err != nil {
		t.Fatalf("ReadFile(plugin.json) error = %v", err)
	}
	if !bytes.Contains(manifest, []byte(`"name": "vibe-pushover"`)) {
		t.Fatalf("Antigravity plugin manifest is unexpected:\n%s", manifest)
	}
	hookPath := filepath.Join(path, "hooks.json")
	hookData, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("ReadFile(hooks.json) error = %v", err)
	}
	for _, want := range []string{
		`"vibe-pushover"`, `"Stop"`, `"type": "command"`, `"timeout": 10`,
		"notify --agent antigravity --event turn-complete --ignore-errors --skip-antigravity-noncompletion",
		"notify --agent antigravity --event attention-required --ignore-errors --only-antigravity-failure",
	} {
		if !bytes.Contains(hookData, []byte(want)) {
			t.Fatalf("Antigravity hooks do not contain %q:\n%s", want, hookData)
		}
	}
	before := append([]byte(nil), hookData...)
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	after, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("ReadFile(after second install) error = %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("second install changed Antigravity hooks:\n%s", after)
	}
}

func TestNotifyCommandFiltersAntigravityStopStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, event, flag, payload string
		wantRequests               int
	}{
		{name: "background work remains", event: "turn-complete", flag: "--skip-antigravity-noncompletion", payload: `{"fullyIdle":false,"terminationReason":"model_stop"}`},
		{name: "failure is not completion", event: "turn-complete", flag: "--skip-antigravity-noncompletion", payload: `{"fullyIdle":true,"terminationReason":"error","error":"boom"}`},
		{name: "normal stop is not attention", event: "attention-required", flag: "--only-antigravity-failure", payload: `{"fullyIdle":true,"terminationReason":"model_stop"}`},
		{name: "failure needs attention", event: "attention-required", flag: "--only-antigravity-failure", payload: `{"fullyIdle":true,"terminationReason":"max_steps_exceeded"}`, wantRequests: 1},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			configPath := filepath.Join(dir, "config.json")
			if err := config.Save(configPath, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
				t.Fatalf("Save() error = %v", err)
			}
			requests := 0
			app := command.New(command.Options{
				Stdin: bytes.NewBufferString(tt.payload), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
				DedupePath: filepath.Join(dir, "dedupe.json"),
				HTTPClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
					requests++
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":1}`)), Header: make(http.Header)}, nil
				})},
				Endpoint: "https://pushover.test/messages.json",
			})
			if err := app.Run(context.Background(), []string{
				"vibe-pushover", "notify", "--agent", "antigravity", "--event", tt.event,
				"--config", configPath, tt.flag,
			}); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if requests != tt.wantRequests {
				t.Fatalf("Pushover requests = %d, want %d", requests, tt.wantRequests)
			}
		})
	}
}

func TestInstallCommandCreatesTabnineLifecycleHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Tabnine documents POSIX executable hook scripts")
	}
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".tabnine", "agent", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"ui":{"theme":"ANSI"},"hooks":{"enabled":false}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	app := command.New(command.Options{
		Stdin: &bytes.Buffer{}, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Executable: "/opt/bin/vibe-pushover",
	})
	args := []string{"vibe-pushover", "install", "--agent", "tabnine", "--agent-config", path}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	settings, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(settings) error = %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(settings, &root); err != nil {
		t.Fatalf("Unmarshal(settings) error = %v", err)
	}
	ui, _ := root["ui"].(map[string]any)
	hookSettings, _ := root["hooks"].(map[string]any)
	if ui["theme"] != "ANSI" || hookSettings["enabled"] != true {
		t.Fatalf("Tabnine settings = %#v", root)
	}

	hooksDir := filepath.Join(dir, ".tabnine", "hooks")
	for name, event := range map[string]string{"after-agent.sh": "turn-complete", "on-error.sh": "attention-required"} {
		data, err := os.ReadFile(filepath.Join(hooksDir, name))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", name, err)
		}
		for _, want := range []string{"# Generated by vibe-pushover for Tabnine CLI.", "--agent tabnine --event " + event, "--ignore-errors --no-input"} {
			if !bytes.Contains(data, []byte(want)) {
				t.Fatalf("Tabnine hook %s does not contain %q:\n%s", name, want, data)
			}
		}
		info, err := os.Stat(filepath.Join(hooksDir, name))
		if err != nil {
			t.Fatalf("Stat(%s) error = %v", name, err)
		}
		if info.Mode().Perm()&0o100 == 0 {
			t.Fatalf("Tabnine hook %s mode = %o, want executable", name, info.Mode().Perm())
		}
	}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
}

func TestInstallCommandCreatesSharedClineHookForIDEAndCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Keep the default Documents fallback deterministic on Linux runners that
	// may have xdg-user-dir installed with host-specific configuration.
	t.Setenv("PATH", "")

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
	// Keep the default Documents fallback deterministic on Linux runners that
	// may have xdg-user-dir installed with host-specific configuration.
	t.Setenv("PATH", "")

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

func TestRunCommandNotifiesAfterLongSuccessfulAgentSession(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := config.Save(configPath, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var received map[string]string
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		received = map[string]string{
			"title":     r.Form.Get("title"),
			"message":   r.Form.Get("message"),
			"priority":  r.Form.Get("priority"),
			"sound":     r.Form.Get("sound"),
			"timestamp": r.Form.Get("timestamp"),
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"status":1,"request":"request-id"}`)),
			Header:     make(http.Header),
		}, nil
	})}

	start := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	nowCalls := 0
	now := func() time.Time {
		nowCalls++
		if nowCalls == 1 {
			return start
		}
		return start.Add(45 * time.Second)
	}
	var gotArgv []string
	var stdout, stderr bytes.Buffer
	app := command.New(command.Options{
		Stdin:  strings.NewReader("interactive input"),
		Stdout: &stdout,
		Stderr: &stderr,
		RunProcess: func(_ context.Context, argv []string, stdin io.Reader, processStdout, processStderr io.Writer) error {
			gotArgv = append([]string(nil), argv...)
			input, err := io.ReadAll(stdin)
			if err != nil {
				return err
			}
			if string(input) != "interactive input" {
				return fmt.Errorf("stdin = %q", input)
			}
			_, _ = io.WriteString(processStdout, "agent output\n")
			_, _ = io.WriteString(processStderr, "agent diagnostics\n")
			return nil
		},
		HTTPClient: httpClient,
		Endpoint:   "https://pushover.test/messages.json",
		DedupePath: filepath.Join(dir, "dedupe.json"),
		Now:        now,
	})
	err := app.Run(context.Background(), []string{
		"vibe-pushover", "run", "--config", configPath, "--agent", "continue", "--after", "30s", "--",
		"cn", "-p", "fix the tests",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if want := []string{"cn", "-p", "fix the tests"}; !reflect.DeepEqual(gotArgv, want) {
		t.Fatalf("argv = %#v, want %#v", gotArgv, want)
	}
	if stdout.String() != "agent output\n" || stderr.String() != "agent diagnostics\n" {
		t.Fatalf("forwarded streams = stdout %q stderr %q", stdout.String(), stderr.String())
	}
	if !strings.Contains(received["title"], "Continue finished") {
		t.Fatalf("title = %q, want Continue completion", received["title"])
	}
	if received["message"] != "Completed · 45s" {
		t.Fatalf("message = %q, want scan-friendly completion duration", received["message"])
	}
	if received["priority"] != "-1" || received["sound"] != "none" {
		t.Fatalf("delivery = priority %q sound %q, want quiet completion", received["priority"], received["sound"])
	}
	if received["timestamp"] != "1784289645" {
		t.Fatalf("timestamp = %q, want process completion time", received["timestamp"])
	}
}

func TestRunCommandSuppressesShortSuccessfulAgentSession(t *testing.T) {
	t.Parallel()

	called := false
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("unexpected Pushover request")
	})}
	start := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	nowCalls := 0
	app := command.New(command.Options{
		Stdin:  &bytes.Buffer{},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		RunProcess: func(_ context.Context, _ []string, _ io.Reader, _, _ io.Writer) error {
			return nil
		},
		HTTPClient: httpClient,
		Endpoint:   "https://pushover.test/messages.json",
		Now: func() time.Time {
			nowCalls++
			if nowCalls == 1 {
				return start
			}
			return start.Add(5 * time.Second)
		},
	})
	if err := app.Run(context.Background(), []string{
		"vibe-pushover", "run", "--config", filepath.Join(t.TempDir(), "missing.json"), "--agent", "plandex", "--", "plandex",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if called {
		t.Fatal("short successful session sent a notification")
	}
}

func TestRunCommandAlwaysNotifiesFailureAndPreservesExitCode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := config.Save(configPath, config.Credentials{AppToken: "app-token", UserKey: "user-key"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var received map[string]string
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		received = map[string]string{
			"title":    r.Form.Get("title"),
			"message":  r.Form.Get("message"),
			"priority": r.Form.Get("priority"),
			"sound":    r.Form.Get("sound"),
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"status":1,"request":"request-id"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	start := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	nowCalls := 0
	app := command.New(command.Options{
		Stdin:  &bytes.Buffer{},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		RunProcess: func(_ context.Context, _ []string, _ io.Reader, _, _ io.Writer) error {
			return testExitError{code: 17}
		},
		HTTPClient: httpClient,
		Endpoint:   "https://pushover.test/messages.json",
		DedupePath: filepath.Join(dir, "dedupe.json"),
		Now: func() time.Time {
			nowCalls++
			if nowCalls == 1 {
				return start
			}
			return start.Add(2 * time.Second)
		},
	})
	app.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	err := app.Run(context.Background(), []string{
		"vibe-pushover", "run", "--config", configPath, "--agent", "crush", "--", "crush", "--yolo",
	})
	var exitCoder interface{ ExitCode() int }
	if !errors.As(err, &exitCoder) || exitCoder.ExitCode() != 17 {
		t.Fatalf("Run() error = %v, want exit code 17", err)
	}
	if command.ShouldPrintError(err) {
		t.Fatalf("Run() error = %v, want transparent child exit without duplicate diagnostic", err)
	}
	if !strings.Contains(received["title"], "Crush needs attention") {
		t.Fatalf("title = %q, want Crush attention", received["title"])
	}
	if received["message"] != "Failed · exit 17 · 2s" {
		t.Fatalf("message = %q, want scan-friendly failure status", received["message"])
	}
	if received["priority"] != "1" || received["sound"] != "persistent" {
		t.Fatalf("delivery = priority %q sound %q, want actionable attention", received["priority"], received["sound"])
	}
}

func TestRunCommandPreservesSignalExitStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signal semantics")
	}

	app := command.New(command.Options{
		Stdin:  &bytes.Buffer{},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	err := app.Run(context.Background(), []string{
		"vibe-pushover", "run",
		"--config", filepath.Join(t.TempDir(), "missing.json"),
		"--agent", "test-agent",
		"--", "/bin/sh", "-c", "kill -TERM $$",
	})
	var exitCoder interface{ ExitCode() int }
	if !errors.As(err, &exitCoder) || exitCoder.ExitCode() != 143 {
		t.Fatalf("Run() error = %v, want conventional SIGTERM status 143", err)
	}
	if command.ShouldPrintError(err) {
		t.Fatalf("Run() error = %v, want transparent signal exit without duplicate diagnostic", err)
	}
}

func TestRunCommandMapsPermissionDeniedStartTo126(t *testing.T) {
	t.Parallel()

	app := command.New(command.Options{
		Stdin:  &bytes.Buffer{},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		RunProcess: func(_ context.Context, _ []string, _ io.Reader, _, _ io.Writer) error {
			return &os.PathError{Op: "fork/exec", Path: "/tmp/agent", Err: os.ErrPermission}
		},
	})
	err := app.Run(context.Background(), []string{
		"vibe-pushover", "run",
		"--config", filepath.Join(t.TempDir(), "missing.json"),
		"--agent", "test-agent",
		"--", "/tmp/agent",
	})
	var exitCoder interface{ ExitCode() int }
	if !errors.As(err, &exitCoder) || exitCoder.ExitCode() != 126 {
		t.Fatalf("Run() error = %v, want permission-denied status 126", err)
	}
	if !command.ShouldPrintError(err) {
		t.Fatalf("Run() error = %v, want startup diagnostic", err)
	}
}

func TestInstallCommandGuidesRunWrapperAgents(t *testing.T) {
	t.Parallel()

	for agent, executable := range map[string]string{
		"coderabbit":     "coderabbit",
		"continue":       "cn",
		"crush":          "crush",
		"gitlab-duo":     "duo",
		"mini-swe-agent": "mini",
		"opendev":        "opendev",
		"plandex":        "plandex",
		"swe-agent":      "sweagent",
	} {
		agent, executable := agent, executable
		t.Run(agent, func(t *testing.T) {
			t.Parallel()
			app := command.New(command.Options{
				Stdin:      &bytes.Buffer{},
				Stdout:     &bytes.Buffer{},
				Stderr:     &bytes.Buffer{},
				Executable: "/opt/bin/vibe-pushover",
			})
			err := app.Run(context.Background(), []string{
				"vibe-pushover", "install", "--agent", agent,
			})
			want := "vibe-pushover run --agent " + agent + " -- " + executable
			if err == nil || !strings.Contains(err.Error(), "uses the run wrapper") || !strings.Contains(err.Error(), want) {
				t.Fatalf("Run() error = %v, want %q wrapper guidance", err, want)
			}
			if agent == "gitlab-duo" && !strings.Contains(err.Error(), "vibe-pushover run --agent gitlab-duo -- glab duo cli") {
				t.Fatalf("Run() error = %v, want GitLab CLI alternative", err)
			}
			if agent == "coderabbit" && !strings.Contains(err.Error(), "vibe-pushover run --agent coderabbit -- cr") {
				t.Fatalf("Run() error = %v, want CodeRabbit short alias", err)
			}
		})
	}
}

func TestInstallDetectedExplainsThatWrapperAgentsNeedNoInstallation(t *testing.T) {
	home := t.TempDir()
	binDir := t.TempDir()
	executable := filepath.Join(binDir, "cn")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	if err := os.WriteFile(executable, nil, 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", executable, err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("PATH", binDir)
	if runtime.GOOS == "windows" {
		t.Setenv("PATHEXT", ".EXE")
	}
	clearAgentDetectionOverrides(t)

	var stdout bytes.Buffer
	app := command.New(command.Options{Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &bytes.Buffer{}})
	if err := app.Run(context.Background(), []string{"vibe-pushover", "install", "--detected"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if output := stdout.String(); !strings.Contains(output, "Run-wrapper agents need no installation: continue") {
		t.Fatalf("install output does not explain detected wrapper:\n%s", output)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

type errorReader struct{}

type errorWriter struct{}

type testExitError struct {
	code int
}

func (e testExitError) Error() string {
	return fmt.Sprintf("process exited with status %d", e.code)
}

func (e testExitError) ExitCode() int {
	return e.code
}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("stdin must not be read")
}

func (errorWriter) Write([]byte) (int, error) {
	return 0, errors.New("stdout write failed")
}

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
