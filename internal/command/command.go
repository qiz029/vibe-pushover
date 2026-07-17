package command

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/qiz029/vibe-pushover/internal/config"
	"github.com/qiz029/vibe-pushover/internal/dedupe"
	"github.com/qiz029/vibe-pushover/internal/hooks"
	"github.com/qiz029/vibe-pushover/internal/notification"
	"github.com/qiz029/vibe-pushover/internal/pushover"
	"github.com/urfave/cli/v3"
)

type Options struct {
	Version           string
	Stdin             io.Reader
	Stdout            io.Writer
	Stderr            io.Writer
	HTTPClient        *http.Client
	Endpoint          string
	Executable        string
	DedupePath        string
	DefaultConfigPath string
	Now               func() time.Time
}

func New(options Options) *cli.Command {
	options = withDefaults(options)
	return &cli.Command{
		Name:                  "vibe-pushover",
		Usage:                 "send Pushover notifications from local agent hooks",
		Version:               options.Version,
		EnableShellCompletion: true,
		Reader:                options.Stdin,
		Writer:                options.Stdout,
		ErrWriter:             options.Stderr,
		Commands: []*cli.Command{
			setupCommand(options),
			profileCommand(options),
			snoozeCommand(options),
			focusCommand(options),
			quietHoursCommand(options),
			silenceCommand(options),
			deviceCommand(options),
			soundCommand(options),
			agentsCommand(options),
			installCommand(options),
			previewCommand(options),
			notifyCommand(options),
			testCommand(options),
		},
	}
}

func silenceCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:  "silence",
		Usage: "silence notifications matching agent and project rules",
		Flags: []cli.Flag{configFlag()},
		Action: func(_ context.Context, cmd *cli.Command) error {
			path, err := configPath(cmd.String("config"))
			if err != nil {
				return err
			}
			credentials, err := config.Load(path)
			if err != nil {
				return err
			}
			if len(credentials.SilenceRules) == 0 {
				_, err = fmt.Fprintln(options.Stdout, "No silence rules configured")
				return err
			}
			for index, rule := range credentials.SilenceRules {
				if _, err := fmt.Fprintf(options.Stdout, "%d: event=%s agent=%s project=%s\n",
					index+1, rule.Event, ruleValue(rule.Agent), ruleValue(rule.Project)); err != nil {
					return err
				}
			}
			return nil
		},
		Commands: []*cli.Command{{
			Name:  "add",
			Usage: "add a silence rule; completion notifications are the safe default",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "agent", Usage: "match this agent name"},
				&cli.StringFlag{Name: "project", Usage: "match this project directory name"},
				&cli.StringFlag{Name: "event", Usage: "turn-complete or all", Value: "turn-complete"},
				configFlag(),
			},
			Action: func(_ context.Context, cmd *cli.Command) error {
				path, err := configPath(cmd.String("config"))
				if err != nil {
					return err
				}
				credentials, err := config.Load(path)
				if err != nil {
					return err
				}
				rule := config.SilenceRule{
					Agent: strings.ToLower(strings.TrimSpace(cmd.String("agent"))), Project: strings.TrimSpace(cmd.String("project")),
					Event: strings.ToLower(strings.TrimSpace(cmd.String("event"))),
				}
				for index, existing := range credentials.SilenceRules {
					if strings.EqualFold(existing.Agent, rule.Agent) && strings.EqualFold(existing.Project, rule.Project) && existing.Event == rule.Event {
						_, err = fmt.Fprintf(options.Stdout, "Silence rule %d already exists\n", index+1)
						return err
					}
				}
				credentials.SilenceRules = append(credentials.SilenceRules, rule)
				if err := config.Save(path, credentials); err != nil {
					return err
				}
				_, err = fmt.Fprintf(options.Stdout, "Silence rule %d added: event=%s agent=%s project=%s\n",
					len(credentials.SilenceRules), rule.Event, ruleValue(rule.Agent), ruleValue(rule.Project))
				return err
			},
		}, {
			Name:      "remove",
			Usage:     "remove a silence rule by its listed number",
			ArgsUsage: "INDEX",
			Flags:     []cli.Flag{configFlag()},
			Action: func(_ context.Context, cmd *cli.Command) error {
				if cmd.Args().Len() != 1 {
					return errors.New("silence remove requires one rule number")
				}
				index, err := strconv.Atoi(strings.TrimSpace(cmd.Args().First()))
				if err != nil || index < 1 {
					return errors.New("silence rule number must be a positive integer")
				}
				path, err := configPath(cmd.String("config"))
				if err != nil {
					return err
				}
				credentials, err := config.Load(path)
				if err != nil {
					return err
				}
				if index > len(credentials.SilenceRules) {
					return fmt.Errorf("silence rule %d does not exist", index)
				}
				credentials.SilenceRules = append(credentials.SilenceRules[:index-1], credentials.SilenceRules[index:]...)
				if err := config.Save(path, credentials); err != nil {
					return err
				}
				_, err = fmt.Fprintf(options.Stdout, "Silence rule %d removed\n", index)
				return err
			},
		}, {
			Name:  "clear",
			Usage: "remove every silence rule",
			Flags: []cli.Flag{configFlag()},
			Action: func(_ context.Context, cmd *cli.Command) error {
				path, err := configPath(cmd.String("config"))
				if err != nil {
					return err
				}
				credentials, err := config.Load(path)
				if err != nil {
					return err
				}
				credentials.SilenceRules = nil
				if err := config.Save(path, credentials); err != nil {
					return err
				}
				_, err = fmt.Fprintln(options.Stdout, "All silence rules cleared")
				return err
			},
		}},
	}
}

