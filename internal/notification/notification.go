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
}

func Build(agent string, event Event, payload map[string]any) (Message, error) {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		agent = "agent"
	}
	project := projectName(stringValue(payload, "cwd"))

	switch event {
	case EventTurnComplete:
		body := fmt.Sprintf("%s completed a turn%s", agent, projectSuffix(project))
		if detail := firstString(payload, "last_assistant_message", "message", "reason"); detail != "" {
			body += "\n" + detail
		}
		return Message{Title: "Agent turn complete", Body: truncate(body, 1024), Priority: 0}, nil
	case EventApprovalRequired:
		body := fmt.Sprintf("%s needs approval%s", agent, projectSuffix(project))
		if detail := approvalDetail(payload); detail != "" {
			body += "\n" + detail
		}
		return Message{Title: "Agent needs approval", Body: truncate(body, 1024), Priority: 1}, nil
	default:
		return Message{}, errors.New("event must be turn-complete or approval-required")
	}
}

func approvalDetail(payload map[string]any) string {
	tool := stringValue(payload, "tool_name")
	if input, ok := payload["tool_input"].(map[string]any); ok {
		if command := stringValue(input, "command"); command != "" {
			if tool != "" {
				return tool + ": " + command
			}
			return command
		}
	}
	detail := firstString(payload, "message", "reason")
	if tool != "" && detail != "" {
		return tool + ": " + detail
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

func projectSuffix(project string) string {
	if project == "" || project == "." {
		return ""
	}
	return " in " + project
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
