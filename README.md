# vibe-pushover

`vibe-pushover` sends a [Pushover](https://pushover.net/) notification when a local coding agent:

- finishes a turn;
- needs manual approval.

The CLI currently installs hooks for Codex CLI and Claude Code, plus a Pi extension. It is written in Go and uses [`urfave/cli`](https://github.com/urfave/cli).

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

Create an application in Pushover, then run the interactive setup:

```sh
vibe-pushover setup

Pushover application token:
Pushover user/group key:
Saved Pushover credentials to ...
```

Both credential values are hidden when setup runs in a terminal. `configure` remains available as an alias for `setup`.

The default follows Go's user config directory: `~/Library/Application Support/vibe-pushover/config.json` on macOS, and `$XDG_CONFIG_HOME/vibe-pushover/config.json` (usually `~/.config/vibe-pushover/config.json`) on Linux. The containing directory and config file are created with `0700` and `0600` permissions. Use `--config PATH` on `setup`, `install`, `test`, or `notify` to override it.

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

Pi:

```sh
vibe-pushover install --agent pi
```

For Codex and Claude, the installer adds `Stop` and `PermissionRequest` command hooks. For Pi, it installs a global extension that sends a notification on `agent_settled`, after automatic retries, compaction, and queued follow-ups are finished. It preserves existing settings and hooks, and repeated installs are idempotent. Defaults:

| Agent | Config file |
| --- | --- |
| Codex CLI | `~/.codex/hooks.json` |
| Claude Code | `~/.claude/settings.json` |
| Pi | `$PI_CODING_AGENT_DIR/extensions/vibe-pushover/index.ts` when set; otherwise `~/.pi/agent/extensions/vibe-pushover/index.ts` |

Use `--agent-config PATH` to target another agent settings file or `--binary PATH` when installing a binary that is not the currently running executable. If credentials were written with `setup --config PATH`, pass the same path to `install --config PATH`; it will be embedded in both installed hook commands.

Restart the agent after installation. Codex may ask you to trust the newly added local hooks before it runs them.

Pi deliberately has no built-in permission popups, so its integration currently sends turn-complete notifications only. `vibe-pushover` does not add a confirmation policy or turn every Pi tool call into a manual approval.

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
