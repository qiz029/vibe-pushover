package command

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	RunProcess        func(context.Context, []string, io.Reader, io.Writer, io.Writer) error
	Random            io.Reader
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
		ExitErrHandler:        func(context.Context, *cli.Command, error) {},
		Commands: []*cli.Command{
			setupCommand(options),
			statusCommand(options),
			profileCommand(options),
			detailCommand(options),
			encryptionCommand(options),
			snoozeCommand(options),
			focusCommand(options),
			quietHoursCommand(options),
			silenceCommand(options),
			deviceCommand(options),
			soundCommand(options),
			agentsCommand(options),
			installCommand(options),
			runCommand(options),
			previewCommand(options),
			notifyCommand(options),
			testCommand(options),
		},
	}
}

func runCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:      "run",
		Usage:     "run any coding-agent CLI and notify when its session exits",
		ArgsUsage: "-- command [args...]",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "agent", Usage: "agent name shown in the notification", Required: true},
			&cli.DurationFlag{Name: "after", Usage: "notify successful sessions only after this duration", Value: 30 * time.Second},
			configFlag(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			argv := cmd.Args().Slice()
			if len(argv) == 0 {
				return errors.New("run requires a command after --")
			}
			after := cmd.Duration("after")
			if after < 0 {
				return errors.New("--after cannot be negative")
			}

			started := options.Now()
			runErr := options.RunProcess(ctx, argv, options.Stdin, options.Stdout, options.Stderr)
			finished := options.Now()
			elapsed := finished.Sub(started)
			if elapsed < 0 {
				elapsed = 0
			}
			if runErr == nil && elapsed < after {
				return nil
			}

			event := notification.EventTurnComplete
			message := fmt.Sprintf("Completed · %s", compactDuration(elapsed))
			if runErr != nil {
				event = notification.EventAttentionRequired
				message = fmt.Sprintf("Failed · exit %d · %s", processExitCode(runErr), compactDuration(elapsed))
			}
			cwd, _ := os.Getwd()
			payload := map[string]any{
				"cwd":       cwd,
				"message":   message,
				"timestamp": finished.Unix(),
			}
			if err := deliverNotification(ctx, options, cmd.String("config"), strings.TrimSpace(cmd.String("agent")), event, payload, finished); err != nil {
				_, _ = fmt.Fprintf(options.Stderr, "vibe-pushover: notification failed: %v\n", err)
			}
			return wrapProcessRunError(runErr)
		},
	}
}

type processRunError struct {
	err    error
	code   int
	silent bool
}

func (e processRunError) Error() string { return e.err.Error() }
func (e processRunError) Unwrap() error { return e.err }
func (e processRunError) ExitCode() int { return e.code }
func (e processRunError) Silent() bool  { return e.silent }

func wrapProcessRunError(err error) error {
	if err == nil {
		return nil
	}
	var exitCoder interface{ ExitCode() int }
	var exitError *exec.ExitError
	return processRunError{
		err:    err,
		code:   processExitCode(err),
		silent: (errors.As(err, &exitCoder) && exitCoder.ExitCode() >= 0) || errors.As(err, &exitError),
	}
}

func compactDuration(duration time.Duration) string {
	if duration < time.Second {
		return "<1s"
	}
	return duration.Round(time.Second).String()
}

func processExitCode(err error) int {
	if code, ok := platformSignalExitCode(err); ok {
		return code
	}
	var exitCoder interface{ ExitCode() int }
	if errors.As(err, &exitCoder) && exitCoder.ExitCode() >= 0 {
		return exitCoder.ExitCode()
	}
	var startError *exec.Error
	if errors.As(err, &startError) {
		if errors.Is(startError.Err, exec.ErrNotFound) {
			return 127
		}
		return 126
	}
	if errors.Is(err, os.ErrPermission) {
		return 126
	}
	return 1
}

// ErrorExitCode returns the process status that the CLI should use for err.
func ErrorExitCode(err error) int {
	if err == nil {
		return 0
	}
	return normalizeCLIExitCode(processExitCode(err))
}

// ShouldPrintError reports whether main should render err after command execution.
func ShouldPrintError(err error) bool {
	if err == nil {
		return false
	}
	var silent interface{ Silent() bool }
	return !errors.As(err, &silent) || !silent.Silent()
}

func statusCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "show the current notification delivery controls",
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
			now := options.Now()
			profile := credentials.NotificationProfile
			if profile == "" {
				profile = "balanced"
			}
			detail := credentials.NotificationDetail
			if detail == "" {
				detail = "summary"
			}
			device := credentials.Device
			if device == "" {
				device = "all"
			}
			encryption := "off"
			if credentials.EncryptionKey != "" {
				encryption = "on"
			}
			if _, err := fmt.Fprintf(options.Stdout, "Profile: %s\nDetail: %s\nDevice target: %s\nEnd-to-end encryption: %s\n", profile, detail, device, encryption); err != nil {
				return err
			}
			if credentials.IsSnoozed(now) {
				until, _ := time.Parse(time.RFC3339Nano, credentials.SnoozedUntil)
				if _, err := fmt.Fprintf(options.Stdout, "Snooze: active until %s\n", formatDeadline(until, now.Location())); err != nil {
					return err
				}
			} else if _, err := fmt.Fprintln(options.Stdout, "Snooze: off"); err != nil {
				return err
			}
			if credentials.IsFocused(now) {
				until, _ := time.Parse(time.RFC3339Nano, credentials.FocusUntil)
				if _, err := fmt.Fprintf(options.Stdout, "Focus: active until %s\n", formatDeadline(until, now.Location())); err != nil {
					return err
				}
			} else if _, err := fmt.Fprintln(options.Stdout, "Focus: off"); err != nil {
				return err
			}
			if credentials.QuietHoursStart == "" {
				if _, err := fmt.Fprintln(options.Stdout, "Quiet hours: off"); err != nil {
					return err
				}
			} else {
				state := "inactive now"
				if credentials.IsQuietHours(now) {
					state = "active now"
				}
				if _, err := fmt.Fprintf(options.Stdout, "Quiet hours: %s-%s (%s)\n", credentials.QuietHoursStart, credentials.QuietHoursEnd, state); err != nil {
					return err
				}
			}
			_, err = fmt.Fprintf(options.Stdout,
				"Silence rules: %d\nSounds: turn-complete=%s approval-required=%s attention-required=%s\n",
				len(credentials.SilenceRules),
				statusSound(credentials, notification.EventTurnComplete),
				statusSound(credentials, notification.EventApprovalRequired),
				statusSound(credentials, notification.EventAttentionRequired),
			)
			if err != nil {
				return err
			}
			if retry, expire, ok := emergencySchedule(profile); ok {
				_, err = fmt.Fprintf(options.Stdout,
					"Emergency retries: every %s for up to %s or until acknowledged\n",
					retry, expire,
				)
			}
			return err
		},
	}
}

func emergencySchedule(profile string) (time.Duration, time.Duration, bool) {
	message, err := notification.Build("vibe-pushover", notification.EventApprovalRequired, nil)
	if err != nil {
		return 0, 0, false
	}
	message, err = notification.ApplyProfile(message, notification.EventApprovalRequired, profile)
	if err != nil || message.Priority != 2 {
		return 0, 0, false
	}
	return time.Duration(message.Retry) * time.Second, time.Duration(message.Expire) * time.Second, true
}

func statusSound(credentials config.Credentials, event notification.Event) string {
	message, err := notification.Build("vibe-pushover", event, nil)
	if err != nil {
		return effectiveSound(credentials, event)
	}
	profile := credentials.NotificationProfile
	if profile == "" {
		profile = "balanced"
	}
	if !notification.ShouldDeliver(event, profile) {
		return "suppressed"
	}
	message, err = notification.ApplyProfile(message, event, profile)
	if err != nil {
		return effectiveSound(credentials, event)
	}
	credentials.NotificationProfile = profile
	message = applyConfiguredSound(message, event, credentials)
	if message.Sound == "" {
		return "default"
	}
	return message.Sound
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
		ArgsUsage: "[balanced|quiet|urgent|watch|on-call]",
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
			if err == nil {
				if retry, expire, ok := emergencySchedule(profile); ok {
					_, err = fmt.Fprintf(options.Stdout,
						"Approval and attention notifications will repeat every %s for up to %s or until acknowledged.\n",
						retry, expire,
					)
				}
			}
			return err
		},
	}
}

func detailCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:      "detail",
		Usage:     "show or change how much hook detail notifications include",
		ArgsUsage: "[summary|minimal|private]",
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
			detail := strings.ToLower(strings.TrimSpace(cmd.Args().First()))
			if detail == "" {
				detail = credentials.NotificationDetail
				if detail == "" {
					detail = "summary"
				}
				_, err = fmt.Fprintln(options.Stdout, detail)
				return err
			}
			if cmd.Args().Len() > 1 {
				return errors.New("detail accepts at most one value")
			}
			credentials.NotificationDetail = detail
			if detail == "summary" {
				credentials.NotificationDetail = ""
			}
			if err := config.Save(path, credentials); err != nil {
				return err
			}
			_, err = fmt.Fprintf(options.Stdout, "Notification detail set to %s\n", detail)
			return err
		},
	}
}

func encryptionCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:      "encryption",
		Aliases:   []string{"e2ee"},
		Usage:     "manage optional Pushover end-to-end encryption",
		ArgsUsage: "[enable|set|rotate|disable]",
		Flags:     []cli.Flag{configFlag()},
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() > 1 {
				return errors.New("encryption accepts enable, set, rotate, or disable")
			}
			path, err := configPath(cmd.String("config"))
			if err != nil {
				return err
			}
			credentials, err := config.Load(path)
			if err != nil {
				return err
			}
			action := strings.ToLower(strings.TrimSpace(cmd.Args().First()))
			switch action {
			case "":
				state := "off"
				if credentials.EncryptionKey != "" {
					state = "on"
				}
				_, err = fmt.Fprintf(options.Stdout, "End-to-end encryption: %s\n", state)
				return err
			case "disable", "off":
				credentials.EncryptionKey = ""
				if err := config.Save(path, credentials); err != nil {
					return err
				}
				_, err = fmt.Fprintln(options.Stdout, "End-to-end encryption disabled; future notifications use HTTPS transport encryption only")
				return err
			case "set":
				prompter := newSecretPrompter(options.Stdin, options.Stdout)
				key, err := prompter.readRequired("64-character Pushover encryption key: ")
				if err != nil {
					return err
				}
				credentials.EncryptionKey = strings.ToLower(key)
				if err := config.Save(path, credentials); err != nil {
					return err
				}
				_, err = fmt.Fprintln(options.Stdout, "End-to-end encryption enabled with the supplied key; configure the same key on every target iOS/Android device")
				return err
			case "enable":
				if credentials.EncryptionKey != "" {
					_, err = fmt.Fprintln(options.Stdout, "End-to-end encryption is already enabled; use rotate to generate a new key")
					return err
				}
			case "rotate":
			default:
				return errors.New("encryption accepts enable, set, rotate, or disable")
			}

			key := make([]byte, 32)
			if _, err := io.ReadFull(options.Random, key); err != nil {
				return fmt.Errorf("generate Pushover encryption key: %w", err)
			}
			previousCredentials := credentials
			credentials.EncryptionKey = hex.EncodeToString(key)
			if err := config.Save(path, credentials); err != nil {
				return err
			}
			_, err = fmt.Fprintf(options.Stdout,
				"End-to-end encryption enabled.\nEncryption key (shown once): %s\nConfigure this key in Pushover v5 on every target iOS/Android device, then run `vibe-pushover test`.\n",
				credentials.EncryptionKey,
			)
			if err == nil {
				return nil
			}
			writeErr := fmt.Errorf("display Pushover encryption key: %w", err)
			if rollbackErr := config.Save(path, previousCredentials); rollbackErr != nil {
				return errors.Join(writeErr, fmt.Errorf("roll back Pushover encryption key: %w", rollbackErr))
			}
			return writeErr
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
			&cli.BoolFlag{Name: "detected", Usage: "show agents with local config or a curated CLI executable on PATH"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			agents := hooks.Agents()
			if cmd.Bool("detected") {
				var err error
				agents, err = hooks.DetectedAgents()
				if err != nil {
					return err
				}
				agents = append(agents, hooks.DetectedRunAgents()...)
			} else {
				agents = append(agents, hooks.RunAgents()...)
			}
			sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })
			if _, err := fmt.Fprintf(options.Stdout, "%-15s %-40s %s\n", "AGENT", "CAPABILITIES", "INTEGRATION"); err != nil {
				return err
			}
			for _, agent := range agents {
				if _, err := fmt.Fprintf(options.Stdout, "%-15s %-40s %s (%s)\n", agent.Name, agent.Capabilities, agent.DisplayName, agent.Resource); err != nil {
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
			&cli.StringFlag{Name: "profile", Usage: "notification profile: balanced, quiet, urgent, watch, or on-call", Value: "balanced"},
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
			now := options.Now()
			if configured && credentials.IsSilenced(cmd.String("agent"), string(event), notification.ProjectName(payload)) {
				_, err = fmt.Fprintln(options.Stdout, "Delivery: suppressed by a matching silence rule")
				return err
			}
			if configured && credentials.IsSnoozed(now) {
				until, _ := time.Parse(time.RFC3339Nano, credentials.SnoozedUntil)
				_, err = fmt.Fprintf(options.Stdout, "Delivery: snoozed until %s\n", formatDeadline(until, now.Location()))
				return err
			}
			if configured && event == notification.EventTurnComplete && credentials.IsFocused(now) {
				until, _ := time.Parse(time.RFC3339Nano, credentials.FocusUntil)
				_, err = fmt.Fprintf(options.Stdout, "Delivery: completion suppressed by focus mode until %s\n", formatDeadline(until, now.Location()))
				return err
			}
			if configured && event == notification.EventTurnComplete && credentials.IsQuietHours(now) {
				_, err = fmt.Fprintf(options.Stdout, "Delivery: completion suppressed by quiet hours (%s-%s)\n", credentials.QuietHoursStart, credentials.QuietHoursEnd)
				return err
			}
			if !notification.ShouldDeliver(event, profile) {
				_, err = fmt.Fprintf(options.Stdout, "Delivery: suppressed by %s profile\n", profile)
				return err
			}
			credentials.NotificationProfile = profile
			message, err = applyNotificationPreferences(message, event, credentials)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(options.Stdout,
				"Title: %s\nBody: %s\nPriority: %d\nSound: %s\nTTL: %s\n",
				message.Title, message.Body, message.Priority, message.Sound, time.Duration(message.TTL)*time.Second,
			)
			if err == nil && message.Priority == 2 {
				_, err = fmt.Fprintf(options.Stdout, "Retry: %s\nExpire: %s\n",
					time.Duration(message.Retry)*time.Second, time.Duration(message.Expire)*time.Second)
			}
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
				"Notification profile [balanced/quiet/urgent/watch/on-call] (balanced): ",
				"balanced", "balanced", "quiet", "urgent", "watch", "on-call",
			)
			if err != nil {
				return err
			}
			if profile == "balanced" {
				profile = ""
			}
			detail, err := prompter.readChoice(
				"Notification detail [summary/minimal/private] (summary): ",
				"summary", "summary", "minimal", "private",
			)
			if err != nil {
				return err
			}
			if detail == "summary" {
				detail = ""
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
				NotificationDetail:  detail,
			}
			path, err := configPath(cmd.String("config"))
			if err != nil {
				return err
			}
			if err := config.Save(path, credentials); err != nil {
				return err
			}
			_, err = fmt.Fprintf(options.Stdout, "Saved Pushover credentials to %s\n", path)
			if err == nil {
				if retry, expire, ok := emergencySchedule(profile); ok {
					_, err = fmt.Fprintf(options.Stdout,
						"Approval and attention notifications will repeat every %s for up to %s or until acknowledged.\n",
						retry, expire,
					)
				}
			}
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
			&cli.BoolFlag{Name: "detected", Usage: "configure every agent with local config or a curated CLI executable on PATH"},
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
			} else if agent == "ccr" {
				agent = "claude-router"
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
			runAgents := hooks.DetectedRunAgents()
			if len(agents) == 0 {
				if len(runAgents) > 0 {
					return printRunWrapperDetection(options.Stdout, runAgents)
				}
				_, err = fmt.Fprintln(options.Stdout, "No supported agent installations detected")
				return err
			}
			var installErrors []error
			for _, info := range agents {
				if err := installAgent(options.Stdout, info.Name, "", cmd.String("binary"), pushoverConfig); err != nil {
					installErrors = append(installErrors, fmt.Errorf("install %s: %w", info.Name, err))
				}
			}
			if len(runAgents) > 0 {
				if err := printRunWrapperDetection(options.Stdout, runAgents); err != nil {
					installErrors = append(installErrors, err)
				}
			}
			return errors.Join(installErrors...)
		},
	}
}

