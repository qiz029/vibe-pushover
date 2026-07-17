package config_test

import (
	"os"
	"path/filepath"
	"reflect"
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
	if !reflect.DeepEqual(got, want) {
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
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestSaveAndLoadNotificationDetail(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	want := config.Credentials{
		AppToken:           "app-token",
		UserKey:            "user-key",
		NotificationDetail: "minimal",
	}
	if err := config.Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
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
	if !reflect.DeepEqual(got, want) {
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

func TestAcceptsOnCallNotificationProfile(t *testing.T) {
	t.Parallel()

	credentials := config.Credentials{AppToken: "app", UserKey: "user", NotificationProfile: "on-call"}
	if err := credentials.Validate(); err != nil {
		t.Fatalf("Validate() rejected on-call notification profile: %v", err)
	}
}

func TestRejectsUnknownNotificationDetail(t *testing.T) {
	t.Parallel()

	err := (config.Credentials{AppToken: "app", UserKey: "user", NotificationDetail: "everything"}).Validate()
	if err == nil || !strings.Contains(err.Error(), "notification detail must be summary, minimal, or private") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestAcceptsPrivateNotificationDetail(t *testing.T) {
	t.Parallel()

	credentials := config.Credentials{AppToken: "app", UserKey: "user", NotificationDetail: "private"}
	if err := credentials.Validate(); err != nil {
		t.Fatalf("Validate() rejected private notification detail: %v", err)
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

func TestQuietHoursUseLocalDailyWindows(t *testing.T) {
	t.Parallel()

	location := time.FixedZone("PDT", -7*60*60)
	overnight := config.Credentials{
		AppToken: "app", UserKey: "user", QuietHoursStart: "22:00", QuietHoursEnd: "08:00",
	}
	for _, test := range []struct {
		hour, minute int
		want         bool
	}{
		{hour: 21, minute: 59, want: false},
		{hour: 22, minute: 0, want: true},
		{hour: 7, minute: 59, want: true},
		{hour: 8, minute: 0, want: false},
	} {
		now := time.Date(2026, time.July, 17, test.hour, test.minute, 0, 0, location)
		if got := overnight.IsQuietHours(now); got != test.want {
			t.Errorf("IsQuietHours(%s) = %t, want %t", now.Format("15:04"), got, test.want)
		}
	}
	daytime := config.Credentials{
		AppToken: "app", UserKey: "user", QuietHoursStart: "13:00", QuietHoursEnd: "15:00",
	}
	if !daytime.IsQuietHours(time.Date(2026, time.July, 17, 14, 0, 0, 0, location)) {
		t.Fatal("IsQuietHours() = false inside daytime window")
	}
	if daytime.IsQuietHours(time.Date(2026, time.July, 17, 16, 0, 0, 0, location)) {
		t.Fatal("IsQuietHours() = true outside daytime window")
	}
	for _, invalid := range []config.Credentials{
		{AppToken: "app", UserKey: "user", QuietHoursStart: "22:00"},
		{AppToken: "app", UserKey: "user", QuietHoursStart: "25:00", QuietHoursEnd: "08:00"},
		{AppToken: "app", UserKey: "user", QuietHoursStart: "08:00", QuietHoursEnd: "08:00"},
	} {
		if err := invalid.Validate(); err == nil {
			t.Fatalf("Validate() accepted invalid quiet hours: %#v", invalid)
		}
	}
}

func TestSilenceRulesMatchAgentProjectAndEvent(t *testing.T) {
	t.Parallel()

	credentials := config.Credentials{
		AppToken: "app", UserKey: "user",
		SilenceRules: []config.SilenceRule{{
			Agent: " Codex ", Project: " Private-Repo ", Event: "turn-complete",
		}},
	}
	if !credentials.IsSilenced("codex", "turn-complete", "private-repo") {
		t.Fatal("IsSilenced() = false for a case-insensitive matching agent and project")
	}
	if credentials.IsSilenced("codex", "approval-required", "private-repo") {
		t.Fatal("IsSilenced() suppressed an approval outside the rule event")
	}
	if credentials.IsSilenced("codex", "turn-complete", "another-repo") {
		t.Fatal("IsSilenced() matched only one half of a conjunctive rule")
	}

	credentials.SilenceRules = []config.SilenceRule{{Project: "private-repo", Event: "all"}}
	if !credentials.IsSilenced("gajae", "attention-required", "private-repo") {
		t.Fatal("IsSilenced() = false for an all-events project rule")
	}
}

func TestRejectsInvalidSilenceRules(t *testing.T) {
	t.Parallel()

	for _, rule := range []config.SilenceRule{
		{Event: "turn-complete"},
		{Agent: "codex", Event: "finished"},
		{Agent: "codex", Event: "approval-required"},
		{Project: "private-repo", Event: "attention-required"},
	} {
		credentials := config.Credentials{
			AppToken: "app", UserKey: "user", SilenceRules: []config.SilenceRule{rule},
		}
		if err := credentials.Validate(); err == nil {
			t.Fatalf("Validate() accepted invalid silence rule %#v", rule)
		}
	}
}
