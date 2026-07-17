package notification

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type Event string

const (
	EventTurnComplete      Event = "turn-complete"
	EventApprovalRequired  Event = "approval-required"
	EventAttentionRequired Event = "attention-required"
)

type Message struct {
	Title    string
	Body     string
	Priority int
	Sound    string
	TTL      int
}

func ApplyProfile(message Message, event Event, profile string) (Message, error) {
	switch profile {
	case "", "balanced":
		return message, nil
	case "quiet":
		message.Priority = 0
		message.Sound = "none"
		return message, nil
	case "watch":
		if event == EventTurnComplete {
			message.Priority = 0
			message.Sound = "pushover"
		}
		return message, nil
	default:
		return Message{}, fmt.Errorf("unknown notification profile %q", profile)
	}
}

func Build(agent string, event Event, payload map[string]any) (Message, error) {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		agent = "agent"
	}
	project := projectName(stringValue(payload, "cwd"))

	switch event {
	case EventTurnComplete:
		body := "Turn completed."
		if detail := firstLine(firstString(payload, "last_assistant_message", "prompt_response", "message", "reason")); detail != "" {
			body = detail
		}
		return Message{
			Title:    truncate(fmt.Sprintf("✓ %s finished%s", displayName(agent), titleProjectSuffix(project)), 250),
			Body:     truncate(body, 180),
			Priority: -1,
			Sound:    "none",
			TTL:      3600,
		}, nil
	case EventApprovalRequired:
		body := "Approval requested."
		if detail := approvalDetail(payload); detail != "" {
			body = detail
		}
		return Message{
			Title:    truncate(fmt.Sprintf("⚠ %s needs approval%s", displayName(agent), titleProjectSuffix(project)), 250),
			Body:     truncate(body, 300),
			Priority: 1,
			Sound:    "persistent",
			TTL:      1800,
		}, nil
	case EventAttentionRequired:
		body := "Agent needs your attention."
		if detail := firstLine(firstString(payload, "message", "reason", "title")); detail != "" {
			body = detail
		}
		return Message{
			Title:    truncate(fmt.Sprintf("⚠ %s needs attention%s", displayName(agent), titleProjectSuffix(project)), 250),
			Body:     truncate(body, 300),
			Priority: 1,
			Sound:    "persistent",
			TTL:      1800,
		}, nil
	default:
		return Message{}, errors.New("event must be turn-complete, approval-required, or attention-required")
	}
}

func firstLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return strings.Join(strings.Fields(line), " ")
		}
	}
	return ""
}

func displayName(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return "Agent"
	}
	return strings.ToUpper(string(runes[0])) + string(runes[1:])
}

func titleProjectSuffix(project string) string {
	if project == "" || project == "." {
		return ""
	}
	return " · " + project
}

func approvalDetail(payload map[string]any) string {
	tool := firstString(payload, "tool_name", "toolName")
	input, _ := payload["tool_input"].(map[string]any)
	if input == nil {
		input, _ = payload["toolArgs"].(map[string]any)
	}
	if input != nil {
		if command := stringValue(input, "command"); command != "" {
			if tool != "" {
				return tool + "\n" + command
			}
			return command
		}
	}
	detail := firstString(payload, "message", "reason")
	if tool != "" && detail != "" {
		return tool + "\n" + detail
	}
	if tool != "" {
		return tool
	}
	return detail
}

func projectName(cwd string) string {
	if cwd == "" {
		return ""
	}
	return filepath.Base(filepath.Clean(cwd))
}

func firstString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(payload, key); value != "" {
			return value
		}
	}
	return ""
}

func stringValue(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func truncate(value string, maxRunes int) string {
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes-1]) + "…"
}