func ruleValue(value string) string {
	if value == "" {
		return "*"
	}
	return value
}

func snoozeCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:      "snooze",
		Aliases:   []string{"pause"},
		Usage:     "temporarily pause all hook notifications",
		ArgsUsage: "[duration|off]",
		Flags:     []cli.Flag{configFlag()},
		Action: func(_ context.Context, cmd *cli.Command) error {
			now := options.Now()
			path, err := configPath(cmd.String("config"))
			if err != nil {
				return err
			}
			credentials, err := config.Load(path)
			if err != nil {
				return err
			}
			value := strings.ToLower(strings.TrimSpace(cmd.Args().First()))
			if value == "" {
				if credentials.IsSnoozed(now) {
					until, _ := time.Parse(time.RFC3339Nano, credentials.SnoozedUntil)
					_, err = fmt.Fprintf(options.Stdout, "Notifications snoozed until %s\n", formatDeadline(until, now.Location()))
					return err
				}
				_, err = fmt.Fprintln(options.Stdout, "Notifications are active")
				return err
			}
			if cmd.Args().Len() > 1 {
				return errors.New("snooze accepts at most one duration or off")
			}
			if value == "off" || value == "resume" {
				credentials.SnoozedUntil = ""
				if err := config.Save(path, credentials); err != nil {
					return err
				}
				_, err = fmt.Fprintln(options.Stdout, "Notifications resumed")
				return err
			}
			duration, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("parse snooze duration %q (examples: 30m, 2h): %w", value, err)
			}
			if duration <= 0 {
				return errors.New("snooze duration must be greater than zero")
			}
			until := now.Add(duration)
			credentials.SnoozedUntil = until.Format(time.RFC3339Nano)
			if err := config.Save(path, credentials); err != nil {
				return err
			}
			_, err = fmt.Fprintf(options.Stdout, "Notifications snoozed until %s\n", formatDeadline(until, now.Location()))
			return err
		},
	}
}

func formatDeadline(until time.Time, location *time.Location) string {
	return until.In(location).Format("2006-01-02 15:04 MST")
}

func focusCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:      "focus",
		Aliases:   []string{"blockers-only"},
		Usage:     "temporarily suppress completion notifications while keeping blockers",
		ArgsUsage: "[duration|off]",
		Flags:     []cli.Flag{configFlag()},
		Action: func(_ context.Context, cmd *cli.Command) error {
			now := options.Now()
			path, err := configPath(cmd.String("config"))
			if err != nil {
				return err
			}
			credentials, err := config.Load(path)
			if err != nil {
				return err
			}
			value := strings.ToLower(strings.TrimSpace(cmd.Args().First()))
			if value == "" {
				if credentials.IsFocused(now) {
					until, _ := time.Parse(time.RFC3339Nano, credentials.FocusUntil)
					_, err = fmt.Fprintf(options.Stdout, "Focus mode active until %s; blocker notifications remain active\n", formatDeadline(until, now.Location()))
					return err
				}
				_, err = fmt.Fprintln(options.Stdout, "Focus mode is off")
				return err
			}
			if cmd.Args().Len() > 1 {
				return errors.New("focus accepts at most one duration or off")
			}
			if value == "off" || value == "resume" {
				credentials.FocusUntil = ""
				if err := config.Save(path, credentials); err != nil {
					return err
				}
				_, err = fmt.Fprintln(options.Stdout, "Focus mode disabled; completion notifications resumed")
				return err
			}
			duration, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("parse focus duration %q (examples: 30m, 2h): %w", value, err)
			}
			if duration <= 0 {
				return errors.New("focus duration must be greater than zero")
			}
			until := now.Add(duration)
			credentials.FocusUntil = until.Format(time.RFC3339Nano)
			if err := config.Save(path, credentials); err != nil {
				return err
			}
			_, err = fmt.Fprintf(options.Stdout, "Focus mode active until %s; blocker notifications remain active\n", formatDeadline(until, now.Location()))
			return err
		},
	}
}

func quietHoursCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:      "quiet-hours",
		Usage:     "suppress completion notifications during a recurring local-time window",
		ArgsUsage: "[HH:MM HH:MM|off]",
		Flags:     []cli.Flag{configFlag()},
		Action: func(_ context.Context, cmd *cli.Command) error {
			path, err := configPath(cmd.String("config"))
			if err != nil {
				return err
			}
			credentials, err := config.Load(path)
			if err != nil {
				return err
			}
			switch cmd.Args().Len() {
			case 0:
				if credentials.QuietHoursStart == "" {
					_, err = fmt.Fprintln(options.Stdout, "Quiet hours are off")
					return err
				}
				_, err = fmt.Fprintf(options.Stdout,
					"Quiet hours active daily from %s to %s; blocker notifications remain active\n",
					credentials.QuietHoursStart, credentials.QuietHoursEnd,
				)
				return err
			case 1:
				value := strings.ToLower(strings.TrimSpace(cmd.Args().First()))
				if value != "off" && value != "resume" {
					return errors.New("quiet-hours requires both start and end times, or off")
				}
				credentials.QuietHoursStart = ""
				credentials.QuietHoursEnd = ""
				if err := config.Save(path, credentials); err != nil {
					return err
				}
				_, err = fmt.Fprintln(options.Stdout, "Quiet hours disabled; completion notifications resumed")
				return err
			case 2:
				credentials.QuietHoursStart = strings.TrimSpace(cmd.Args().Get(0))
				credentials.QuietHoursEnd = strings.TrimSpace(cmd.Args().Get(1))
				if err := config.Save(path, credentials); err != nil {
					return err
				}
				_, err = fmt.Fprintf(options.Stdout,
					"Quiet hours active daily from %s to %s; blocker notifications remain active\n",
					credentials.QuietHoursStart, credentials.QuietHoursEnd,
				)
				return err
			default:
				return errors.New("quiet-hours accepts start and end times, or off")
			}
		},
	}
}

func deviceCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:      "device",
		Usage:     "show or change the target Pushover device(s)",
		ArgsUsage: "[name[,name...]|all]",
		Flags:     []cli.Flag{configFlag()},
		Action: func(_ context.Context, cmd *cli.Command) error {
			path, err := configPath(cmd.String("config"))
			if err != nil {
				return err
			}
			credentials, err := config.Load(path)
			if err != nil {
				return err
			}
			device := strings.TrimSpace(cmd.Args().First())
			if device == "" {
				device = credentials.Device
				if device == "" {
					device = "all"
				}
				_, err = fmt.Fprintln(options.Stdout, device)
				return err
			}
			if cmd.Args().Len() > 1 {
				return errors.New("device accepts at most one value")
			}
			credentials.Device = normalizeDeviceTarget(device)
			if credentials.Device == "" {
				device = "all"
			}
			if err := config.Save(path, credentials); err != nil {
				return err
			}
			_, err = fmt.Fprintf(options.Stdout, "Target Pushover device(s) set to %s\n", device)
			return err
		},
	}
}

func profileCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:      "profile",
		Usage:     "show or change the notification profile",
		ArgsUsage: "[balanced|quiet|urgent|watch]",
		Flags:     []cli.Flag{configFlag()},
		Action: func(_ context.Context, cmd *cli.Command) error {
			path, err := configPath(cmd.String("config"))
			if err != nil {
				return err
			}
			credentials, err := config.Load(path)
			if err != nil {
				return err
			}
			profile := strings.ToLower(strings.TrimSpace(cmd.Args().First()))
			if profile == "" {
				profile = credentials.NotificationProfile
				if profile == "" {
					profile = "balanced"
				}
				_, err = fmt.Fprintln(options.Stdout, profile)
				return err
			}
			if cmd.Args().Len() > 1 {
				return errors.New("profile accepts at most one value")
			}
			credentials.NotificationProfile = profile
			if profile == "balanced" {
				credentials.NotificationProfile = ""
			}
			if err := config.Save(path, credentials); err != nil {
				return err
			}
			_, err = fmt.Fprintf(options.Stdout, "Notification profile set to %s\n", profile)
			return err
		},
	}
}

func soundCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:      "sound",
		Usage:     "show or change the Pushover sound for each notification event",
		ArgsUsage: "[event [sound|default|reset]]",
		Flags:     []cli.Flag{configFlag()},
		Action: func(_ context.Context, cmd *cli.Command) error {
			path, err := configPath(cmd.String("config"))
			if err != nil {
				return err
			}
			credentials, err := config.Load(path)
			if err != nil {
				return err
			}
			if cmd.Args().Len() > 2 {
				return errors.New("sound accepts an event and at most one sound value")
			}
			if cmd.Args().Len() == 0 {
				for _, event := range []notification.Event{notification.EventTurnComplete, notification.EventApprovalRequired, notification.EventAttentionRequired} {
					if _, err := fmt.Fprintf(options.Stdout, "%s: %s\n", event, effectiveSound(credentials, event)); err != nil {
						return err
					}
				}
				return nil
			}
			event := notification.Event(strings.ToLower(strings.TrimSpace(cmd.Args().Get(0))))
			preference, preset, err := soundPreference(&credentials, event)
			if err != nil {
				return err
			}
			if cmd.Args().Len() == 1 {
				_, err = fmt.Fprintln(options.Stdout, effectiveSound(credentials, event))
				return err
			}
			value := strings.TrimSpace(cmd.Args().Get(1))
			if value == "" {
				return errors.New("sound value cannot be empty")
			}
			if strings.EqualFold(value, "reset") {
				*preference = ""
				if err := config.Save(path, credentials); err != nil {
					return err
				}
				_, err = fmt.Fprintf(options.Stdout, "%s sound reset to %s\n", event, preset)
				return err
			}
			if strings.EqualFold(value, "default") {
				value = "default"
			}
			*preference = value
			if err := config.Save(path, credentials); err != nil {
				return err
			}
			_, err = fmt.Fprintf(options.Stdout, "%s sound set to %s\n", event, value)
			return err
		},
	}
}

