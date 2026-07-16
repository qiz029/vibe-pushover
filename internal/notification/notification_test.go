package notification_test

import (
	"testing"

	"github.com/qiz029/vibe-pushover/internal/notification"
)

func TestBuildTurnCompleteNotification(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("codex", notification.EventTurnComplete, map[string]any{
		"cwd":                    "/Users/toddzheng/Workspace/golang/vibe-pushover",
		"last_assistant_message": "Implemented the CLI and all tests pass.",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Title != "Agent turn complete" {
		t.Fatalf("Title = %q", got.Title)
	}
	wantBody := "codex completed a turn in vibe-pushover\nImplemented the CLI and all tests pass."
	if got.Body != wantBody {
		t.Fatalf("Body = %q, want %q", got.Body, wantBody)
	}
	if got.Priority != 0 {
		t.Fatalf("Priority = %d, want 0", got.Priority)
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
	if got.Title != "Agent needs approval" {
		t.Fatalf("Title = %q", got.Title)
	}
	wantBody := "claude needs approval in vibe-pushover\nBash: go test ./..."
	if got.Body != wantBody {
		t.Fatalf("Body = %q, want %q", got.Body, wantBody)
	}
	if got.Priority != 1 {
		t.Fatalf("Priority = %d, want 1", got.Priority)
	}
}
