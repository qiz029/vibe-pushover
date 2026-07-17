package config_test

import (
	"os"
	"path/filepath"
	"testing"

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