func soundPreference(credentials *config.Credentials, event notification.Event) (*string, string, error) {
	switch event {
	case notification.EventTurnComplete:
		return &credentials.TurnCompleteSound, "none", nil
	case notification.EventApprovalRequired:
		return &credentials.ApprovalSound, "persistent", nil
	case notification.EventAttentionRequired:
		return &credentials.AttentionSound, "persistent", nil
	default:
		return nil, "", errors.New("sound event must be turn-complete, approval-required, or attention-required")
	}
}

func effectiveSound(credentials config.Credentials, event notification.Event) string {
	preference, preset, err := soundPreference(&credentials, event)
	if err != nil || *preference == "" {
		return preset
	}
	return *preference
}

func agentsCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:  "agents",
		Usage: "list supported coding agents and notification capabilities",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "detected", Usage: "show only supported agents detected on this machine"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			agents := hooks.Agents()
			if cmd.Bool("detected") {
				var err error
				agents, err = hooks.DetectedAgents()
				if err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(options.Stdout, "AGENT      CAPABILITIES                    INTEGRATION"); err != nil {
				return err
			}
			for _, agent := range agents {
				if _, err := fmt.Fprintf(options.Stdout, "%-10s %-31s %s (%s)\n", agent.Name, agent.Capabilities, agent.DisplayName, agent.Resource); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func previewCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:  "preview",
		Usage: "preview the compact notification generated from a hook payload",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "agent", Usage: "source agent name", Required: true},
			&cli.StringFlag{Name: "event", Usage: "turn-complete, approval-required, or attention-required", Required: true},
			&cli.StringFlag{Name: "profile", Usage: "notification profile: balanced, quiet, urgent, or watch", Value: "balanced"},
			configFlag(),
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			payload, err := readPayload(options.Stdin)
			if err != nil {
				return err
			}
			event := notification.Event(cmd.String("event"))
			message, err := notification.Build(cmd.String("agent"), event, payload)
			if err != nil {
				return err
			}
			profile := strings.ToLower(strings.TrimSpace(cmd.String("profile")))
			var credentials config.Credentials
			configured := false
			previewConfigPath := cmd.String("config")
			explicitConfig := previewConfigPath != ""
			if previewConfigPath == "" {
				previewConfigPath = options.DefaultConfigPath
				if previewConfigPath == "" {
					previewConfigPath, err = config.DefaultPath()
					if err != nil {
						return err
					}
				}
			}
			credentials, err = config.Load(previewConfigPath)
			if err == nil {
				configured = true
				if !cmd.IsSet("profile") {
					profile = credentials.NotificationProfile
				}
			} else if explicitConfig || !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if configured && credentials.IsSilenced(cmd.String("agent"), string(event), notification.ProjectName(payload)) {
				_, err = fmt.Fprintln(options.Stdout, "Delivery: suppressed by a matching silence rule")
				return err
			}
			if !notification.ShouldDeliver(event, profile) {
				_, err = fmt.Fprintf(options.Stdout, "Delivery: suppressed by %s profile\n", profile)
				return err
			}
			message, err = notification.ApplyProfile(message, event, profile)
			if err != nil {
				return err
			}
			if configured {
				credentials.NotificationProfile = profile
				message = applyConfiguredSound(message, event, credentials)
			}
			_, err = fmt.Fprintf(options.Stdout,
				"Title: %s\nBody: %s\nPriority: %d\nSound: %s\nTTL: %s\n",
				message.Title, message.Body, message.Priority, message.Sound, time.Duration(message.TTL)*time.Second,
			)
			if err == nil && message.URL != "" {
				_, err = fmt.Fprintf(options.Stdout, "Action: %s (%s)\n", message.URLTitle, message.URL)
			}
			return err
		},
	}
}

func setupCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:    "setup",
		Aliases: []string{"configure"},
		Usage:   "interactively create the local Pushover config",
		Flags: []cli.Flag{
			configFlag(),
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			prompter := newSecretPrompter(options.Stdin, options.Stdout)
			appToken, err := prompter.readRequired("Pushover application token: ")
			if err != nil {
				return err
			}
			userKey, err := prompter.readRequired("Pushover user/group key: ")
			if err != nil {
				return err
			}
			profile, err := prompter.readChoice(
				"Notification profile [balanced/quiet/urgent/watch] (balanced): ",
				"balanced", "balanced", "quiet", "urgent", "watch",
			)
			if err != nil {
				return err
			}
			if profile == "balanced" {
				profile = ""
			}
			device, err := prompter.readOptional("Target Pushover device(s), comma-separated (all; groups may ignore): ")
			if err != nil {
				return err
			}
			device = normalizeDeviceTarget(device)
			credentials := config.Credentials{
				AppToken:            appToken,
				UserKey:             userKey,
				Device:              device,
				NotificationProfile: profile,
			}
			path, err := configPath(cmd.String("config"))
			if err != nil {
				return err
			}
			if err := config.Save(path, credentials); err != nil {
				return err
			}
			_, err = fmt.Fprintf(options.Stdout, "Saved Pushover credentials to %s\n", path)
			return err
		},
	}
}

func normalizeDeviceTarget(device string) string {
	device = strings.TrimSpace(device)
	if strings.EqualFold(device, "all") {
		return ""
	}
	return device
}

func installCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:  "install",
		Usage: "install notification hooks or an extension for a local agent",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "agent", Usage: "agent to configure; run `vibe-pushover agents` for the list"},
			&cli.BoolFlag{Name: "detected", Usage: "configure every supported agent detected on this machine"},
			&cli.StringFlag{Name: "agent-config", Usage: "override the agent hook or extension path"},
			&cli.StringFlag{Name: "binary", Usage: "override the vibe-pushover executable path", Value: options.Executable},
			configFlag(),
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			agent := strings.ToLower(strings.TrimSpace(cmd.String("agent")))
			detected := cmd.Bool("detected")
			if (agent == "") == !detected {
				return errors.New("install requires exactly one of --agent or --detected")
			}
			if detected && cmd.String("agent-config") != "" {
				return errors.New("--agent-config cannot be used with --detected")
			}
			if agent == "deepseek" {
				agent = "codewhale"
			}
			pushoverConfig := cmd.String("config")
			if pushoverConfig != "" {
				var err error
				pushoverConfig, err = filepath.Abs(pushoverConfig)
				if err != nil {
					return fmt.Errorf("resolve Pushover config path: %w", err)
				}
			}
			if !detected {
				return installAgent(options.Stdout, agent, cmd.String("agent-config"), cmd.String("binary"), pushoverConfig)
			}
			agents, err := hooks.DetectedAgents()
			if err != nil {
				return err
			}
			if len(agents) == 0 {
				_, err = fmt.Fprintln(options.Stdout, "No supported agent installations detected")
				return err
			}
			var installErrors []error
			for _, info := range agents {
				if err := installAgent(options.Stdout, info.Name, "", cmd.String("binary"), pushoverConfig); err != nil {
					installErrors = append(installErrors, fmt.Errorf("install %s: %w", info.Name, err))
				}
			}
			return errors.Join(installErrors...)
		},
	}
}

func installAgent(stdout io.Writer, agent, agentConfig, executable, pushoverConfig string) error {
	paths := []string{agentConfig}
	if paths[0] == "" {
		var err error
		paths, err = hooks.DefaultPaths(agent)
		if err != nil {
			return err
		}
	}
	resource := "integration"
	for _, info := range hooks.Agents() {
		if info.Name == agent {
			resource = info.Resource
			break
		}
	}
	changed, err := hooks.InstallAll(agent, paths, executable, pushoverConfig)
	if err != nil {
		return err
	}
	for index, path := range paths {
		if changed[index] {
			_, err = fmt.Fprintf(stdout, "Installed %s %s in %s\n", agent, resource, path)
		} else {
			_, err = fmt.Fprintf(stdout, "%s %s already installed in %s\n", agent, resource, path)
		}
		if err != nil {
			return err
		}
	}
	if agent == "dotcraft" {
		_, err = fmt.Fprintln(stdout, "DotCraft requires new or changed user hooks to be trusted in Settings > Hooks before they can run.")
		if err != nil {
			return err
		}
	}
	return nil
}

func notifyCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:   "notify",
		Usage:  "read an agent hook payload from stdin and send a notification",
		Hidden: false,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "agent", Usage: "source agent name", Required: true},
			&cli.StringFlag{Name: "event", Usage: "turn-complete, approval-required, or attention-required", Required: true},
			&cli.BoolFlag{Name: "ignore-errors", Usage: "log delivery failures without failing the hook"},
			&cli.BoolFlag{Name: "no-input", Hidden: true},
			&cli.StringFlag{Name: "payload-env", Hidden: true},
			&cli.BoolFlag{Name: "skip-active-stop", Hidden: true},
			&cli.BoolFlag{Name: "skip-non-completion", Hidden: true},
			&cli.BoolFlag{Name: "skip-active-qwen-stop", Hidden: true},
			&cli.BoolFlag{Name: "skip-noninteractive-approval", Hidden: true},
			&cli.BoolFlag{Name: "skip-mistral-subagent", Hidden: true},
			&cli.BoolFlag{Name: "skip-cline-subagent", Hidden: true},
			&cli.BoolFlag{Name: "skip-antigravity-noncompletion", Hidden: true},
			&cli.BoolFlag{Name: "only-antigravity-failure", Hidden: true},
			&cli.BoolFlag{Name: "skip-codewhale-noncompletion", Hidden: true},
			configFlag(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			var payload map[string]any
			var err error
			payloadEnv := strings.TrimSpace(cmd.String("payload-env"))
			if cmd.Bool("no-input") && payloadEnv != "" {
				return errors.New("--no-input and --payload-env cannot be used together")
			}
			if payloadEnv != "" {
				value, ok := os.LookupEnv(payloadEnv)
				if !ok {
					return fmt.Errorf("payload environment variable %s is not set", payloadEnv)
				}
				payload, err = readPayload(strings.NewReader(value))
				if err != nil {
					return fmt.Errorf("read payload from %s: %w", payloadEnv, err)
				}
			} else if cmd.Bool("no-input") {
				payload = map[string]any{}
				if cwd, cwdErr := os.Getwd(); cwdErr == nil {
					payload["cwd"] = cwd
				}
			} else {
				payload, err = readPayload(options.Stdin)
				if err != nil {
					return err
				}
			}
			if cmd.Bool("skip-non-completion") {
				cause, _ := payload["agent_stop_cause"].(string)
				cause = strings.TrimSpace(cause)
				if cause != "end_turn" {
					return nil
				}
			}
			// Kimi release candidates briefly emitted --skip-active-stop as a no-op.
			// Preserve that behavior while using the flag for agents whose Stop
			// contract defines stop_hook_active re-entry semantics.
			if (cmd.Bool("skip-active-stop") && cmd.String("agent") != "kimi") || cmd.Bool("skip-active-qwen-stop") {
				active, _ := payload["stop_hook_active"].(bool)
				if active {
					return nil
				}
			}
			if cmd.Bool("skip-noninteractive-approval") {
				extra, _ := payload["extra"].(map[string]any)
				surface, _ := extra["surface"].(string)
				if surface == "smart" {
					return nil
				}
			}
			if cmd.Bool("skip-mistral-subagent") {
				parentSessionID, _ := payload["parent_session_id"].(string)
				if parentSessionID == "" {
					parentSessionID, _ = payload["parentSessionId"].(string)
				}
				transcriptPath, _ := payload["transcript_path"].(string)
				if strings.TrimSpace(parentSessionID) != "" && isMistralSubagentTranscript(transcriptPath) {
					return nil
				}
			}
			if cmd.Bool("skip-cline-subagent") {
				parentAgentID, _ := payload["parent_agent_id"].(string)
				if parentAgentID == "" {
					parentAgentID, _ = payload["parentAgentId"].(string)
				}
				if strings.TrimSpace(parentAgentID) != "" {
					return nil
				}
			}
			if cmd.Bool("skip-antigravity-noncompletion") {
				fullyIdle, _ := payload["fullyIdle"].(bool)
				reason, _ := payload["terminationReason"].(string)
				if !fullyIdle || strings.TrimSpace(reason) != "model_stop" || antigravityStopFailed(payload) {
					return nil
				}
			}
			if cmd.Bool("only-antigravity-failure") && !antigravityStopFailed(payload) {
				return nil
			}
			if cmd.Bool("skip-codewhale-noncompletion") {
				status, _ := payload["status"].(string)
				errorMessage, _ := payload["error"].(string)
				if strings.TrimSpace(status) != "completed" || strings.TrimSpace(errorMessage) != "" {
					return nil
				}
			}
			event := notification.Event(cmd.String("event"))
			message, err := notification.Build(cmd.String("agent"), event, payload)
			if err == nil {
				path, pathErr := configPath(cmd.String("config"))
				if pathErr != nil {
					err = pathErr
				} else {
					credentials, loadErr := config.Load(path)
					if loadErr != nil {
						err = loadErr
					} else if credentials.IsSilenced(cmd.String("agent"), string(event), notification.ProjectName(payload)) {
						return nil
					} else if credentials.IsSnoozed(options.Now()) {
						return nil
					} else if event == notification.EventTurnComplete && (credentials.IsFocused(options.Now()) || credentials.IsQuietHours(options.Now())) {
						return nil
					} else if !notification.ShouldDeliver(event, credentials.NotificationProfile) {
						return nil
					} else if message, err = notification.ApplyProfile(message, event, credentials.NotificationProfile); err == nil {
						message = applyConfiguredSound(message, event, credentials)
						fingerprint := notificationFingerprint(cmd.String("agent"), event, notificationDestination(credentials), payload, message)
						store := dedupe.Store{Path: options.DedupePath, Now: options.Now}
						reservation, duplicate, dedupeErr := reserveNotification(store, fingerprint)
						if dedupeErr != nil {
							_, _ = fmt.Fprintf(options.Stderr, "vibe-pushover: dedupe unavailable: %v\n", dedupeErr)
						} else if duplicate {
							return nil
						}
						err = sendWithCredentials(ctx, options, credentials, message)
						if reservation.Token != "" {
							if err == nil {
								if commitErr := store.Commit(reservation); commitErr != nil {
									_, _ = fmt.Fprintf(options.Stderr, "vibe-pushover: dedupe commit failed: %v\n", commitErr)
								}
							} else if releaseErr := store.Release(reservation); releaseErr != nil {
								_, _ = fmt.Fprintf(options.Stderr, "vibe-pushover: dedupe release failed: %v\n", releaseErr)
							}
						}
					}
				}
			}
			if err != nil && cmd.Bool("ignore-errors") {
				_, _ = fmt.Fprintf(options.Stderr, "vibe-pushover: %v\n", err)
				return nil
			}
			return err
		},
	}
}

