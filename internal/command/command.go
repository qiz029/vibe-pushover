package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiz029/vibe-pushover/internal/config"
	"github.com/qiz029/vibe-pushover/internal/hooks"
	"github.com/qiz029/vibe-pushover/internal/notification"
	"github.com/qiz029/vibe-pushover/internal/pushover"
	"github.com/urfave/cli/v3"
)

type Options struct {
	Version    string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	HTTPClient *http.Client
	Endpoint   string
	Executable string
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
			agentsCommand(options),
			installCommand(options),
			previewCommand(options),
			notifyCommand(options),
			testCommand(options),
		},
	}
}

func profileCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:      "profile",
		Usage:     "show or change the notification profile",
		ArgsUsage: "[balanced|quiet|watch]",
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

func agentsCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:  "agents",
		Usage: "list supported coding agents and notification capabilities",
		Action: func(_ context.Context, _ *cli.Command) error {
			if _, err := fmt.Fprintln(options.Stdout, "AGENT      CAPABILITIES                    INTEGRATION"); err != nil {
				return err
			}
			for _, agent := range hooks.Agents() {
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
			&cli.StringFlag{Name: "profile", Usage: "notification profile: balanced, quiet, or watch", Value: "balanced"},
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
			message, err = notification.ApplyProfile(message, event, strings.ToLower(strings.TrimSpace(cmd.String("profile"))))
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(options.Stdout,
				"Title: %s\nBody: %s\nPriority: %d\nSound: %s\nTTL: %s\n",
				message.Title, message.Body, message.Priority, message.Sound, time.Duration(message.TTL)*time.Second,
			)
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
				"Notification profile [balanced/quiet/watch] (balanced): ",
				"balanced", "balanced", "quiet", "watch",
			)
			if err != nil {
				return err
			}
			if profile == "balanced" {
				profile = ""
			}
			credentials := config.Credentials{
				AppToken:            appToken,
				UserKey:             userKey,
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

func installCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:  "install",
		Usage: "install notification hooks or an extension for a local agent",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "agent", Usage: "agent to configure; run `vibe-pushover agents` for the list", Required: true},
			&cli.StringFlag{Name: "agent-config", Usage: "override the agent hook or extension path"},
			&cli.StringFlag{Name: "binary", Usage: "override the vibe-pushover executable path", Value: options.Executable},
			configFlag(),
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			agent := strings.ToLower(strings.TrimSpace(cmd.String("agent")))
			pushoverConfig := cmd.String("config")
			if pushoverConfig != "" {
				var err error
				pushoverConfig, err = filepath.Abs(pushoverConfig)
				if err != nil {
					return fmt.Errorf("resolve Pushover config path: %w", err)
				}
			}
			path := cmd.String("agent-config")
			if path == "" {
				var err error
				path, err = hooks.DefaultPath(agent)
				if err != nil {
					return err
				}
			}
			changed, err := hooks.Install(agent, path, cmd.String("binary"), pushoverConfig)
			if err != nil {
				return err
			}
			resource := "integration"
			for _, info := range hooks.Agents() {
				if info.Name == agent {
					resource = info.Resource
					break
				}
			}
			if changed {
				_, err = fmt.Fprintf(options.Stdout, "Installed %s %s in %s\n", agent, resource, path)
			} else {
				_, err = fmt.Fprintf(options.Stdout, "%s %s already installed in %s\n", agent, resource, path)
			}
			return err
		},
	}
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
			// Kept as a no-op so hooks installed by the pre-Kimi release candidate keep working.
			&cli.BoolFlag{Name: "skip-active-stop", Hidden: true},
			&cli.BoolFlag{Name: "skip-non-completion", Hidden: true},
			&cli.BoolFlag{Name: "skip-active-qwen-stop", Hidden: true},
			configFlag(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			payload, err := readPayload(options.Stdin)
			if err != nil {
				return err
			}
			if cmd.Bool("skip-non-completion") {
				cause, _ := payload["agent_stop_cause"].(string)
				cause = strings.TrimSpace(cause)
				if cause != "end_turn" {
					return nil
				}
			}
			if cmd.Bool("skip-active-qwen-stop") {
				active, _ := payload["stop_hook_active"].(bool)
				if active {
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
					} else if message, err = notification.ApplyProfile(message, event, credentials.NotificationProfile); err == nil {
						err = sendWithCredentials(ctx, options, credentials, message)
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

func testCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:  "test",
		Usage: "send a test notification",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "message", Usage: "test message body", Value: "vibe-pushover is configured correctly"},
			configFlag(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			message := notification.Message{
				Title: "vibe-pushover test",
				Body:  cmd.String("message"),
			}
			if err := send(ctx, options, cmd.String("config"), message); err != nil {
				return err
			}
			_, err := fmt.Fprintln(options.Stdout, "Test notification sent")
			return err
		},
	}
}

func send(ctx context.Context, options Options, path string, message notification.Message) error {
	path, err := configPath(path)
	if err != nil {
		return err
	}
	credentials, err := config.Load(path)
	if err != nil {
		return err
	}
	return sendWithCredentials(ctx, options, credentials, message)
}

func sendWithCredentials(ctx context.Context, options Options, credentials config.Credentials, message notification.Message) error {
	client := pushover.NewClient(options.HTTPClient, options.Endpoint)
	return client.Send(ctx, pushover.Message{
		AppToken: credentials.AppToken,
		UserKey:  credentials.UserKey,
		Title:    message.Title,
		Body:     message.Body,
		Priority: message.Priority,
		Sound:    message.Sound,
		TTL:      message.TTL,
	})
}

func readPayload(reader io.Reader) (map[string]any, error) {
	decoder := json.NewDecoder(io.LimitReader(reader, 1<<20))
	var payload map[string]any
	if err := decoder.Decode(&payload); errors.Is(err, io.EOF) {
		return map[string]any{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("parse hook payload: %w", err)
	}
	return payload, nil
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
	if options.Executable == "" {
		if executable, err := os.Executable(); err == nil {
			options.Executable = executable
		}
	}
	return options
}
