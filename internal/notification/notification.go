package notification

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Event string

const (
	EventTurnComplete      Event = "turn-complete"
	EventApprovalRequired  Event = "approval-required"
	EventAttentionRequired Event = "attention-required"
)

type Message struct {
	Title     string
	Body      string
	URL       string
	URLTitle  string
	Timestamp int64
	Priority  int
	Sound     string
	TTL       int
	Retry     int
	Expire    int
	Monospace bool
}

func ShouldDeliver(event Event, profile string) bool {
	return (profile != "urgent" && profile != "on-call") || event != EventTurnComplete
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
	case "on-call":
		if event == EventApprovalRequired || event == EventAttentionRequired {
			message.Priority = 2
			message.Sound = "persistent"
			message.TTL = 0
			message.Retry = 60
			message.Expire = 900
		}
		return message, nil
	default:
		return Message{}, fmt.Errorf("unknown notification profile %q", profile)
	}
}

func ApplyDetail(message Message, event Event, detail string) (Message, error) {
	switch detail {
	case "", "summary":
		return message, nil
	case "minimal", "private":
		message.Monospace = false
		switch event {
		case EventTurnComplete:
			message.Body = "Turn completed."
		case EventApprovalRequired:
			message.Body = "Approval requested."
		case EventAttentionRequired:
			message.Body = "Agent needs your attention."
		default:
			return Message{}, errors.New("event must be turn-complete, approval-required, or attention-required")
		}
		if detail == "private" {
			if separator := strings.Index(message.Title, " · "); separator >= 0 {
				message.Title = message.Title[:separator]
			}
			message.URL = ""
			message.URLTitle = ""
		}
		return message, nil
	default:
		return Message{}, fmt.Errorf("notification detail must be summary, minimal, or private, got %q", detail)
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
	project := projectName(payloadProjectDirectory(payload))

	switch event {
	case EventTurnComplete:
		body := "Turn completed."
		if detail := completionDetail(agent, payload); detail != "" {
			body = detail
		}
		return withSupplementaryAction(Message{
			Title:     truncate(fmt.Sprintf("✓ %s finished%s", displayName(agent), titleProjectSuffix(project)), 250),
			Body:      truncate(body, 180),
			Timestamp: hookTimestamp(payload),
			Priority:  -1,
			Sound:     "none",
			TTL:       3600,
		}, event, payload), nil
	case EventApprovalRequired:
		body := "Approval requested."
		monospace := false
		if detail, command := approvalDetail(agent, payload); detail != "" {
			body = detail
			monospace = command
		}
		return withSupplementaryAction(Message{
			Title:     truncate(fmt.Sprintf("⚠ %s needs approval%s", displayName(agent), titleProjectSuffix(project)), 250),
			Body:      truncate(body, 300),
			Timestamp: hookTimestamp(payload),
			Priority:  1,
			Sound:     "persistent",
			TTL:       1800,
			Monospace: monospace,
		}, event, payload), nil
	case EventAttentionRequired:
		body := "Agent needs your attention."
		if detail := attentionDetail(agent, payload); detail != "" {
			body = detail
		}
		return withSupplementaryAction(Message{
			Title:     truncate(fmt.Sprintf("⚠ %s needs attention%s", displayName(agent), titleProjectSuffix(project)), 250),
			Body:      truncate(body, 300),
			Timestamp: hookTimestamp(payload),
			Priority:  1,
			Sound:     "persistent",
			TTL:       1800,
		}, event, payload), nil
	default:
		return Message{}, errors.New("event must be turn-complete, approval-required, or attention-required")
	}
}

func hookTimestamp(payload map[string]any) int64 {
	raw := payload["timestamp"]
	if value, ok := raw.(string); ok {
		value = strings.TrimSpace(value)
		if numeric, err := strconv.ParseInt(value, 10, 64); err == nil {
			return normalizeHookTimestamp(numeric)
		}
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return 0
		}
		return plausibleHookTimestamp(parsed.Unix())
	}
	var value int64
	switch typed := raw.(type) {
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			floating, floatErr := typed.Float64()
			if floatErr != nil {
				return 0
			}
			parsed = int64(floating)
		}
		value = parsed
	case float64:
		value = int64(typed)
	case int64:
		value = typed
	case int:
		value = int64(typed)
	default:
		return 0
	}
	return normalizeHookTimestamp(value)
}

func normalizeHookTimestamp(value int64) int64 {
	if value <= 0 {
		return 0
	}
	for _, divisor := range []int64{1, 1_000, 1_000_000, 1_000_000_000} {
		if normalized := plausibleHookTimestamp(value / divisor); normalized != 0 {
			return normalized
		}
	}
	return 0
}

func plausibleHookTimestamp(value int64) int64 {
	const (
		earliest = int64(946_684_800)   // 2000-01-01T00:00:00Z
		latest   = int64(4_102_444_800) // 2100-01-01T00:00:00Z
	)
	if value < earliest || value >= latest {
		return 0
	}
	return value
}

func attentionDetail(agent string, payload map[string]any) string {
	if agent == "junie" {
		errorClass := firstLine(stringValue(payload, "error"))
		details := firstLine(stringValue(payload, "error_details"))
		if errorClass != "" && details != "" {
			return errorClass + "\n" + details
		}
		if details != "" {
			return details
		}
	}
	return firstLine(firstString(payload, "message", "reason", "error", "title", "terminationReason", "status"))
}