func printRunWrapperDetection(stdout io.Writer, agents []hooks.AgentInfo) error {
	names := make([]string, 0, len(agents))
	for _, agent := range agents {
		names = append(names, agent.Name)
	}
	_, err := fmt.Fprintf(stdout, "Run-wrapper agents need no installation: %s\n", strings.Join(names, ", "))
	return err
}

func installAgent(stdout io.Writer, agent, agentConfig, executable, pushoverConfig string) error {
	if hooks.IsRunAgent(agent) {
		invocations, ok := hooks.RunAgentInvocations(agent)
		if !ok {
			return fmt.Errorf("%s run wrapper has no canonical command", agent)
		}
		examples := make([]string, 0, len(invocations))
		for _, invocation := range invocations {
			examples = append(examples, fmt.Sprintf("vibe-pushover run --agent %s -- %s", agent, invocation))
		}
		return fmt.Errorf("%s uses the run wrapper instead of an installed hook; use: %s", agent, strings.Join(examples, " or "))
	}
	paths := []string{agentConfig}
	if paths[0] == "" {
		var err error
		paths, err = hooks.DefaultPaths(agent)
		if err != nil {
			return err
		}
	}
	if len(paths) == 0 {
		if agent == "craft" {
			return errors.New("no Craft Agents workspaces found; create a workspace or pass --agent-config")
		}
		return fmt.Errorf("no configuration paths found for %s; pass --agent-config", agent)
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
			err = deliverNotification(ctx, options, cmd.String("config"), cmd.String("agent"), event, payload, options.Now())
			if err != nil && cmd.Bool("ignore-errors") {
				_, _ = fmt.Fprintf(options.Stderr, "vibe-pushover: %v\n", err)
				return nil
			}
			return err
		},
	}
}

