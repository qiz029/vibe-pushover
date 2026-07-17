package notification_test

import (
	"strings"
	"testing"

	"github.com/qiz029/vibe-pushover/internal/notification"
)

func TestBuildTurnCompleteNotification(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("codex", notification.EventTurnComplete, map[string]any{
		"cwd":                    "/Users/toddzheng/Workspace/golang/vibe-pushover",
		"last_assistant_message": "Implemented the compact notification.\n\nTests pass and the release is ready.",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Title != "✓ Codex finished · vibe-pushover" {
		t.Fatalf("Title = %q", got.Title)
	}
	wantBody := "Implemented the compact notification."
	if got.Body != wantBody {
		t.Fatalf("Body = %q, want %q", got.Body, wantBody)
	}
	if got.Priority != -1 {
		t.Fatalf("Priority = %d, want -1", got.Priority)
	}
	if got.Sound != "none" {
		t.Fatalf("Sound = %q, want none", got.Sound)
	}
	if got.TTL != 3600 {
		t.Fatalf("TTL = %d, want 3600", got.TTL)
	}
}

func TestBuildTurnCompleteNotificationTruncatesUnicodeSummary(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("pi", notification.EventTurnComplete, map[string]any{
		"last_assistant_message": strings.Repeat("好", 200),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if want := strings.Repeat("好", 179) + "…"; got.Body != want {
		t.Fatalf("Body = %q, want 180-rune summary", got.Body)
	}
}

func TestBuildApprovalNotification(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("claude", notification.EventApprovalRequired, map[string]any{
		"cwd":       "/tmp/vibe-pushover",
		"tool_name": "Bash",
		"tool_input": map[string]any{
			"command": "go test ./...",
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Title != "⚠ Claude needs approval · vibe-pushover" {
		t.Fatalf("Title = %q", got.Title)
	}
	wantBody := "Bash\ngo test ./..."
	if got.Body != wantBody {
		t.Fatalf("Body = %q, want %q", got.Body, wantBody)
	}
	if got.Priority != 1 {
		t.Fatalf("Priority = %d, want 1", got.Priority)
	}
	if got.Sound != "persistent" {
		t.Fatalf("Sound = %q, want persistent", got.Sound)
	}
	if got.TTL != 1800 {
		t.Fatalf("TTL = %d, want 1800", got.TTL)
	}
}

func TestBuildNotificationsAlwaysHaveABody(t *testing.T) {
	t.Parallel()

	for _, event := range []notification.Event{
		notification.EventTurnComplete,
		notification.EventApprovalRequired,
	} {
		got, err := notification.Build("codex", event, nil)
		if err != nil {
			t.Fatalf("Build(%q) error = %v", event, err)
		}
		if strings.TrimSpace(got.Body) == "" {
			t.Fatalf("Build(%q) returned an empty body", event)
		}
	}
}