func antigravityStopFailed(payload map[string]any) bool {
	errorMessage, _ := payload["error"].(string)
	if strings.TrimSpace(errorMessage) != "" {
		return true
	}
	reason, _ := payload["terminationReason"].(string)
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "error", "max_steps_exceeded":
		return true
	default:
		return false
	}
}

func isMistralSubagentTranscript(path string) bool {
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' })
	return len(parts) >= 3 && parts[len(parts)-3] == "agents" && parts[len(parts)-1] == "messages.jsonl"
}

func testCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:  "test",
		Usage: "send a real notification using the configured delivery experience",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "event", Usage: "turn-complete, approval-required, or attention-required", Value: "approval-required"},
			&cli.StringFlag{Name: "message", Usage: "test message body", Value: "Test notification delivered successfully."},
			&cli.BoolFlag{Name: "force", Usage: "send even when snooze, focus, quiet hours, or silence rules suppress delivery"},
			configFlag(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			path, err := configPath(cmd.String("config"))
			if err != nil {
				return err
			}
			credentials, err := config.Load(path)
			if err != nil {
				return err
			}
			event := notification.Event(strings.ToLower(strings.TrimSpace(cmd.String("event"))))
			now := options.Now()
			cwd, _ := os.Getwd()
			payload := map[string]any{"cwd": cwd, "message": cmd.String("message")}
			if credentials.IsSilenced("vibe-pushover", string(event), notification.ProjectName(payload)) && !cmd.Bool("force") {
				_, err = fmt.Fprintf(options.Stdout,
					"Test %s notification suppressed by a matching silence rule; use --force to send\n", event,
				)
				return err
			}
			if credentials.IsSnoozed(now) && !cmd.Bool("force") {
				until, _ := time.Parse(time.RFC3339Nano, credentials.SnoozedUntil)
				_, err = fmt.Fprintf(options.Stdout,
					"Test %s notification suppressed while notifications are snoozed until %s; use --force to send\n",
					event, formatDeadline(until, now.Location()),
				)
				return err
			}
			if event == notification.EventTurnComplete && credentials.IsFocused(now) && !cmd.Bool("force") {
				until, _ := time.Parse(time.RFC3339Nano, credentials.FocusUntil)
				_, err = fmt.Fprintf(options.Stdout,
					"Test %s notification suppressed while focus mode is active until %s; use --force to send\n",
					event, formatDeadline(until, now.Location()),
				)
				return err
			}
			if event == notification.EventTurnComplete && credentials.IsQuietHours(now) && !cmd.Bool("force") {
				_, err = fmt.Fprintf(options.Stdout,
					"Test %s notification suppressed during quiet hours %s-%s; use --force to send\n",
					event, credentials.QuietHoursStart, credentials.QuietHoursEnd,
				)
				return err
			}
			message, err := notification.Build("vibe-pushover", event, payload)
			if err != nil {
				return err
			}
			if !notification.ShouldDeliver(event, credentials.NotificationProfile) {
				_, err = fmt.Fprintf(options.Stdout, "Test %s notification suppressed by %s profile\n", event, credentials.NotificationProfile)
				return err
			}
			message, err = notification.ApplyProfile(message, event, credentials.NotificationProfile)
			if err != nil {
				return err
			}
			message = applyConfiguredSound(message, event, credentials)
			if err := sendWithCredentials(ctx, options, credentials, message); err != nil {
				return err
			}
			_, err = fmt.Fprintf(options.Stdout, "Test %s notification sent\n", event)
			return err
		},
	}
}

func applyConfiguredSound(message notification.Message, event notification.Event, credentials config.Credentials) notification.Message {
	if credentials.NotificationProfile == "quiet" {
		return message
	}
	preference, _, err := soundPreference(&credentials, event)
	if err != nil || *preference == "" {
		return message
	}
	if *preference == "default" {
		message.Sound = ""
	} else {
		message.Sound = *preference
	}
	return message
}

