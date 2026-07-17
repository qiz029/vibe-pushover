package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiz029/vibe-pushover/internal/config"
)

func TestSaveAndLoadCredentials(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "config.json")
	want := config.Credentials{AppToken: "app-token", UserKey: "user-key"}

	if err := config.Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0o600 {
		t.Fatalf("config mode = %o, want 600", gotMode)
	}
}

func TestSaveAndLoadNotificationProfile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	want := config.Credentials{
		AppToken:            "app-token",
		UserKey:             "user-key",
		NotificationProfile: "watch",
	}
	if err := config.Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestSaveAndLoadTargetDevices(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	want := config.Credentials{
		AppToken: "app-token",
		UserKey:  "user-key",
		Device:   "iphone,ipad",
	}
	if err := config.Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestRejectsUnknownNotificationProfile(t *testing.T) {
	t.Parallel()

	err := (config.Credentials{AppToken: "app", UserKey: "user", NotificationProfile: "loudest"}).Validate()
	if err == nil {
		t.Fatal("Validate() accepted unknown notification profile")
	}
}

func TestRejectsInvalidTargetDevice(t *testing.T) {
	t.Parallel()

	for _, device := range []string{"iphone 15", "iphone,", "this-device-name-is-over-25-characters"} {
		err := (config.Credentials{AppToken: "app", UserKey: "user", Device: device}).Validate()
		if err == nil {
			t.Fatalf("Validate() accepted invalid device %q", device)
		}
	}
}

func TestSoundPreferencesSupportBuiltInCustomAndAccountDefaultNames(t *testing.T) {
	t.Parallel()

	valid := config.Credentials{
		AppToken: "app", UserKey: "user",
		TurnCompleteSound: "magic", ApprovalSound: "team_alert-1", AttentionSound: "default",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() rejected sound preferences: %v", err)
	}
	for _, sound := range []string{"two words", "bad/sound", strings.Repeat("a", 65)} {
		credentials := config.Credentials{AppToken: "app", UserKey: "user", TurnCompleteSound: sound}
		if err := credentials.Validate(); err == nil {
			t.Fatalf("Validate() accepted invalid sound %q", sound)
		}
	}
}

func TestSnoozeStateUsesValidatedDeadline(t *testing.T) {
	t.Parallel()

	credentials := config.Credentials{
		AppToken: "app", UserKey: "user", SnoozedUntil: "2026-07-17T12:30:00Z",
	}
	before := time.Date(2026, time.July, 17, 12, 29, 59, 0, time.UTC)
	atDeadline := time.Date(2026, time.July, 17, 12, 30, 0, 0, time.UTC)
	if !credentials.IsSnoozed(before) {
		t.Fatal("IsSnoozed() = false before deadline")
	}
	if credentials.IsSnoozed(atDeadline) {
		t.Fatal("IsSnoozed() = true at deadline")
	}
	credentials.SnoozedUntil = "tomorrow"
	if err := credentials.Validate(); err == nil {
		t.Fatal("Validate() accepted malformed snooze deadline")
	}
}

func TestFocusStateUsesValidatedDeadline(t *testing.T) {
	t.Parallel()

	credentials := config.Credentials{
		AppToken: "app", UserKey: "user", FocusUntil: "2026-07-17T12:30:00Z",
	}
	before := time.Date(2026, time.July, 17, 12, 29, 59, 0, time.UTC)
	atDeadline := time.Date(2026, time.July, 17, 12, 30, 0, 0, time.UTC)
	if !credentials.IsFocused(before) {
		t.Fatal("IsFocused() = false before deadline")
	}
	if credentials.IsFocused(atDeadline) {
		t.Fatal("IsFocused() = true at deadline")
	}
	credentials.FocusUntil = "later"
	if err := credentials.Validate(); err == nil {
		t.Fatal("Validate() accepted malformed focus deadline")
	}
}
