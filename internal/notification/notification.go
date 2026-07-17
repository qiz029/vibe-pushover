package notification

import (
	"errors"
	"fmt"
	"net/url"
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
	URL      string
	URLTitle string
	Priority int
	Sound    string
	TTL      int
}

func ShouldDeliver(event Event, profile string) bool {
	return profile != "urgent" || event != EventTurnComplete
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
	case "urgent":
		return message, nil
	default:
		return Message{}, fmt.Errorf("unknown notification profile %q", profile)
	}
}

func Build(agent string, event Event, payload map[string]any) (Message, error) {
	agent = strings.TrimSpace(agent)
	if agent == "copilot-vscode" {
		if stringValue(payload, "hook_event_name") != "" {
			agent = "VS Code"
		} else {
			agent = "Copilot CLI"
		}
	}
	if agent == "" {
		agent = "agent"
	}
	project := projectName(payloadWorkingDirectory(payload))

	switch event {
	case EventTurnComplete:
		body := "Turn completed."
		if detail := completionDetail(agent, payload); detail != "" {
			body = detail
		}
		return withSupplementaryAction(Message{
			Title:    truncate(fmt.Sprintf("✓ %s finished%s", displayName(agent), titleProjectSuffix(project)), 250),
			Body:     truncate(body, 180),
			Priority: -1,
			Sound:    "none",
			TTL:      3600,
		}, event, payload), nil
	case EventApprovalRequired:
		body := "Approval requested."
		if detail := approvalDetail(agent, payload); detail != "" {
			body = detail
		}
		return withSupplementaryAction(Message{
			Title:    truncate(fmt.Sprintf("⚠ %s needs approval%s", displayName(agent), titleProjectSuffix(project)), 250),
			Body:     truncate(body, 300),
			Priority: 1,
			Sound:    "persistent",
			TTL:      1800,
		}, event, payload), nil
	case EventAttentionRequired:
		body := "Agent needs your attention."
		if detail := firstLine(firstString(payload, "message", "reason", "error", "title")); detail != "" {
			body = detail
		}
		return withSupplementaryAction(Message{
			Title:    truncate(fmt.Sprintf("⚠ %s needs attention%s", displayName(agent), titleProjectSuffix(project)), 250),
			Body:     truncate(body, 300),
			Priority: 1,
			Sound:    "persistent",
			TTL:      1800,
		}, event, payload), nil
	default:
		return Message{}, errors.New("event must be turn-complete, approval-required, or attention-required")
	}
}

func withSupplementaryAction(message Message, event Event, payload map[string]any) Message {
	rawURL := supplementaryURL(payload)
	if rawURL == "" {
		return message
	}
	message.URL = rawURL
	message.URLTitle = firstString(payload, "url_title", "urlTitle")
	if message.URLTitle == "" {
		if event == EventTurnComplete {
			message.URLTitle = "Open result"
		} else {
			message.URLTitle = "Open agent"
		}
	}
	message.URLTitle = truncate(message.URLTitle, 100)
	return message
}

func supplementaryURL(payload map[string]any) string {
	for _, key := range []string{"url", "session_url", "sessionUrl", "web_url", "webUrl", "details_url", "detailsUrl"} {
		rawURL, _ := payload[key].(string)
		rawURL = strings.TrimSpace(rawURL)
		parsed, err := url.Parse(rawURL)
		if err == nil && utf8.RuneCountInString(rawURL) <= 512 && parsed.Host != "" && (parsed.Scheme == "https" || parsed.Scheme == "http") {
			return rawURL
		}
	}
	return ""
}

func completionDetail(agent string, payload map[string]any) string {
	if detail := completionLine(firstString(payload, "last_assistant_message", "prompt_response", "assistant_response", "message", "reason")); detail != "" {
		return detail
	}
	if agent == "hermes" {
		extra, _ := payload["extra"].(map[string]any)
		if detail := completionLine(firstString(extra, "assistant_response", "message", "reason")); detail != "" {
			return detail
		}
	}
	if agent == "cline" {
		if taskComplete, ok := payload["taskComplete"].(map[string]any); ok {
			if taskMetadata, ok := taskComplete["taskMetadata"].(map[string]any); ok {
				if detail := completionLine(stringValue(taskMetadata, "result")); detail != "" {
					return detail
				}
			}
		}
		if turn, ok := payload["turn"].(map[string]any); ok {
			if detail := completionLine(stringValue(turn, "outputText")); detail != "" {
				return detail
			}
		}
	}
	if agent == "windsurf" {
		if toolInfo, ok := payload["tool_info"].(map[string]any); ok {
			return lastContentLine(stringValue(toolInfo, "response"))
		}
	}
	if agent == "auggie" {
		if conversation, ok := payload["conversation"].(map[string]any); ok {
			return completionLine(stringValue(conversation, "agentTextResponse"))
		}
	}
	return ""
}

func completionLine(value string) string {
	fallback := ""
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if fallback == "" {
				fallback = strings.Join(strings.Fields(strings.TrimSpace(strings.TrimLeft(line, "#"))), " ")
			}
			continue
		}
		return strings.Join(strings.Fields(line), " ")
	}
	return fallback
}

func payloadWorkingDirectory(payload map[string]any) string {
	if cwd := stringValue(payload, "cwd"); cwd != "" {
		return cwd
	}
	if workingDir := firstString(payload, "working_dir", "workingDir"); workingDir != "" {
		return workingDir
	}
	if workspace := firstString(payload, "workspaceRoot", "workspace_root", "workspace"); workspace != "" {
		return workspace
	}
	for _, key := range []string{"workspace_roots", "workspaceRoots", "workspacePaths"} {
		if roots, ok := payload[key].([]any); ok && len(roots) > 0 {
			for _, raw := range roots {
				if root, ok := raw.(string); ok && strings.TrimSpace(root) != "" {
					return strings.TrimSpace(root)
				}
			}
		}
		if roots, ok := payload[key].([]string); ok && len(roots) > 0 {
			for _, root := range roots {
				if strings.TrimSpace(root) != "" {
					return strings.TrimSpace(root)
				}
			}
		}
	}
	return ""
}

func lastContentLine(value string) string {
	lines := strings.Split(value, "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return strings.Join(strings.Fields(line), " ")
	}
	return ""
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
	if name, ok := map[string]string{
		"antigravity":   "Antigravity",
		"codebuddy":     "CodeBuddy",
		"codewhale":     "CodeWhale",
		"cortex":        "Cortex Code",
		"grok":          "Grok Build",
		"mimo":          "MiMo Code",
		"mistral":       "Mistral Vibe",
		"omp":           "Oh My Pi",
		"openhands":     "OpenHands",
		"opencode":      "OpenCode",
		"qwen":          "Qwen Code",
		"rovo":          "Rovo Dev",
		"tabnine":       "Tabnine",
		"trae":          "TRAE",
		"vibe-pushover": "vibe-pushover",
		"vscode":        "VS Code",
	}[strings.ToLower(value)]; ok {
		return name
	}
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

func approvalDetail(agent string, payload map[string]any) string {
	if agent == "hermes" {
		extra, _ := payload["extra"].(map[string]any)
		description := firstString(extra, "description", "message", "reason")
		command := stringValue(extra, "command")
		if description != "" && command != "" {
			return description + "\n" + command
		}
		if command != "" {
			return command
		}
		if description != "" {
			return description
		}
	}
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