func sendWithCredentials(ctx context.Context, options Options, credentials config.Credentials, message notification.Message) error {
	client := pushover.NewClient(options.HTTPClient, options.Endpoint)
	return client.Send(ctx, pushover.Message{
		AppToken: credentials.AppToken,
		UserKey:  credentials.UserKey,
		Device:   credentials.Device,
		Title:    message.Title,
		Body:     message.Body,
		URL:      message.URL,
		URLTitle: message.URLTitle,
		Priority: message.Priority,
		Sound:    message.Sound,
		TTL:      message.TTL,
	})
}

func reserveNotification(store dedupe.Store, fingerprint string) (dedupe.Reservation, bool, error) {
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		result, err := store.Reserve(fingerprint)
		if err != nil {
			return dedupe.Reservation{}, false, err
		}
		switch result.Status {
		case dedupe.StatusAcquired:
			return result.Reservation, false, nil
		case dedupe.StatusDelivered:
			return dedupe.Reservation{}, true, nil
		case dedupe.StatusPending:
			if !time.Now().Before(deadline) {
				return dedupe.Reservation{}, false, errors.New("pending reservation did not settle within 500ms")
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
}

func notificationDestination(credentials config.Credentials) string {
	data := credentials.AppToken + "\x00" + credentials.UserKey + "\x00" + credentials.Device
	return fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
}

func notificationFingerprint(agent string, event notification.Event, destination string, payload map[string]any, message notification.Message) string {
	identity := map[string]any{
		"agent": agent, "event": event, "destination": destination,
		"title": message.Title, "body": message.Body, "url": message.URL, "url_title": message.URLTitle,
		"priority": message.Priority, "sound": message.Sound, "ttl": message.TTL,
	}
	for _, key := range []string{"session_id", "sessionId", "turn_id", "turnId", "tool_call_id", "toolCallId", "approval_id", "approvalId"} {
		if value := fingerprintScalar(payload[key]); value != "" {
			identity[key] = value
		}
	}
	if extra, ok := payload["extra"].(map[string]any); ok {
		for _, key := range []string{"turn_id", "turnId", "tool_call_id", "toolCallId", "approval_id", "approvalId"} {
			if value := fingerprintScalar(extra[key]); value != "" {
				identity["extra."+key] = value
			}
		}
	}
	data, _ := json.Marshal(identity)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func fingerprintScalar(value any) string {
	switch value.(type) {
	case string, json.Number, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return strings.TrimSpace(fmt.Sprint(value))
	default:
		return ""
	}
}

func readPayload(reader io.Reader) (map[string]any, error) {
	decoder := json.NewDecoder(io.LimitReader(reader, 1<<20))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); errors.Is(err, io.EOF) {
		payload = map[string]any{}
	} else if err != nil {
		return nil, fmt.Errorf("parse hook payload: %w", err)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if !hasUsableWorkspace(payload) {
		if cwd, err := os.Getwd(); err == nil {
			payload["cwd"] = cwd
		}
	}
	return payload, nil
}

func hasUsableWorkspace(payload map[string]any) bool {
	if cwd, ok := payload["cwd"].(string); ok && strings.TrimSpace(cwd) != "" {
		return true
	}
	for _, key := range []string{"working_dir", "workingDir"} {
		if workingDir, ok := payload[key].(string); ok && strings.TrimSpace(workingDir) != "" {
			return true
		}
	}
	for _, key := range []string{"workspace_roots", "workspaceRoots", "workspacePaths"} {
		switch roots := payload[key].(type) {
		case []any:
			for _, root := range roots {
				if value, ok := root.(string); ok && strings.TrimSpace(value) != "" {
					return true
				}
			}
		case []string:
			for _, root := range roots {
				if strings.TrimSpace(root) != "" {
					return true
				}
			}
		}
	}
	return false
}

func configFlag() cli.Flag {
	return &cli.StringFlag{Name: "config", Usage: "override the vibe-pushover config path"}
}

func configPath(path string) (string, error) {
	if path != "" {
		return path, nil
	}
	return config.DefaultPath()
}

func withDefaults(options Options) Options {
	customDelivery := options.HTTPClient != nil || options.Endpoint != ""
	if options.Version == "" {
		options.Version = "dev"
	}
	if options.Stdin == nil {
		options.Stdin = os.Stdin
	}
	if options.Stdout == nil {
		options.Stdout = os.Stdout
	}
	if options.Stderr == nil {
		options.Stderr = os.Stderr
	}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if options.Endpoint == "" {
		options.Endpoint = pushover.DefaultEndpoint
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.DedupePath == "" && !customDelivery {
		if path, err := dedupe.DefaultPath(); err == nil {
			options.DedupePath = path
		}
	}
	if options.Executable == "" {
		if executable, err := os.Executable(); err == nil {
			options.Executable = executable
		}
	}
	return options
}
