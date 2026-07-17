package notification_test

import (
	"encoding/json"
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

func TestBuildAddsSupplementaryActionForHTTPSURL(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("mistral", notification.EventTurnComplete, map[string]any{
		"cwd":         "/tmp/demo",
		"session_url": "https://example.com/agent/sessions/42",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.URL != "https://example.com/agent/sessions/42" || got.URLTitle != "Open result" {
		t.Fatalf("supplementary action = %q (%q)", got.URL, got.URLTitle)
	}
}

func TestBuildIgnoresUnsafeSupplementaryURL(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("codex", notification.EventApprovalRequired, map[string]any{
		"url": "javascript:alert(document.domain)",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.URL != "" || got.URLTitle != "" {
		t.Fatalf("unsafe supplementary action = %q (%q)", got.URL, got.URLTitle)
	}
}

func TestBuildAttentionUsesTerminationReason(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("antigravity", notification.EventAttentionRequired, map[string]any{
		"terminationReason": "max_steps_exceeded",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Body != "max_steps_exceeded" {
		t.Fatalf("Body = %q, want termination reason", got.Body)
	}
}

func TestBuildJunieFailureAttentionUsesClassAndDetails(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("junie", notification.EventAttentionRequired, map[string]any{
		"cwd":           "/tmp/demo",
		"error":         "rate_limit",
		"error_details": "429 Too Many Requests from the configured provider",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Title != "⚠ Junie needs attention · demo" {
		t.Fatalf("Title = %q", got.Title)
	}
	if got.Body != "rate_limit\n429 Too Many Requests from the configured provider" {
		t.Fatalf("Body = %q", got.Body)
	}
}

func TestBuildUsesSafeSupplementaryURLAfterUnsafeCandidate(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("codex", notification.EventTurnComplete, map[string]any{
		"url":         "file:///tmp/local-result",
		"session_url": "https://example.com/agent/sessions/42",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.URL != "https://example.com/agent/sessions/42" || got.URLTitle != "Open result" {
		t.Fatalf("supplementary action = %q (%q)", got.URL, got.URLTitle)
	}
}

func TestBuildIgnoresOverlongSupplementaryURL(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("codex", notification.EventTurnComplete, map[string]any{
		"url": "https://example.com/" + strings.Repeat("a", 500),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.URL != "" || got.URLTitle != "" {
		t.Fatalf("overlong supplementary action = %q (%q)", got.URL, got.URLTitle)
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

func TestBuildTurnCompleteNotificationSkipsMarkdownHeading(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("amp", notification.EventTurnComplete, map[string]any{
		"last_assistant_message": "## Summary\n\nImplemented Amp notifications and all tests pass.",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Body != "Implemented Amp notifications and all tests pass." {
		t.Fatalf("Body = %q", got.Body)
	}
}

func TestBuildTurnCompleteNotificationUsesHeadingWhenItIsOnlyContent(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("kiro", notification.EventTurnComplete, map[string]any{
		"last_assistant_message": "# Done",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Body != "Done" {
		t.Fatalf("Body = %q", got.Body)
	}
}

func TestBuildKiroCompletionUsesAssistantResponse(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("kiro", notification.EventTurnComplete, map[string]any{
		"assistant_response": "## Result\n\nImplemented the requested Kiro hook.",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Body != "Implemented the requested Kiro hook." {
		t.Fatalf("Body = %q", got.Body)
	}
}

func TestBuildZCodeCompletionUsesResponsePreview(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("zcode", notification.EventTurnComplete, map[string]any{
		"cwd":             "/tmp/zcode-demo",
		"message":         "Stop hook invoked",
		"responsePreview": "## Result\n\nImplemented ZCode notifications.\nAll tests pass.",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Title != "✓ ZCode finished · zcode-demo" || got.Body != "Implemented ZCode notifications." {
		t.Fatalf("ZCode notification = %#v", got)
	}
}

func TestBuildClineIDECompletionUsesTaskResultAndWorkspace(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("cline", notification.EventTurnComplete, map[string]any{
		"workspaceRoots": []any{"/Users/toddzheng/Workspace/golang/demo"},
		"taskComplete": map[string]any{
			"taskMetadata": map[string]any{
				"result": "## Result\n\nImplemented the requested Cline integration.\nAll tests pass.",
			},
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Title != "✓ Cline finished · demo" || got.Body != "Implemented the requested Cline integration." {
		t.Fatalf("Cline IDE notification = %#v", got)
	}
}

func TestBuildClineCLICompletionUsesTurnOutput(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("cline", notification.EventTurnComplete, map[string]any{
		"workspaceRoots": []string{"/tmp/cline-cli-demo"},
		"turn": map[string]any{
			"outputText": "Completed the CLI task successfully.\nAdditional implementation detail.",
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Title != "✓ Cline finished · cline-cli-demo" || got.Body != "Completed the CLI task successfully." {
		t.Fatalf("Cline CLI notification = %#v", got)
	}
}

func TestBuildOpenHandsCompletionUsesWorkingDirectory(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("openhands", notification.EventTurnComplete, map[string]any{
		"event_type":  "Stop",
		"working_dir": "/tmp/demo",
		"metadata":    map[string]any{"reason": "Agent finished"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Title != "✓ OpenHands finished · demo" {
		t.Fatalf("Title = %q", got.Title)
	}
	if got.Body != "Turn completed." {
		t.Fatalf("Body = %q", got.Body)
	}
}

func TestBuildHermesCompletionUsesNestedAssistantResponse(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("hermes", notification.EventTurnComplete, map[string]any{
		"cwd":   "/tmp/demo",
		"extra": map[string]any{"assistant_response": "## Result\n\nImplemented Hermes notifications.\nTests pass."},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Title != "✓ Hermes finished · demo" || got.Body != "Implemented Hermes notifications." {
		t.Fatalf("Hermes notification = %#v", got)
	}
}

func TestBuildHermesApprovalUsesNestedCommandAndDescription(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("hermes", notification.EventApprovalRequired, map[string]any{
		"cwd":   "/tmp/demo",
		"extra": map[string]any{"surface": "cli", "command": "git push origin main", "description": "network write"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Title != "⚠ Hermes needs approval · demo" || got.Body != "network write\ngit push origin main" {
		t.Fatalf("Hermes approval notification = %#v", got)
	}
}

func TestBuildNonHermesCompletionIgnoresIncidentalExtra(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("windsurf", notification.EventTurnComplete, map[string]any{
		"extra":     map[string]any{"message": "internal metadata"},
		"tool_info": map[string]any{"response": "Planner output\n\nCanonical Windsurf result."},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Body != "Canonical Windsurf result." {
		t.Fatalf("Body = %q", got.Body)
	}
}

func TestBuildNonHermesApprovalIgnoresIncidentalExtra(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("claude", notification.EventApprovalRequired, map[string]any{
		"extra":      map[string]any{"description": "internal metadata", "command": "hidden"},
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "go test ./..."},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Body != "Bash\ngo test ./..." {
		t.Fatalf("Body = %q", got.Body)
	}
}

func TestBuildGeminiApprovalUsesToolPermissionDetails(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("gemini", notification.EventApprovalRequired, map[string]any{
		"cwd":               "/tmp/vibe-pushover",
		"notification_type": "ToolPermission",
		"message":           "Confirm ShellTool execution",
		"details": map[string]any{
			"type":    "exec",
			"title":   "ShellTool execution",
			"command": "go test ./...",
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Title != "⚠ Gemini needs approval · vibe-pushover" {
		t.Fatalf("Title = %q", got.Title)
	}
	if got.Body != "ShellTool execution\ngo test ./..." {
		t.Fatalf("Body = %q", got.Body)
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

func TestBuildApprovalNotificationAcceptsCamelCasePayload(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("copilot", notification.EventApprovalRequired, map[string]any{
		"toolName": "bash",
		"toolArgs": map[string]any{"command": "git push origin main"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Body != "bash\ngit push origin main" {
		t.Fatalf("Body = %q", got.Body)
	}
}

func TestBuildNotificationsAlwaysHaveABody(t *testing.T) {
	t.Parallel()

	for _, event := range []notification.Event{
		notification.EventTurnComplete,
		notification.EventApprovalRequired,
		notification.EventAttentionRequired,
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

func TestBuildPreservesHookEventTimestamp(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("copilot", notification.EventTurnComplete, map[string]any{
		"timestamp": float64(1_752_761_234_567),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Timestamp != 1_752_761_234 {
		t.Fatalf("Timestamp = %d, want Unix seconds", got.Timestamp)
	}
}

func TestBuildNormalizesSupportedUnixTimestampRepresentations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		timestamp any
		want      int64
	}{
		{name: "seconds", timestamp: float64(1_752_761_234), want: 1_752_761_234},
		{name: "early milliseconds", timestamp: float64(946_684_800_000), want: 946_684_800},
		{name: "microseconds", timestamp: float64(1_752_761_234_567_000), want: 1_752_761_234},
		{name: "nanoseconds", timestamp: json.Number("1752761234567000000"), want: 1_752_761_234},
		{name: "digit string", timestamp: "1752761234", want: 1_752_761_234},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := notification.Build("custom-agent", notification.EventTurnComplete, map[string]any{"timestamp": tt.timestamp})
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if got.Timestamp != tt.want {
				t.Fatalf("Timestamp = %d, want %d", got.Timestamp, tt.want)
			}
		})
	}
}

func TestBuildParsesRFC3339HookTimestamp(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("vscode", notification.EventTurnComplete, map[string]any{
		"timestamp": "2025-07-17T12:34:56.789Z",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Timestamp != 1_752_755_696 {
		t.Fatalf("Timestamp = %d, want RFC3339 Unix seconds", got.Timestamp)
	}
}

func TestBuildIgnoresNonEventTimestamp(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("custom-agent", notification.EventTurnComplete, map[string]any{
		"timestamp": float64(42),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Timestamp != 0 {
		t.Fatalf("Timestamp = %d, want an implausible event time ignored", got.Timestamp)
	}
}

func TestApplyDetailMinimalHidesHookPayloadContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		event notification.Event
		body  string
	}{
		{event: notification.EventTurnComplete, body: "Turn completed."},
		{event: notification.EventApprovalRequired, body: "Approval requested."},
		{event: notification.EventAttentionRequired, body: "Agent needs your attention."},
	}
	for _, test := range tests {
		message := notification.Message{
			Title: "keep project context", Body: "sensitive hook detail", URL: "https://example.com/session", URLTitle: "Open agent",
		}
		got, err := notification.ApplyDetail(message, test.event, "minimal")
		if err != nil {
			t.Fatalf("ApplyDetail(%q) error = %v", test.event, err)
		}
		if got.Body != test.body {
			t.Errorf("ApplyDetail(%q) body = %q, want %q", test.event, got.Body, test.body)
		}
		if got.Title != message.Title || got.URL != message.URL || got.URLTitle != message.URLTitle {
			t.Errorf("ApplyDetail(%q) changed notification context: %#v", test.event, got)
		}
	}
}

func TestApplyDetailPrivateHidesLockScreenContextAndAction(t *testing.T) {
	t.Parallel()

	message := notification.Message{
		Title:    "✓ Autohand Code finished · private · project",
		Body:     "Implemented confidential authentication changes.",
		URL:      "https://example.com/private-session",
		URLTitle: "Open result",
	}
	got, err := notification.ApplyDetail(message, notification.EventTurnComplete, "private")
	if err != nil {
		t.Fatalf("ApplyDetail() error = %v", err)
	}
	if got.Title != "✓ Autohand Code finished" || got.Body != "Turn completed." {
		t.Fatalf("private title/body = %q / %q", got.Title, got.Body)
	}
	if got.URL != "" || got.URLTitle != "" {
		t.Fatalf("private detail kept supplementary action: %#v", got)
	}
}

func TestBuildAttentionNotification(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("droid", notification.EventAttentionRequired, map[string]any{
		"cwd": "/tmp/demo", "message": "Droid is waiting for your input",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Title != "⚠ Droid needs attention · demo" || got.Body != "Droid is waiting for your input" {
		t.Fatalf("attention notification = %#v", got)
	}
}

func TestBuildGeminiCompletionUsesPromptResponse(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("gemini", notification.EventTurnComplete, map[string]any{
		"prompt_response": "Implemented support for Gemini CLI.\nTests pass.",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Body != "Implemented support for Gemini CLI." {
		t.Fatalf("Body = %q", got.Body)
	}
}

func TestBuildWindsurfCompletionUsesLastResponseLine(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("windsurf", notification.EventTurnComplete, map[string]any{
		"cwd": "/tmp/demo",
		"tool_info": map[string]any{
			"response": "### Planner Response\n\nI'll help you create that file.\n\n*Created file `/tmp/demo/main.go`*\n\n### Planner Response\n\nThe file has been created successfully.",
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Title != "✓ Windsurf finished · demo" || got.Body != "The file has been created successfully." {
		t.Fatalf("Windsurf notification = %#v", got)
	}
}

func TestBuildAuggieCompletionUsesConversationAndWorkspaceRoot(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("auggie", notification.EventTurnComplete, map[string]any{
		"agent_stop_cause": "end_turn",
		"workspace_roots":  []any{"/tmp/demo"},
		"conversation": map[string]any{
			"agentTextResponse": "Implemented the requested changes.\nAll tests pass.",
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Title != "✓ Auggie finished · demo" || got.Body != "Implemented the requested changes." {
		t.Fatalf("Auggie notification = %#v", got)
	}
}

func TestBuildUsesFirstNonEmptyWorkspaceRoot(t *testing.T) {
	t.Parallel()

	got, err := notification.Build("kiro", notification.EventTurnComplete, map[string]any{
		"workspace_roots": []any{"", "/tmp/demo"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Title != "✓ Kiro finished · demo" {
		t.Fatalf("Title = %q", got.Title)
	}
}

func TestBuildDetectsSharedCopilotAndVSCodeSource(t *testing.T) {
	t.Parallel()

	for name, payload := range map[string]map[string]any{
		"Copilot CLI": {"sessionId": "copilot-session", "cwd": "/tmp/demo"},
		"VS Code":     {"hook_event_name": "Stop", "session_id": "vscode-session", "cwd": "/tmp/demo"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := notification.Build("copilot-vscode", notification.EventTurnComplete, payload)
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if !strings.HasPrefix(got.Title, "✓ "+name+" finished") {
				t.Fatalf("title = %q, want %s source", got.Title, name)
			}
		})
	}
}

func TestBuildUsesProductNamesInNotificationTitles(t *testing.T) {
	t.Parallel()

	for agent, want := range map[string]string{
		"autohand":  "✓ Autohand Code finished · demo",
		"craft":     "✓ Craft Agents finished · demo",
		"cortex":    "✓ Cortex Code finished · demo",
		"dotcraft":  "✓ DotCraft finished · demo",
		"kilo":      "✓ Kilo Code finished · demo",
		"mimo":      "✓ MiMo Code finished · demo",
		"mistral":   "✓ Mistral Vibe finished · demo",
		"omp":       "✓ Oh My Pi finished · demo",
		"openhands": "✓ OpenHands finished · demo",
		"opencode":  "✓ OpenCode finished · demo",
		"qwen":      "✓ Qwen Code finished · demo",
		"trae":      "✓ TRAE finished · demo",
		"vscode":    "✓ VS Code finished · demo",
	} {
		got, err := notification.Build(agent, notification.EventTurnComplete, map[string]any{"cwd": "/tmp/demo"})
		if err != nil {
			t.Fatalf("Build(%q) error = %v", agent, err)
		}
		if got.Title != want {
			t.Errorf("Build(%q) title = %q, want %q", agent, got.Title, want)
		}
	}
}

func TestUrgentProfileOnlySuppressesCompletedTurns(t *testing.T) {
	t.Parallel()

	if notification.ShouldDeliver(notification.EventTurnComplete, "urgent") {
		t.Fatal("urgent profile delivers completed turns")
	}
	for _, event := range []notification.Event{notification.EventApprovalRequired, notification.EventAttentionRequired} {
		if !notification.ShouldDeliver(event, "urgent") {
			t.Fatalf("urgent profile suppresses %s", event)
		}
	}
	message, err := notification.ApplyProfile(notification.Message{Priority: 1, Sound: "persistent"}, notification.EventApprovalRequired, "urgent")
	if err != nil {
		t.Fatalf("ApplyProfile() error = %v", err)
	}
	if message.Priority != 1 || message.Sound != "persistent" {
		t.Fatalf("urgent approval delivery = %#v", message)
	}
}

func TestOnCallProfileSuppressesCompletionAndEscalatesBlockers(t *testing.T) {
	t.Parallel()

	if notification.ShouldDeliver(notification.EventTurnComplete, "on-call") {
		t.Fatal("on-call profile delivers completed turns")
	}
	for _, event := range []notification.Event{notification.EventApprovalRequired, notification.EventAttentionRequired} {
		if !notification.ShouldDeliver(event, "on-call") {
			t.Fatalf("on-call profile suppresses %s", event)
		}
		message, err := notification.ApplyProfile(notification.Message{
			Priority: 1, Sound: "persistent", TTL: 1800,
		}, event, "on-call")
		if err != nil {
			t.Fatalf("ApplyProfile(%s) error = %v", event, err)
		}
		if message.Priority != 2 || message.Sound != "persistent" || message.TTL != 0 || message.Retry != 60 || message.Expire != 900 {
			t.Fatalf("on-call %s delivery = %#v", event, message)
		}
	}
}

func TestBuildFormatsGrokBuildHookPayload(t *testing.T) {
	t.Parallel()

	completion, err := notification.Build("grok", notification.EventTurnComplete, map[string]any{
		"workspaceRoot": "/tmp/grok-project",
	})
	if err != nil {
		t.Fatalf("Build(completion) error = %v", err)
	}
	if completion.Title != "✓ Grok Build finished · grok-project" {
		t.Fatalf("completion title = %q", completion.Title)
	}

	failure, err := notification.Build("grok", notification.EventAttentionRequired, map[string]any{
		"workspaceRoot": "/tmp/grok-project",
		"error":         "Model request timed out",
	})
	if err != nil {
		t.Fatalf("Build(failure) error = %v", err)
	}
	if failure.Title != "⚠ Grok Build needs attention · grok-project" || failure.Body != "Model request timed out" {
		t.Fatalf("failure notification = %#v", failure)
	}
}

func TestApplyQuietProfileSilencesApproval(t *testing.T) {
	t.Parallel()

	message, err := notification.Build("codex", notification.EventApprovalRequired, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	got, err := notification.ApplyProfile(message, notification.EventApprovalRequired, "quiet")
	if err != nil {
		t.Fatalf("ApplyProfile() error = %v", err)
	}
	if got.Priority != 0 || got.Sound != "none" {
		t.Fatalf("quiet approval style = priority %d, sound %q", got.Priority, got.Sound)
	}
}

func TestApplyWatchProfileMakesCompletionNoticeable(t *testing.T) {
	t.Parallel()

	message, err := notification.Build("codex", notification.EventTurnComplete, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	got, err := notification.ApplyProfile(message, notification.EventTurnComplete, "watch")
	if err != nil {
		t.Fatalf("ApplyProfile() error = %v", err)
	}
	if got.Priority != 0 || got.Sound != "pushover" {
		t.Fatalf("watch completion style = priority %d, sound %q", got.Priority, got.Sound)
	}
}