func deliverNotification(
	ctx context.Context,
	options Options,
	configOverride string,
	agent string,
	event notification.Event,
	payload map[string]any,
	now time.Time,
) error {
	message, err := notification.Build(agent, event, payload)
	if err != nil {
		return err
	}
	path, err := configPath(configOverride)
	if err != nil {
		return err
	}
	credentials, err := config.Load(path)
	if err != nil {
		return err
	}
	if credentials.IsSilenced(agent, string(event), notification.ProjectName(payload)) || credentials.IsSnoozed(now) {
		return nil
	}
	if event == notification.EventTurnComplete && (credentials.IsFocused(now) || credentials.IsQuietHours(now)) {
		return nil
	}
	if !notification.ShouldDeliver(event, credentials.NotificationProfile) {
		return nil
	}
	message, err = applyNotificationPreferences(message, event, credentials)
	if err != nil {
		return err
	}

	fingerprint := notificationFingerprint(agent, event, notificationDestination(credentials), payload, message)
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
	return err
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
			message, err := notification.Build("vibe-pushover", event, payload)
			if err != nil {
				return err
			}
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
			if !notification.ShouldDeliver(event, credentials.NotificationProfile) {
				_, err = fmt.Fprintf(options.Stdout, "Test %s notification suppressed by %s profile\n", event, credentials.NotificationProfile)
				return err
			}
			message, err = applyNotificationPreferences(message, event, credentials)
			if err != nil {
				return err
			}
			if err := sendWithCredentials(ctx, options, credentials, message); err != nil {
				return err
			}
			_, err = fmt.Fprintf(options.Stdout, "Test %s notification sent\n", event)
			if err == nil && message.Priority == 2 {
				_, err = fmt.Fprintf(options.Stdout,
					"Emergency notification repeats every %s for up to %s or until acknowledged.\n",
					time.Duration(message.Retry)*time.Second,
					time.Duration(message.Expire)*time.Second,
				)
			}
			return err
		},
	}
}

func applyNotificationPreferences(message notification.Message, event notification.Event, credentials config.Credentials) (notification.Message, error) {
	message, err := notification.ApplyDetail(message, event, credentials.NotificationDetail)
	if err != nil {
		return notification.Message{}, err
	}
	message, err = notification.ApplyProfile(message, event, credentials.NotificationProfile)
	if err != nil {
		return notification.Message{}, err
	}
	return applyConfiguredSound(message, event, credentials), nil
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
		AppToken:      credentials.AppToken,
		UserKey:       credentials.UserKey,
		EncryptionKey: credentials.EncryptionKey,
		Device:        credentials.Device,
		Title:         message.Title,
		Body:          message.Body,
		URL:           message.URL,
		URLTitle:      message.URLTitle,
		Timestamp:     message.Timestamp,
		Priority:      message.Priority,
		Sound:         message.Sound,
		TTL:           message.TTL,
		Retry:         message.Retry,
		Expire:        message.Expire,
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
	data := credentials.AppToken + "\x00" + credentials.UserKey + "\x00" + credentials.Device + "\x00" + credentials.EncryptionKey
	return fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
}

func notificationFingerprint(agent string, event notification.Event, destination string, payload map[string]any, message notification.Message) string {
	identity := map[string]any{
		"agent": agent, "event": event, "destination": destination,
		"title": message.Title, "body": message.Body, "url": message.URL, "url_title": message.URLTitle,
		"timestamp": message.Timestamp, "priority": message.Priority, "sound": message.Sound, "ttl": message.TTL,
		"retry": message.Retry, "expire": message.Expire,
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
	if options.Random == nil {
		options.Random = cryptorand.Reader
	}
	if options.RunProcess == nil {
		options.RunProcess = func(ctx context.Context, argv []string, stdin io.Reader, stdout, stderr io.Writer) error {
			process := exec.CommandContext(ctx, argv[0], argv[1:]...)
			process.Stdin = stdin
			process.Stdout = stdout
			process.Stderr = stderr
			return process.Run()
		}
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
