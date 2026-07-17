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
			installCommand(options),
			notifyCommand(options),
			testCommand(options),
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
			credentials := config.Credentials{
				AppToken: appToken,
				UserKey:  userKey,
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
			&cli.StringFlag{Name: "agent", Usage: "agent to configure: codex, claude, kimi, or pi", Required: true},
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
			resource := "hooks"
			if agent == "pi" {
				resource = "extension"
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
			&cli.StringFlag{Name: "event", Usage: "turn-complete or approval-required", Required: true},
			&cli.BoolFlag{Name: "ignore-errors", Usage: "log delivery failures without failing the hook"},
			// Kept as a no-op so hooks installed by the pre-Kimi release candidate keep working.
			&cli.BoolFlag{Name: "skip-active-stop", Hidden: true},
			configFlag(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			payload, err := readPayload(options.Stdin)
			if err != nil {
				return err
			}
			message, err := notification.Build(cmd.String("agent"), notification.Event(cmd.String("event")), payload)
			if err == nil {
				err = send(ctx, options, cmd.String("config"), message)
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