func ProjectName(payload map[string]any) string {
	return projectName(payloadProjectDirectory(payload))
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
	if agent == "zcode" {
		if detail := completionLine(stringValue(payload, "responsePreview")); detail != "" {
			return detail
		}
	}
	if detail := completionLine(firstString(payload, "last_assistant_message", "lastAssistantMessage", "prompt_response", "assistant_response", "message", "reason")); detail != "" {
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

func payloadProjectDirectory(payload map[string]any) string {
	cwd := firstString(payload, "cwd", "working_dir", "workingDir")
	roots := payloadWorkspaceRoots(payload)
	if cwd != "" {
		best := ""
		for _, root := range roots {
			if pathContains(root, cwd) && len(root) > len(best) {
				best = root
			}
		}
		if best != "" {
			return best
		}
	}
	if len(roots) > 0 {
		return roots[0]
	}
	return cwd
}

func payloadWorkspaceRoots(payload map[string]any) []string {
	roots := make([]string, 0, 2)
	if workspace := firstString(payload, "workspaceRoot", "workspace_root", "workspace"); workspace != "" {
		roots = append(roots, workspace)
	}
	for _, key := range []string{"workspace_roots", "workspaceRoots", "workspacePaths"} {
		if rawRoots, ok := payload[key].([]any); ok && len(rawRoots) > 0 {
			for _, raw := range rawRoots {
				if root, ok := raw.(string); ok && strings.TrimSpace(root) != "" {
					roots = append(roots, strings.TrimSpace(root))
				}
			}
		}
		if rawRoots, ok := payload[key].([]string); ok && len(rawRoots) > 0 {
			for _, root := range rawRoots {
				if strings.TrimSpace(root) != "" {
					roots = append(roots, strings.TrimSpace(root))
				}
			}
		}
	}
	return roots
}

func pathContains(root, path string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
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
		"antigravity":    "Antigravity",
		"autohand":       "Autohand Code",
		"codebuddy":      "CodeBuddy",
		"coderabbit":     "CodeRabbit CLI",
		"codewhale":      "CodeWhale",
		"craft":          "Craft Agents",
		"cortex":         "Cortex Code",
		"dotcraft":       "DotCraft",
		"gajae":          "Gajae Code",
		"gitlab-duo":     "GitLab Duo",
		"grok":           "Grok Build",
		"gptme":          "gptme",
		"kilo":           "Kilo Code",
		"mimo":           "MiMo Code",
		"mini-swe-agent": "mini-SWE-agent",
		"mistral":        "Mistral Vibe",
		"omp":            "Oh My Pi",
		"openhands":      "OpenHands",
		"opencode":       "OpenCode",
		"opendev":        "OpenDev",
		"qwen":           "Qwen Code",
		"rovo":           "Rovo Dev",
		"swe-agent":      "SWE-agent",
		"tabnine":        "Tabnine",
		"trae":           "TRAE",
		"vibe-pushover":  "vibe-pushover",
		"vscode":         "VS Code",
		"workbuddy":      "WorkBuddy",
		"zcode":          "ZCode",
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

func approvalDetail(agent string, payload map[string]any) (string, bool) {
	if agent == "hermes" {
		extra, _ := payload["extra"].(map[string]any)
		description := approvalLabel(firstString(extra, "description", "message", "reason"))
		command := compactCommand(stringValue(extra, "command"))
		if description != "" && command != "" {
			return description + "\n" + command, true
		}
		if command != "" {
			return command, true
		}
		if description != "" {
			return description, false
		}
	}
	if agent == "gemini" {
		details, _ := payload["details"].(map[string]any)
		title := approvalLabel(stringValue(details, "title"))
		command := compactCommand(stringValue(details, "command"))
		if title != "" && command != "" {
			return title + "\n" + command, true
		}
		if command != "" {
			return command, true
		}
		if title != "" {
			return title, false
		}
	}
	tool := approvalLabel(firstString(payload, "tool_name", "toolName"))
	input, _ := payload["tool_input"].(map[string]any)
	if input == nil {
		input, _ = payload["toolArgs"].(map[string]any)
	}
	if input != nil {
		if command := compactCommand(stringValue(input, "command")); command != "" {
			if tool != "" {
				return tool + "\n" + command, true
			}
			return command, true
		}
	}
	detail := firstLine(firstString(payload, "message", "reason"))
	if tool != "" && detail != "" {
		return tool + "\n" + detail, false
	}
	if tool != "" {
		return tool, false
	}
	return detail, false
}

func compactCommand(value string) string {
	const maxCommandRunes = 220
	first := ""
	extraLines := 0
	for _, line := range strings.Split(value, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		if first == "" {
			first = line
			continue
		}
		extraLines++
	}
	if extraLines > 0 {
		label := "lines"
		if extraLines == 1 {
			label = "line"
		}
		suffix := fmt.Sprintf(" … (+%d %s)", extraLines, label)
		available := maxCommandRunes - utf8.RuneCountInString(suffix)
		runes := []rune(first)
		if len(runes) > available {
			first = string(runes[:available])
		}
		return first + suffix
	}
	return truncate(first, maxCommandRunes)
}

func approvalLabel(value string) string {
	return truncate(firstLine(value), 72)
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
