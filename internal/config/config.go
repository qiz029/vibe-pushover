package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Credentials struct {
	AppToken            string `json:"app_token"`
	UserKey             string `json:"user_key"`
	Device              string `json:"device,omitempty"`
	NotificationProfile string `json:"notification_profile,omitempty"`
	SnoozedUntil        string `json:"snoozed_until,omitempty"`
	FocusUntil          string `json:"focus_until,omitempty"`
	TurnCompleteSound   string `json:"turn_complete_sound,omitempty"`
	ApprovalSound       string `json:"approval_required_sound,omitempty"`
	AttentionSound      string `json:"attention_required_sound,omitempty"`
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(dir, "vibe-pushover", "config.json"), nil
}

func Load(path string) (Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Credentials{}, fmt.Errorf("read config: %w", err)
	}

	var credentials Credentials
	if err := json.Unmarshal(data, &credentials); err != nil {
		return Credentials{}, fmt.Errorf("parse config: %w", err)
	}
	if err := credentials.Validate(); err != nil {
		return Credentials{}, err
	}
	return credentials, nil
}

func Save(path string, credentials Credentials) error {
	if err := credentials.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("set config permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

func (c Credentials) Validate() error {
	if c.AppToken == "" {
		return errors.New("app token is required")
	}
	if c.UserKey == "" {
		return errors.New("user key is required")
	}
	if c.Device != "" {
		for _, device := range strings.Split(c.Device, ",") {
			if !validDeviceName(device) {
				return fmt.Errorf("Pushover device names must be 1-25 characters using letters, numbers, underscore, or hyphen, got %q", device)
			}
		}
	}
	switch c.NotificationProfile {
	case "", "balanced", "quiet", "urgent", "watch":
	default:
		return fmt.Errorf("notification profile must be balanced, quiet, urgent, or watch, got %q", c.NotificationProfile)
	}
	if c.SnoozedUntil != "" {
		if _, err := time.Parse(time.RFC3339Nano, c.SnoozedUntil); err != nil {
			return fmt.Errorf("snoozed_until must be an RFC3339 timestamp: %w", err)
		}
	}
	if c.FocusUntil != "" {
		if _, err := time.Parse(time.RFC3339Nano, c.FocusUntil); err != nil {
			return fmt.Errorf("focus_until must be an RFC3339 timestamp: %w", err)
		}
	}
	for _, preference := range []struct {
		field string
		sound string
	}{
		{field: "turn_complete_sound", sound: c.TurnCompleteSound},
		{field: "approval_required_sound", sound: c.ApprovalSound},
		{field: "attention_required_sound", sound: c.AttentionSound},
	} {
		if !validSoundName(preference.sound) {
			return fmt.Errorf("%s must be default or a 1-64 character Pushover sound name using letters, numbers, underscore, or hyphen, got %q", preference.field, preference.sound)
		}
	}
	return nil
}

func (c Credentials) IsSnoozed(now time.Time) bool {
	until, err := time.Parse(time.RFC3339Nano, c.SnoozedUntil)
	return err == nil && now.Before(until)
}

func (c Credentials) IsFocused(now time.Time) bool {
	until, err := time.Parse(time.RFC3339Nano, c.FocusUntil)
	return err == nil && now.Before(until)
}

func validDeviceName(device string) bool {
	if len(device) == 0 || len(device) > 25 {
		return false
	}
	for _, char := range device {
		if (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func validSoundName(sound string) bool {
	if sound == "" || sound == "default" {
		return true
	}
	if len(sound) > 64 {
		return false
	}
	for _, char := range sound {
		if (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}
