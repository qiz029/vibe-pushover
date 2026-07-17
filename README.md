# vibe-pushover

`vibe-pushover` sends a [Pushover](https://pushover.net/) notification when a local coding agent:

- finishes a turn;
- needs manual approval or otherwise needs your attention.

The CLI currently integrates with 26 coding agents: Aider, Amp, Augment Auggie, Claude Code, Cline, Codex CLI, GitHub Copilot CLI, Snowflake Cortex Code, Cursor, Factory Droid, Gemini CLI, Goose, Grok Build, Hermes Agent, Kimi Code CLI, Kiro, MiMo Code, Mistral Vibe, Oh My Pi, OpenCode, Pi, Qoder, Qwen Code, TRAE, VS Code Agent, and Windsurf. It is written in Go and uses [`urfave/cli`](https://github.com/urfave/cli).

## Install

Install the latest release on macOS or Linux:

```sh
curl -fsSL https://github.com/qiz029/vibe-pushover/releases/latest/download/install.sh | sh
```

By default the installer writes to `~/.local/bin`. Override it with `VIBE_PUSHOVER_INSTALL_DIR`, or pin a release with `VIBE_PUSHOVER_VERSION`:

```sh
curl -fsSL https://github.com/qiz029/vibe-pushover/releases/download/v0.11.1/install.sh | \
  VIBE_PUSHOVER_VERSION=v0.11.1 VIBE_PUSHOVER_INSTALL_DIR="$HOME/bin" sh
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
Notification profile [balanced/quiet/urgent/watch] (balanced):
Target Pushover device(s), comma-separated (all; groups may ignore):
Saved Pushover credentials to ...
```

Both credential values are hidden when setup runs in a terminal. `configure` remains available as an alias for `setup`.

The default follows Go's user config directory: `~/Library/Application Support/vibe-pushover/config.json` on macOS, and `$XDG_CONFIG_HOME/vibe-pushover/config.json` (usually `~/.config/vibe-pushover/config.json`) on Linux. The containing directory and config file are created with `0700` and `0600` permissions. Use `--config PATH` on `setup`, `profile`, `device`, `snooze`, `install`, `test`, or `notify` to override it.

Send a real test notification:

```sh
vibe-pushover test
vibe-pushover test --event turn-complete --message "Completion style looks good."
```

The default test simulates an approval request so its high-priority sound, expiry, configured device target, and Pushover application icon can be checked end to end. Use `--event turn-complete` or `--event attention-required` to exercise the other styles. Tests honor the configured profile and temporary snooze state, so suppressed delivery is reported instead of sending a misleading notification. Add `--force` when you deliberately want to test delivery while snoozed.

The notification profile can be viewed or changed later without re-entering the Pushover credentials:

```sh
vibe-pushover profile
vibe-pushover profile watch
vibe-pushover profile urgent

vibe-pushover device
vibe-pushover device iphone
vibe-pushover device iphone,ipad
vibe-pushover device all
```

Temporarily pause every hook notification without changing the permanent profile:

```sh
vibe-pushover snooze 30m
vibe-pushover snooze 2h
vibe-pushover snooze       # show current status
vibe-pushover snooze off   # resume immediately
```

`pause` is an alias for `snooze`. Expired snoozes automatically stop suppressing delivery.

Targeting a device is optional. It is useful when the same Pushover account is active on several computers or phones and you only want coding-agent alerts on the phone that mirrors to your watch. Device names come from the Pushover dashboard, use letters, numbers, `_`, or `-`, and are limited to 25 characters each. Pushover ignores device targeting for ordinary groups and multi-user requests; a single Team-owned group can filter by device name. If a named device is invalid or no longer enabled, Pushover may broadcast to all active devices instead of dropping the message.

## Install agent integrations

List the supported agents and their capabilities, then install one or more integrations:

```sh
vibe-pushover agents
vibe-pushover install --agent cline
vibe-pushover install --agent codex
vibe-pushover install --agent gemini
vibe-pushover install --agent grok
vibe-pushover install --agent mimo
vibe-pushover install --agent mistral
vibe-pushover install --agent omp
vibe-pushover install --agent opencode
vibe-pushover install --agent trae
```

| Agent | Notifications | Default integration path |
| --- | --- | --- |
| Aider | completion (macOS/Linux) | `~/.aider.conf.yml` plus `~/.aider/vibe-pushover-notify.sh` |
| Amp | completion, approval, error attention | `~/.config/amp/plugins/vibe-pushover.ts` |
| Augment Auggie | completion (macOS/Linux) | `~/.augment/settings.json` plus `~/.augment/hooks/vibe-pushover.sh` |
| Claude Code | completion, approval | `~/.claude/settings.json` |
| Cline | completion | `<Documents>/Cline/Hooks/TaskComplete` (`TaskComplete.ps1` on Windows); when Windows My Documents or Linux XDG Documents is redirected, also `$CLINE_DIR/hooks/TaskComplete[.ps1]` or `~/.cline/hooks/TaskComplete[.ps1]` for CLI |
| Codex CLI | completion, approval | `~/.codex/hooks.json` |
| GitHub Copilot CLI | completion, attention | `$COPILOT_HOME/hooks/vibe-pushover.json` or `~/.copilot/hooks/vibe-pushover.json` |
| Snowflake Cortex Code | completion, approval | `~/.snowflake/cortex/hooks.json` (preview hook API) |
| Cursor | completion | `~/.cursor/hooks.json` |
| Factory Droid | completion, attention | `~/.factory/settings.json` |
| Gemini CLI | completion | `$GEMINI_CLI_HOME/.gemini/settings.json` or `~/.gemini/settings.json` |
| Goose | completion | `~/.agents/plugins/vibe-pushover/` |
| Grok Build | completion, failure attention | `$GROK_HOME/hooks/vibe-pushover.json` or `~/.grok/hooks/vibe-pushover.json` |
| Hermes Agent | completion, approval | `$HERMES_HOME/config.yaml` or `~/.hermes/config.yaml` |
| Kimi Code CLI | completion, approval | `$KIMI_CODE_HOME/config.toml` or `~/.kimi-code/config.toml` |
| Kiro | completion (macOS/Linux) | `~/.kiro/hooks/vibe-pushover.json` |
| MiMo Code | completion, approval | `$MIMOCODE_HOME/config/plugins/vibe-pushover.ts`; otherwise `$XDG_CONFIG_HOME/mimocode/plugins/vibe-pushover.ts`, `~/.config/mimocode/plugins/vibe-pushover.ts` (macOS/Linux), or `%LOCALAPPDATA%\mimocode\plugins\vibe-pushover.ts` (Windows) |
| Mistral Vibe | completion | `$VIBE_HOME/hooks.toml` or `~/.vibe/hooks.toml` (experimental hook API) |
| Oh My Pi | completion, approval | `$PI_CODING_AGENT_DIR/extensions/vibe-pushover/index.ts` or `~/.omp/agent/extensions/vibe-pushover/index.ts` |
| OpenCode | completion, approval | `$XDG_CONFIG_HOME/opencode/plugins/vibe-pushover.ts` or `~/.config/opencode/plugins/vibe-pushover.ts` |
| Pi | completion | `$PI_CODING_AGENT_DIR/extensions/vibe-pushover/index.ts` or `~/.pi/agent/extensions/vibe-pushover/index.ts` |
| Qoder | completion | `~/.qoder/settings.json` |
| Qwen Code | completion, approval, idle attention | `~/.qwen/settings.json` |
| TRAE | completion, approval | `~/.trae/hooks.json` |
| VS Code Agent | completion | `$COPILOT_HOME/hooks/vibe-pushover.json` or `~/.copilot/hooks/vibe-pushover.json` (preview hook API) |
| Windsurf | completion | `~/.codeium/windsurf/hooks.json` |

The integrations follow each agent's native hook, plugin, or notification-command mechanism. They preserve existing settings, only replace entries owned by `vibe-pushover`, and repeated installs are idempotent. Copilot's attention event is limited to permission and elicitation dialogs. Droid's attention event can also mean the agent has been idle and is waiting for input. Amp reports its `awaiting-approval` state separately from turn errors. Grok Build maps its top-level `Stop` event to completion and `StopFailure` to attention; its separate `SubagentStop` event is deliberately not installed, so child completions stay quiet. MiMo Code uses its native OpenCode-compatible `session.idle` and `permission.asked` plugin events, but installs into MiMo Code's own config tree and does not share the OpenCode plugin. Qwen sends separate approval notifications and idle-input attention notifications; re-entered active `Stop` hooks are filtered to avoid duplicate or premature completion messages. TRAE maps its top-level `Stop` hook to completion and only its `Notification` events matched as `permission_prompt` to approval; installation preserves unrelated and third-party hooks in the same global manifest. Qoder applies the same active-Stop filter. Hermes sends approval notifications only for human-facing CLI or gateway decisions and skips `approvals.mode=smart` automatic decisions. Oh My Pi ignores `agent_end` events that announce an automatic continuation and reports its native tool-approval event separately. When session logging is enabled, Mistral Vibe filters inherited subagent `post_agent_turn` events using their official `agents/<session>/messages.jsonl` layout; with logging disabled, its payload does not identify subagents, so fan-out can produce extra completion notifications. Aider, Auggie, Cline, Cursor, Gemini, Goose, Kiro, Mistral Vibe, Pi, Qoder, VS Code Agent, and Windsurf currently expose completion notifications only through the installed integration.

Because Aider supports only one `notifications-command`, installation refuses to replace an existing custom command. Remove or compose that command yourself before retrying if you want `vibe-pushover` to own the setting. Cline also supports one file per event in a hook root; installation refuses to replace a non-`vibe-pushover` `TaskComplete` hook.

Use `--agent-config PATH` to target another agent settings file or `--binary PATH` when installing a binary that is not the currently running executable. If credentials were written with `setup --config PATH`, pass the same path to `install --config PATH`; it will be embedded in both installed hook commands.

Restart the agent after installation. In Amp, use `plugins: reload` from the command palette instead if you want to activate the generated plugin without restarting. Codex may ask you to trust the newly added local hooks before it runs them. Hermes asks for first-use consent for each installed `(event, command)` pair; approve it interactively or manage it with `hermes hooks`, rather than enabling `hooks_auto_accept` globally. Cortex Code and VS Code Agent hook support are currently preview features. Mistral Vibe installation also enables its experimental hook gate in the sibling `config.toml`; Vibe currently exposes a reliable completion hook but no hook that proves a tool is actually waiting for manual approval. Copilot CLI and VS Code Agent discover the same compatible manifest under `~/.copilot/hooks`, so installing either integration configures one shared completion hook; the notification source is detected from each hook payload instead of registering duplicate hooks. Oh My Pi discovers the generated extension automatically unless extension discovery is disabled; named OMP profiles have separate agent directories and should be installed with `--agent-config` for that profile. Auggie's Unix wrapper filters interrupted, error, maximum-iteration, and malformed stops, so only its normal `end_turn` cause sends a completion notification. Windsurf notifications extract the final content line from the Cascade response instead of forwarding the full response payload.

Kimi loads the new TOML hooks when a new session starts; its `Stop` event does not include the final assistant message, so the completion notification uses the compact fallback body `Turn completed.`. Kimi exposes `Stop` just before the turn ends and runs hooks in parallel. If another Kimi `Stop` hook blocks the turn, the notification may arrive before that continuation finishes because Kimi does not expose a later turn-ended hook.

Pi deliberately has no built-in permission popups, so its integration currently sends turn-complete notifications only. `vibe-pushover` does not add a confirmation policy or turn every Pi tool call into a manual approval.

Cline installs its stable `TaskComplete` hook into the operating system's real Documents directory. At the standard `~/Documents` location that one hook is discovered by both the IDE and current CLI/SDK runtime, avoiding duplicate execution. If Windows My Documents or Linux XDG Documents is redirected elsewhere, installation also writes the CLI copy under `~/.cline/hooks`; both paths are ownership-checked before either is changed. It extracts the compact result from either the IDE's `taskComplete.taskMetadata.result` or the newer CLI/SDK `turn.outputText` payload, and ignores child-agent completions identified by `parent_agent_id`. Cline exposes pre-tool hooks but no dedicated event proving that a tool is blocked waiting for manual approval, so `vibe-pushover` deliberately reports completion only instead of producing false approval alerts. Use `--agent-config PATH` with the full `TaskComplete` hook file path when intentionally targeting a custom Cline hooks directory.

Roo Code, Continue, and Crush have been audited but are not listed as supported because they do not currently expose a stable user-level turn-complete or approval hook suitable for this integration. Crush's preliminary hook API currently exposes only `PreToolUse`; its built-in desktop notifications are not an external lifecycle hook. Warp already provides its own desktop notifications for completed agents and agents that need attention, but does not expose a corresponding external lifecycle hook. `vibe-pushover` intentionally avoids log polling for these tools because it would be fragile and could leak conversation content.

Installed hooks use `--ignore-errors`: a network or Pushover failure is written to the agent's stderr but does not fail the agent turn.

## Hook command

The installed command is also usable with any agent that sends a JSON hook payload on stdin:

```sh
printf '%s' '{"cwd":"/tmp/demo","last_assistant_message":"Done"}' | \
  vibe-pushover notify --agent my-agent --event turn-complete

printf '%s' '{"cwd":"/tmp/demo","message":"Done","session_url":"https://example.com/session/42"}' | \
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

- A completed turn uses a title such as `✓ Codex finished · vibe-pushover` and the first useful line of the agent's final message, skipping Markdown headings when a result line follows and truncating it to 180 Unicode characters. It is silent and expires after one hour.
- An approval request uses a title such as `⚠ Codex needs approval · vibe-pushover` and shows only the tool plus its command or reason, truncated to 300 Unicode characters. It is high priority, uses Pushover's `persistent` sound, and expires after 30 minutes.
- An attention notification uses a title such as `⚠ Droid needs attention · vibe-pushover`, carries a compact reason, and uses the same high-priority delivery as an approval request.
- If a hook payload supplies an HTTP(S) `url`, `session_url`, `web_url`, or `details_url`, the notification includes Pushover's supplementary `Open result` or `Open agent` action. Unsafe and local-only URL schemes are ignored.
- By default Pushover delivers to every active device on the account. `setup` or `vibe-pushover device ...` can restrict delivery to one or more named devices without changing the agent hooks.

Hook delivery also suppresses exact repeats for three seconds across CLI processes. The fingerprint includes the agent, event, a non-secret hash of the Pushover destination, rendered notification fields, and stable session/turn/tool IDs when available. A failed Pushover request releases its reservation so a later hook can retry; an unavailable or corrupt dedupe cache fails open and sends the notification with a warning. The private cache is stored at `~/Library/Caches/vibe-pushover/dedupe.json` on macOS or `$XDG_CACHE_HOME/vibe-pushover/dedupe.json` on Linux.

`snooze` temporarily suppresses all hook delivery and stores only the expiry timestamp beside the existing local configuration. Delivery resumes automatically after the deadline or immediately after `vibe-pushover snooze off`; `vibe-pushover test --force` remains available for an intentional end-to-end check during a snooze.

Profiles control how noticeable those messages are:

| Profile | Completion | Approval and attention |
| --- | --- | --- |
| `balanced` | silent, low priority | persistent sound, high priority |
| `quiet` | silent, normal priority | silent, normal priority |
| `urgent` | suppressed | persistent sound, high priority |
| `watch` | default Pushover sound, normal priority | persistent sound, high priority |

Use `urgent` when completion messages are too noisy but approval and attention requests must still reach you. `preview --profile urgent --event turn-complete` reports that the delivery would be suppressed instead of showing a notification that will not be sent.

The notification icon is attached to the Pushover Application identified by the configured app token; it cannot be selected per message. To customize it, sign in to the [Pushover dashboard](https://pushover.net/), open the application whose API token you configured, and upload its icon. Pushover remains the host application, so operating-system surfaces may still show Pushover branding alongside the per-application icon.

## Development

```sh
make test
make vet
make build
```

Tests use an in-memory HTTP transport and never call the real Pushover API.
