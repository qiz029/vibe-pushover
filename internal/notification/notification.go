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
	EventTurnComplete     Event = "turn-complete"
	EventApprovalRequired Event = "approval-required"
)

type Message struct {
	Title    string
	Body     string
	Priority int
	Sound    string
	TTL      int
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
		if detail := firstLine(firstString(payload, "last_assistant_message", "message", "reason")); detail != "" {
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
	default:
		return Message{}, errors.New("event must be turn-complete or approval-required")
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
	tool := stringValue(payload, "tool_name")
	if input, ok := payload["tool_input"].(map[string]any); ok {
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
