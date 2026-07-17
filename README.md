# vibe-pushover

`vibe-pushover` sends a [Pushover](https://pushover.net/) notification when a local coding agent:

- finishes a turn;
- needs manual approval or otherwise needs your attention.

The CLI currently integrates with 10 coding agents: Claude Code, Codex CLI, GitHub Copilot CLI, Cursor, Factory Droid, Gemini CLI, Goose, Kimi Code CLI, OpenCode, and Pi. It is written in Go and uses [`urfave/cli`](https://github.com/urfave/cli).

## Install

Install the latest release on macOS or Linux:

```sh
curl -fsSL https://github.com/qiz029/vibe-pushover/releases/latest/download/install.sh | sh
```

By default the installer writes to `~/.local/bin`. Override it with `VIBE_PUSHOVER_INSTALL_DIR`, or pin a release with `VIBE_PUSHOVER_VERSION`:

```sh
curl -fsSL https://github.com/qiz029/vibe-pushover/releases/download/v0.2.0/install.sh | \
  VIBE_PUSHOVER_VERSION=v0.2.0 VIBE_PUSHOVER_INSTALL_DIR="$HOME/bin" sh
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
Notification profile [balanced/quiet/watch] (balanced):
Saved Pushover credentials to ...
```

Both credential values are hidden when setup runs in a terminal. `configure` remains available as an alias for `setup`.

The default follows Go's user config directory: `~/Library/Application Support/vibe-pushover/config.json` on macOS, and `$XDG_CONFIG_HOME/vibe-pushover/config.json` (usually `~/.config/vibe-pushover/config.json`) on Linux. The containing directory and config file are created with `0700` and `0600` permissions. Use `--config PATH` on `setup`, `install`, `test`, or `notify` to override it.

Send a real test notification:

```sh
vibe-pushover test
```

The notification profile can be viewed or changed later without re-entering the Pushover credentials:

```sh
vibe-pushover profile
vibe-pushover profile watch
```

## Install agent integrations

List the supported agents and their capabilities, then install one or more integrations:

```sh
vibe-pushover agents
vibe-pushover install --agent codex
vibe-pushover install --agent gemini
vibe-pushover install --agent opencode
```

| Agent | Notifications | Default integration path |
| --- | --- | --- |
| Claude Code | completion, approval | `~/.claude/settings.json` |
| Codex CLI | completion, approval | `~/.codex/hooks.json` |
| GitHub Copilot CLI | completion, attention | `$COPILOT_HOME/hooks/vibe-pushover.json` or `~/.copilot/hooks/vibe-pushover.json` |
| Cursor | completion | `~/.cursor/hooks.json` |
| Factory Droid | completion, attention | `~/.factory/settings.json` |
| Gemini CLI | completion | `$GEMINI_CLI_HOME/.gemini/settings.json` or `~/.gemini/settings.json` |
| Goose | completion | `~/.agents/plugins/vibe-pushover/` |
| Kimi Code CLI | completion, approval | `$KIMI_CODE_HOME/config.toml` or `~/.kimi-code/config.toml` |
| OpenCode | completion, approval | `$XDG_CONFIG_HOME/opencode/plugins/vibe-pushover.ts` or `~/.config/opencode/plugins/vibe-pushover.ts` |
| Pi | completion | `$PI_CODING_AGENT_DIR/extensions/vibe-pushover/index.ts` or `~/.pi/agent/extensions/vibe-pushover/index.ts` |

The integrations follow each agent's native hook or plugin mechanism. They preserve existing settings, only replace entries owned by `vibe-pushover`, and repeated installs are idempotent. Copilot's attention event is limited to permission and elicitation dialogs. Droid's attention event can also mean the agent has been idle and is waiting for input. Cursor, Gemini, Goose, and Pi currently expose completion notifications only through the installed integration.

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

printf '%s' '{"cwd":"/tmp/demo","message":"Waiting for input"}' | \
  vibe-pushover notify --agent my-agent --event attention-required
```

Preview a payload without loading credentials or sending anything:

```sh
printf '%s' '{"cwd":"/tmp/demo","last_assistant_message":"All tests pass."}' | \
  vibe-pushover preview --agent codex --event turn-complete --profile watch
```

## Notification format, profiles, and icon

Notifications are intentionally compact for phones and watches:

- A completed turn uses a title such as `✓ Codex finished · vibe-pushover` and only the first non-empty line of the agent's final message, truncated to 180 Unicode characters. It is silent and expires after one hour.
- An approval request uses a title such as `⚠ Codex needs approval · vibe-pushover` and shows only the tool plus its command or reason, truncated to 300 Unicode characters. It is high priority, uses Pushover's `persistent` sound, and expires after 30 minutes.
- An attention notification uses a title such as `⚠ Droid needs attention · vibe-pushover`, carries a compact reason, and uses the same high-priority delivery as an approval request.

Profiles control how noticeable those messages are:

| Profile | Completion | Approval and attention |
| --- | --- | --- |
| `balanced` | silent, low priority | persistent sound, high priority |
| `quiet` | silent, normal priority | silent, normal priority |
| `watch` | default Pushover sound, normal priority | persistent sound, high priority |

The notification icon is attached to the Pushover Application identified by the configured app token; it cannot be selected per message. To customize it, sign in to the [Pushover dashboard](https://pushover.net/), open the application whose API token you configured, and upload its icon. Pushover remains the host application, so operating-system surfaces may still show Pushover branding alongside the per-application icon.

## Development

```sh
make test
make vet
make build
```

Tests use an in-memory HTTP transport and never call the real Pushover API.
