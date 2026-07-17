# vibe-pushover

`vibe-pushover` sends a [Pushover](https://pushover.net/) notification when a local coding agent:

- finishes a turn;
- needs manual approval.

The CLI currently installs hooks for Codex CLI, Claude Code, and Kimi Code CLI, plus a Pi extension. It is written in Go and uses [`urfave/cli`](https://github.com/urfave/cli).

## Install

Install the latest release on macOS or Linux:

```sh
curl -fsSL https://github.com/qiz029/vibe-pushover/releases/latest/download/install.sh | sh
```

By default the installer writes to `~/.local/bin`. Override it with `VIBE_PUSHOVER_INSTALL_DIR`, or pin a release with `VIBE_PUSHOVER_VERSION`:

```sh
curl -fsSL https://github.com/qiz029/vibe-pushover/releases/download/v0.1.0/install.sh | \
  VIBE_PUSHOVER_VERSION=v0.1.0 VIBE_PUSHOVER_INSTALL_DIR="$HOME/bin" sh
```

`VIBE_PUSHOVER_DOWNLOAD_BASE_URL` can point the installer at a trusted mirror; when set, `VIBE_PUSHOVER_VERSION` is also required.

Build from source instead:

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

Kimi Code CLI:

```sh
vibe-pushover install --agent kimi
```

Pi:

```sh
vibe-pushover install --agent pi
```

For Codex, Claude, and Kimi, the installer adds `Stop` and `PermissionRequest` command hooks. For Pi, it installs a global extension that sends a notification on `agent_settled`, after automatic retries, compaction, and queued follow-ups are finished. It preserves existing settings and hooks, and repeated installs are idempotent. Defaults:

| Agent | Config file |
| --- | --- |
| Codex CLI | `~/.codex/hooks.json` |
| Claude Code | `~/.claude/settings.json` |
| Kimi Code CLI | `$KIMI_CODE_HOME/config.toml` when set; otherwise `~/.kimi-code/config.toml` |
| Pi | `$PI_CODING_AGENT_DIR/extensions/vibe-pushover/index.ts` when set; otherwise `~/.pi/agent/extensions/vibe-pushover/index.ts` |

Use `--agent-config PATH` to target another agent settings file or `--binary PATH` when installing a binary that is not the currently running executable. If credentials were written with `setup --config PATH`, pass the same path to `install --config PATH`; it will be embedded in both installed hook commands.

Restart the agent after installation. Codex may ask you to trust the newly added local hooks before it runs them. Kimi loads the new TOML hooks when a new session starts; its `Stop` event does not include the final assistant message, so the completion notification uses the compact fallback body `Turn completed.`. Kimi exposes `Stop` just before the turn ends and runs hooks in parallel. If another Kimi `Stop` hook blocks the turn, the notification may arrive before that continuation finishes because Kimi does not expose a later turn-ended hook.

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

## Notification format and icon

Notifications are intentionally compact for phones and watches:

- A completed turn uses a title such as `✓ Codex finished · vibe-pushover` and only the first non-empty line of the agent's final message, truncated to 180 Unicode characters. It is silent and expires after one hour.
- An approval request uses a title such as `⚠ Codex needs approval · vibe-pushover` and shows only the tool plus its command or reason, truncated to 300 Unicode characters. It is high priority, uses Pushover's `persistent` sound, and expires after 30 minutes.

The notification icon is attached to the Pushover Application identified by the configured app token; it cannot be selected per message. To customize it, sign in to the [Pushover dashboard](https://pushover.net/), open the application whose API token you configured, and upload its icon. Pushover remains the host application, so operating-system surfaces may still show Pushover branding alongside the per-application icon.

## Development

```sh
make test
make vet
make build
```

Tests use an in-memory HTTP transport and never call the real Pushover API.
