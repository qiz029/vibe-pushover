# vibe-pushover

`vibe-pushover` sends a [Pushover](https://pushover.net/) notification when a local coding agent:

- finishes a turn;
- needs manual approval.

The CLI currently installs hooks for Codex CLI and Claude Code. It is written in Go and uses [`urfave/cli`](https://github.com/urfave/cli).

## Install

Build the binary from this repository:

```sh
go install ./cmd/vibe-pushover
```

Or build it into the repository root:

```sh
make build
```

Make sure `vibe-pushover` is on your `PATH` before installing hooks.

## Configure Pushover

Create an application in Pushover, then store its application token and your user/group key:

```sh
VIBE_PUSHOVER_TOKEN='your-app-token' \
VIBE_PUSHOVER_USER='your-user-key' \
vibe-pushover configure
```

You can also use `--token` and `--user`, but environment variables avoid putting credentials in shell history.

The default follows Go's user config directory: `~/Library/Application Support/vibe-pushover/config.json` on macOS, and `$XDG_CONFIG_HOME/vibe-pushover/config.json` (usually `~/.config/vibe-pushover/config.json`) on Linux. The containing directory and config file are created with `0700` and `0600` permissions. Use `--config PATH` on `configure`, `install`, `test`, or `notify` to override it.

Send a real test notification:

```sh
vibe-pushover test
```

## Install agent hooks

Codex CLI:

```sh
vibe-pushover install --agent codex
```

Claude Code:

```sh
vibe-pushover install --agent claude
```

The installer adds `Stop` and `PermissionRequest` command hooks. It preserves existing settings and hooks, and repeated installs are idempotent. Defaults:

| Agent | Config file |
| --- | --- |
| Codex CLI | `~/.codex/hooks.json` |
| Claude Code | `~/.claude/settings.json` |

Use `--agent-config PATH` to target another agent settings file or `--binary PATH` when installing a binary that is not the currently running executable. If credentials were written with `configure --config PATH`, pass the same path to `install --config PATH`; it will be embedded in both installed hook commands.

Restart the agent after installation. Codex may ask you to trust the newly added local hooks before it runs them.

Installed hooks use `--ignore-errors`: a network or Pushover failure is written to the agent's stderr but does not fail the agent turn.

## Hook command

The installed command is also usable with any agent that sends a JSON hook payload on stdin:

```sh
printf '%s' '{"cwd":"/tmp/demo","last_assistant_message":"Done"}' | \
  vibe-pushover notify --agent my-agent --event turn-complete

printf '%s' '{"cwd":"/tmp/demo","tool_name":"Bash","tool_input":{"command":"make deploy"}}' | \
  vibe-pushover notify --agent my-agent --event approval-required
```

For completed turns, the notification includes the project directory and, when supplied, the last assistant message. Approval notifications include the tool name and command/reason when supplied. Pushover limits are enforced by truncating notification bodies to 1,024 Unicode characters.

## Development

```sh
make test
make vet
make build
```

Tests use an in-memory HTTP transport and never call the real Pushover API.
