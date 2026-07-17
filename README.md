# vibe-pushover

`vibe-pushover` sends a [Pushover](https://pushover.net/) notification when a local coding agent:

- finishes a turn or wrapped CLI session;
- needs manual approval or otherwise needs your attention.

The CLI currently integrates with 44 coding agents and compatible runtimes. Forty-one use their native hook, plugin, extension, automation, or notification-command surface: Aider, Amp, Antigravity CLI, Autohand Code, Augment Auggie, Claude Code, Claude Code Router, Cline, CodeBuddy Code, CodeWhale (formerly DeepSeek-TUI), Codex CLI, GitHub Copilot CLI, Craft Agents, Snowflake Cortex Code, Cursor, Factory Droid, DotCraft, Gajae Code, Gemini CLI, Goose, Grok Build, Hermes Agent, JetBrains Junie CLI, Kimi Code CLI, Kiro, Kilo Code, MiMo Code, Mistral Vibe, Oh My Pi, OpenHands CLI, OpenCode, Pi, Qoder, Qwen Code, Rovo Dev CLI, Tabnine CLI, TRAE, VS Code Agent, Windsurf, WorkBuddy, and ZCode. Continue CLI, Crush, and Plandex are supported through the transparent `run` wrapper for session-exit and failure notifications. It is written in Go and uses [`urfave/cli`](https://github.com/urfave/cli).

## Install

Install the latest release on macOS or Linux:

```sh
curl -fsSL https://github.com/qiz029/vibe-pushover/releases/latest/download/install.sh | sh
```

By default the installer writes to `~/.local/bin`. Override it with `VIBE_PUSHOVER_INSTALL_DIR`, or pin a release with `VIBE_PUSHOVER_VERSION`:

```sh
curl -fsSL https://github.com/qiz029/vibe-pushover/releases/download/v0.23.0/install.sh | \
  VIBE_PUSHOVER_VERSION=v0.23.0 VIBE_PUSHOVER_INSTALL_DIR="$HOME/bin" sh
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
Notification profile [balanced/quiet/urgent/watch/on-call] (balanced):
Notification detail [summary/minimal/private] (summary):
Target Pushover device(s), comma-separated (all; groups may ignore):
Saved Pushover credentials to ...
```

Both credential values are hidden when setup runs in a terminal. `configure` remains available as an alias for `setup`.

The default follows Go's user config directory: `~/Library/Application Support/vibe-pushover/config.json` on macOS, and `$XDG_CONFIG_HOME/vibe-pushover/config.json` (usually `~/.config/vibe-pushover/config.json`) on Linux. The containing directory and config file are created with `0700` and `0600` permissions. Use `--config PATH` on `setup`, `status`, `profile`, `detail`, `encryption`, `device`, `sound`, `snooze`, `focus`, `quiet-hours`, `silence`, `install`, `run`, `preview`, `test`, or `notify` to override it.

Inspect every delivery control in one credential-safe summary:

```sh
vibe-pushover status
```

`status` shows the profile, message-detail mode, target devices, whether end-to-end encryption is enabled, current snooze and focus deadlines, quiet-hours schedule and whether it is active now, silence-rule count, and effective event sounds. For `on-call`, it also makes the emergency retry interval and maximum alert window explicit. It never prints the Pushover application token, user/group key, or encryption key.

Optional Pushover v5 end-to-end encryption protects the notification title, body, and supplementary URL from the Pushover service. Generate a key locally, enter the displayed key in Pushover v5 on every target iOS/Android device, and send a test notification:

```sh
vibe-pushover encryption enable
vibe-pushover test
```

The key is shown only by `enable` and `rotate`; `status` reports only `on` or `off`. Use `vibe-pushover encryption set` to enter an existing 64-character hexadecimal key through a hidden prompt, `vibe-pushover encryption rotate` to replace it, or `vibe-pushover encryption disable` to return to HTTPS transport encryption only. Rotation requires updating every target device before it can decrypt new messages. Pushover currently documents E2EE support for its v5 iOS and Android apps; when the account also has desktop or browser clients, target the configured mobile device with `vibe-pushover device NAME` before enabling encryption. Pushover still receives delivery metadata such as the application token, recipient, device, priority, sound, and timestamp. See Pushover's Message API [end-to-end encryption specification](https://pushover.net/api#end-to-end-encryption) and [security overview](https://support.pushover.net/i46-are-messages-notifications-encrypted).

Send a real test notification:

```sh
vibe-pushover test
vibe-pushover test --event turn-complete --message "Completion style looks good."
```

The default test simulates an approval request so its high-priority sound, expiry, configured device target, and Pushover application icon can be checked end to end. Use `--event turn-complete` or `--event attention-required` to exercise the other styles. Tests honor the configured profile, snooze state, focus mode, quiet hours, and silence rules, so suppressed delivery is reported instead of sending a misleading notification. Add `--force` when you deliberately want to test a suppressed delivery. After an `on-call` blocker test, the CLI reminds you that the emergency notification repeats until acknowledged or its 15-minute window expires.

The notification profile can be viewed or changed later without re-entering the Pushover credentials:

```sh
vibe-pushover profile
vibe-pushover profile watch
vibe-pushover profile urgent
vibe-pushover profile on-call

vibe-pushover detail
vibe-pushover detail minimal
vibe-pushover detail private
vibe-pushover detail summary

vibe-pushover device
vibe-pushover device iphone
vibe-pushover device iphone,ipad
vibe-pushover device all
```

`detail summary` keeps the compact first useful line, tool command, or failure reason in the notification body. Multi-line approval commands show only the first effective line and an omitted-line count, so heredocs and script bodies are not dumped onto the lock screen. `detail minimal` replaces hook-provided body content with an event-only message such as `Turn completed.` or `Approval requested.` while retaining the agent and project in the title. `detail private` also removes the project name and supplementary action URL, leaving only the agent and event on a phone or watch lock screen. Existing configurations default to `summary`.

Temporarily pause every hook notification without changing the permanent profile:

```sh
vibe-pushover snooze 30m
vibe-pushover snooze 2h
vibe-pushover snooze       # show current status
vibe-pushover snooze off   # resume immediately
```

`pause` is an alias for `snooze`. Expired snoozes automatically stop suppressing delivery.

Keep blocker notifications while temporarily silencing routine completions:

```sh
vibe-pushover focus 45m
vibe-pushover focus 2h
vibe-pushover focus       # show current status
vibe-pushover focus off   # resume completion notifications
```

While focus mode is active, approval and attention notifications continue normally. Only `turn-complete` is suppressed. `blockers-only` is an alias for `focus`, and the mode expires automatically.

Schedule the same blocker-only behavior every day using local 24-hour time. Overnight windows are supported:

```sh
vibe-pushover quiet-hours 22:00 08:00
vibe-pushover quiet-hours             # show the schedule
vibe-pushover quiet-hours off         # disable the schedule
```

Quiet hours recur in the machine's current local timezone and suppress only `turn-complete`; approval and attention notifications remain active. Start is inclusive and end is exclusive.

Permanently silence routine notifications from an exact agent, project directory name, or both:

```sh
vibe-pushover silence add --agent codex
vibe-pushover silence add --project private-repo
vibe-pushover silence add --agent gajae --project demo
vibe-pushover silence add --project private-repo --event all
vibe-pushover silence
vibe-pushover silence remove 1
vibe-pushover silence clear
```

New rules default to `turn-complete`, so approval and attention alerts keep working. Use `--event all` only when every event for the match should be silent. When both `--agent` and `--project` are present, both must match; agent and project matching is case-insensitive.

Choose a distinct Pushover sound for each event without reinstalling any agent hooks:

```sh
vibe-pushover sound
vibe-pushover sound turn-complete magic
vibe-pushover sound approval-required persistent
vibe-pushover sound attention-required default  # use the account/device default
vibe-pushover sound turn-complete reset           # restore vibe-pushover's preset
```

Built-in names include `pushover`, `magic`, `incoming`, `persistent`, `vibrate`, and `none`. Names of custom sounds uploaded to the Pushover account that owns the application token are accepted too. The `quiet` profile always stays silent; event-specific choices override the sound preset of the other profiles.

Targeting a device is optional. It is useful when the same Pushover account is active on several computers or phones and you only want coding-agent alerts on the phone that mirrors to your watch. Device names come from the Pushover dashboard, use letters, numbers, `_`, or `-`, and are limited to 25 characters each. Pushover ignores device targeting for ordinary groups and multi-user requests; a single Team-owned group can filter by device name. If a named device is invalid or no longer enabled, Pushover may broadcast to all active devices instead of dropping the message.

## Install agent integrations

List the supported agents and their capabilities, then install one or more integrations:

```sh
vibe-pushover agents
vibe-pushover agents --detected
vibe-pushover install --detected
vibe-pushover install --agent antigravity
vibe-pushover install --agent autohand
vibe-pushover install --agent claude-router
vibe-pushover install --agent cline
vibe-pushover install --agent codebuddy
vibe-pushover install --agent codewhale
vibe-pushover install --agent codex
vibe-pushover install --agent craft
vibe-pushover install --agent dotcraft
vibe-pushover install --agent gajae
vibe-pushover install --agent gemini
vibe-pushover install --agent grok
vibe-pushover install --agent junie
vibe-pushover install --agent kilo
vibe-pushover install --agent mimo
vibe-pushover install --agent mistral
vibe-pushover install --agent omp
vibe-pushover install --agent openhands
vibe-pushover install --agent opencode
vibe-pushover install --agent rovo
vibe-pushover install --agent tabnine
vibe-pushover install --agent trae
vibe-pushover install --agent workbuddy
vibe-pushover install --agent zcode
```

`deepseek` is accepted as an install alias for `codewhale` to support existing DeepSeek-TUI users.

`agents --detected` is a read-only preview of supported agent configuration homes and curated CLI executables found on the machine. This means a newly installed CLI can be recognized before its first run creates a config directory. The native-integration PATH signals are `aider`, `amp`, `autohand`, `auggie`, `claude`, `ccr`, `cline`, `codebuddy`, `codex`, `copilot`, `gemini`, `junie`, `kimi`, `kiro-cli`, `kilo`, `omp`, `openhands`, `opencode`, `pi`, `qodercli`, `qwen`, and `tabnine`; wrapper detection also recognizes `cn`, `crush`, `plandex`, and `pdx`. IDE-only agents and collision-prone command names such as `code`, `craft`, `goose`, and `vibe` remain configuration-marker-only to avoid false positives.

`install --detected` installs every native-integration item in that preview, preserves unrelated settings and third-party hooks, and remains safe to repeat. Detected run-wrapper agents need no files installed and are reported separately. Detection never starts an agent or creates configuration for agents that were not found; a stale configuration home may still be reported. When `PI_CODING_AGENT_DIR` is set, a single matching `pi` or `omp` executable on PATH identifies the runtime. If both or neither executable is present, choose `--agent pi` or `--agent omp` explicitly because the shared directory override cannot distinguish the two runtimes safely.

| Agent | Notifications | Default integration path |
| --- | --- | --- |
| Aider | completion (macOS/Linux) | `~/.aider.conf.yml` plus `~/.aider/vibe-pushover-notify.sh` |
| Amp | completion, approval, error attention | `~/.config/amp/plugins/vibe-pushover.ts` |
| Antigravity CLI | completion, failure attention | `~/.gemini/antigravity-cli/plugins/vibe-pushover/` |
| Autohand Code | completion, approval, error attention | `$AUTOHAND_CONFIG` or `~/.autohand/config.json` |
| Augment Auggie | completion (macOS/Linux) | `~/.augment/settings.json` plus `~/.augment/hooks/vibe-pushover.sh` |
| Claude Code | completion, approval | `~/.claude/settings.json` |
| Claude Code Router | completion, approval through Claude Code | shared `~/.claude/settings.json` |
| Cline | completion | `<Documents>/Cline/Hooks/TaskComplete` (`TaskComplete.ps1` on Windows); when Windows My Documents or Linux XDG Documents is redirected, also `$CLINE_DIR/hooks/TaskComplete[.ps1]` or `~/.cline/hooks/TaskComplete[.ps1]` for CLI |
| CodeBuddy Code | completion, approval, failure attention | `~/.codebuddy/settings.json` (beta hook API) |
| CodeWhale (DeepSeek-TUI) | completion, error attention | `$CODEWHALE_CONFIG_PATH`, `$DEEPSEEK_CONFIG_PATH`, `$CODEWHALE_HOME/config.toml`, existing `~/.codewhale/config.toml`, or legacy `~/.deepseek/config.toml` |
| Codex CLI | completion, approval | `~/.codex/hooks.json` |
| Continue CLI | session exit, process failure | `vibe-pushover run --agent continue -- cn ...` |
| GitHub Copilot CLI | completion, approval, question attention | `$COPILOT_HOME/hooks/vibe-pushover.json` or `~/.copilot/hooks/vibe-pushover.json` |
| Craft Agents | completion, approval, idle attention | `$CRAFT_CONFIG_DIR/workspaces/*/automations.json` or `~/.craft-agent/workspaces/*/automations.json` |
| Crush | session exit, process failure | `vibe-pushover run --agent crush -- crush ...` |
| Snowflake Cortex Code | completion, approval | `~/.snowflake/cortex/hooks.json` (preview hook API) |
| Cursor | completion | `~/.cursor/hooks.json` |
| Factory Droid | completion, attention | `~/.factory/settings.json` |
| DotCraft | completion, approval, Stop-hook failure attention | `~/.craft/hooks.json` |
| Gajae Code | completion | `$GJC_CODING_AGENT_DIR/config.yml`, `~/<GJC_CONFIG_DIR>/agent/config.yml`, or `~/.gjc/agent/config.yml` |
| Gemini CLI | completion, approval | `$GEMINI_CLI_HOME/.gemini/settings.json` or `~/.gemini/settings.json` |
| Goose | completion | `~/.agents/plugins/vibe-pushover/` |
| Grok Build | completion, failure attention | `$GROK_HOME/hooks/vibe-pushover.json` or `~/.grok/hooks/vibe-pushover.json` |
| Hermes Agent | completion, approval | `$HERMES_HOME/config.yaml` or `~/.hermes/config.yaml` |
| JetBrains Junie CLI | completion, approval, model/API failure attention | `~/.junie/config.json` (Early Access hooks) |
| Kimi Code CLI | completion, approval | `$KIMI_CODE_HOME/config.toml` or `~/.kimi-code/config.toml` |
| Kiro | completion (macOS/Linux) | `~/.kiro/hooks/vibe-pushover.json` |
| Kilo Code | completion, approval, error attention | `$XDG_CONFIG_HOME/kilo/plugin/vibe-pushover.ts`, `~/.config/kilo/plugin/vibe-pushover.ts` (macOS/Linux), or `%APPDATA%\kilo\plugin\vibe-pushover.ts` (Windows) |
| MiMo Code | completion, approval, error attention | `$MIMOCODE_HOME/config/plugins/vibe-pushover.ts`; otherwise `$XDG_CONFIG_HOME/mimocode/plugins/vibe-pushover.ts`, `~/.config/mimocode/plugins/vibe-pushover.ts` (macOS/Linux), or `%LOCALAPPDATA%\mimocode\plugins\vibe-pushover.ts` (Windows) |
| Mistral Vibe | completion | `$VIBE_HOME/hooks.toml` or `~/.vibe/hooks.toml` (experimental hook API) |
| Oh My Pi | completion, approval | `$PI_CODING_AGENT_DIR/extensions/vibe-pushover/index.ts` or `~/.omp/agent/extensions/vibe-pushover/index.ts` |
| OpenHands CLI | completion | `~/.openhands/hooks.json` |
| OpenCode | completion, approval, error attention | `$XDG_CONFIG_HOME/opencode/plugins/vibe-pushover.ts` or `~/.config/opencode/plugins/vibe-pushover.ts` |
| Pi | completion | `$PI_CODING_AGENT_DIR/extensions/vibe-pushover/index.ts` or `~/.pi/agent/extensions/vibe-pushover/index.ts` |
| Plandex | session exit, process failure | `vibe-pushover run --agent plandex -- plandex ...` |
| Qoder | completion | `~/.qoder/settings.json` |
| Qwen Code | completion, approval, idle attention | `~/.qwen/settings.json` |
| Rovo Dev CLI | completion, approval, error attention | `~/.rovodev/config.yml` |
| Tabnine CLI | completion, error attention (macOS/Linux) | `~/.tabnine/agent/settings.json` plus `~/.tabnine/hooks/{after-agent,on-error}.sh` |
| TRAE | completion, approval | `~/.trae/hooks.json` |
| VS Code Agent | completion | `$COPILOT_HOME/hooks/vibe-pushover.json` or `~/.copilot/hooks/vibe-pushover.json` (preview hook API) |
| Windsurf | completion | `~/.codeium/windsurf/hooks.json` |
| WorkBuddy | completion, approval, failure attention | `~/.workbuddy/settings.json` |
| ZCode | completion, approval | `~/.zcode/cli/config.json` |

The integrations follow each agent's native hook, plugin, or notification-command mechanism. They preserve existing settings, only replace entries owned by `vibe-pushover`, and repeated installs are idempotent. Copilot's attention event is limited to permission and elicitation dialogs. Droid's attention event can also mean the agent has been idle and is waiting for input. Amp reports its `awaiting-approval` state separately from turn errors. Gemini maps its official `AfterAgent` hook to completion and its `Notification` hook matched as `ToolPermission` to approval; the latter extracts the compact tool title and command from Gemini's nested details. Grok Build maps its top-level `Stop` event to completion and `StopFailure` to attention; its separate `SubagentStop` event is deliberately not installed, so child completions stay quiet. Kilo Code, MiMo Code, and OpenCode use compatible `session.idle`, `permission.asked`, and `session.error` plugin events for completion, approval, and error attention; child-session completion and error events stay quiet. Each installs into its own config tree and does not share a plugin file. Qwen sends separate approval notifications and idle-input attention notifications; re-entered active `Stop` hooks are filtered to avoid duplicate or premature completion messages. TRAE maps its top-level `Stop` hook to completion and only its `Notification` events matched as `permission_prompt` to approval; installation preserves unrelated and third-party hooks in the same global manifest. Qoder applies the same active-Stop filter. Hermes sends approval notifications only for human-facing CLI or gateway decisions and skips `approvals.mode=smart` automatic decisions. Oh My Pi ignores `agent_end` events that announce an automatic continuation and reports its native tool-approval event separately. When session logging is enabled, Mistral Vibe filters inherited subagent `post_agent_turn` events using their official `agents/<session>/messages.jsonl` layout; with logging disabled, its payload does not identify subagents, so fan-out can produce extra completion notifications. Aider, Auggie, Cline, Cursor, Gajae Code, Goose, Kiro, Mistral Vibe, OpenHands CLI, Pi, Qoder, VS Code Agent, and Windsurf currently expose completion notifications only through the installed integration.

Claude Code Router starts the official Claude Code runtime and therefore discovers the same `~/.claude/settings.json` hooks. `vibe-pushover install --agent claude-router` (or the `ccr` alias) installs that shared lifecycle configuration using Claude as the notification source; it is idempotent with `install --agent claude`, so running both never creates duplicate hooks. Detection reports the Router target only when `~/.claude-code-router` exists rather than inferring it from a normal Claude installation.

Because Aider supports only one `notifications-command`, installation refuses to replace an existing custom command. Gajae Code similarly supports one `completion.notifyCommand`; its installer also refuses to replace an unrelated command. Remove or compose the existing command yourself before retrying if you want `vibe-pushover` to own the setting. Cline supports one file per event in a hook root and likewise refuses to replace a non-`vibe-pushover` `TaskComplete` hook.

Use `--agent-config PATH` to target another agent settings file or `--binary PATH` when installing a binary that is not the currently running executable. If credentials were written with `setup --config PATH`, pass the same path to `install --config PATH`; it will be embedded in both installed hook commands.

Restart the agent after installation. In Amp, use `plugins: reload` from the command palette instead if you want to activate the generated plugin without restarting. Codex may ask you to trust the newly added local hooks before it runs them. DotCraft always starts new or changed user hooks as untrusted; open **Settings > Hooks** and trust the installed hooks before using them. Hermes asks for first-use consent for each installed `(event, command)` pair; approve it interactively or manage it with `hermes hooks`, rather than enabling `hooks_auto_accept` globally. Junie hooks currently require the Junie CLI Early Access build. Cortex Code and VS Code Agent hook support are currently preview features. Mistral Vibe installation also enables its experimental hook gate in the sibling `config.toml`; Vibe currently exposes a reliable completion hook but no hook that proves a tool is actually waiting for manual approval. Copilot CLI and VS Code Agent discover the same compatible manifest under `~/.copilot/hooks`, so installing either integration configures one shared completion hook; the notification source is detected from each completion-hook payload instead of registering duplicates. Copilot CLI additionally exposes its own asynchronous notification hook for real permission prompts and elicitation dialogs; VS Code Agent does not fire that CLI-only event. Oh My Pi discovers the generated extension automatically unless extension discovery is disabled; named OMP profiles have separate agent directories and should be installed with `--agent-config` for that profile. Auggie's Unix wrapper filters interrupted, error, maximum-iteration, and malformed stops, so only its normal `end_turn` cause sends a completion notification. Windsurf notifications extract the final content line from the Cascade response instead of forwarding the full response payload.

Kimi loads the new TOML hooks when a new session starts; its `Stop` event does not include the final assistant message, so the completion notification uses the compact fallback body `Turn completed.`. Kimi exposes `Stop` just before the turn ends and runs hooks in parallel. If another Kimi `Stop` hook blocks the turn, the notification may arrive before that continuation finishes because Kimi does not expose a later turn-ended hook.

Junie CLI uses its official [`Stop`, `StopFailure`, and `PermissionRequest` hooks](https://junie.jetbrains.com/docs/junie-cli-hooks.html). Completion and approval commands are installed as `async` observational hooks: this keeps notification delivery off the task's critical path and, critically, prevents Junie's `PermissionRequest` hook semantics from auto-approving the sensitive action. Re-entered `Stop` calls after another hook blocks completion are filtered. `StopFailure` reports classified model/API failures such as rate limits or authentication errors, including a compact failure detail. The hooks currently require Junie's Early Access build and are not invoked by Junie's ACP or server hosts.

Craft Agents uses its official per-workspace [`automations.json`](https://agents.craft.do/docs/automations/overview) command actions. Installation discovers every existing workspace, respects `CRAFT_CONFIG_DIR`, and merges `Stop` completion plus `Notification` matchers for `permission_prompt` and `idle_prompt`; it does not install a `PermissionRequest` action that could participate in the approval decision. Each fixed notification command runs in the documented `allow-all` automation mode so Craft does not stop to approve its own notifier. Existing automations and workspace settings are preserved, symlinked automation files remain symlinks, changes load without a restart, and a later workspace can be configured by rerunning `install --agent craft` or targeting its file with `--agent-config`.

Autohand Code uses its official [`on_agent_response`, `on_permission_request`, and `on_error` hooks](https://docs.autohand.ai/working-with-autohand-code/hooks) for completion, approval, and error attention. The installed actions are asynchronous and use the hook's project working directory without interpolating response, command, or error text into a shell command. This keeps the integration observational and avoids template-driven shell injection; Autohand notifications therefore use compact generic bodies while still showing the project under `summary` and `minimal`. Installation respects `AUTOHAND_CONFIG`, preserves unrelated settings and third-party hooks, and updates symlink targets without replacing the symlink.

GitHub Copilot CLI uses the official [`agentStop` and asynchronous `notification` hooks](https://docs.github.com/en/copilot/reference/hooks-reference). A `permission_prompt` becomes a high-priority approval notification, while an `elicitation_dialog` remains an attention notification for questions that need a human response. The notifier returns no decision output, so Copilot's normal permission service remains authoritative. Reinstalling migrates the older combined notification matcher while preserving unrelated personal hooks.

Antigravity CLI installs a native plugin with its documented [`Stop` hook](https://antigravity.google/docs/hooks). A `model_stop` is reported as completion only after `fullyIdle` becomes true; stops with background work are ignored, while `error` and `max_steps_exceeded` become attention notifications. The current hook surface has no approval event. CodeBuddy Code uses its documented [beta hooks](https://www.codebuddy.ai/docs/cli/hooks): `Stop`, `StopFailure`, and `PermissionRequest` become completion, attention, and approval notifications respectively. Re-entered active `Stop` hooks are filtered, but CodeBuddy runs Stop hooks in parallel and exposes no later finalized-stop event; if another Stop hook rejects stopping, the first completion notification can arrive before that continuation finishes. WorkBuddy uses the same lifecycle hook runtime with the independent `.workbuddy` configuration home introduced in its [v2.48.0 release](https://www.workbuddy.ai/docs/cli/release-notes/v2.48.0), so it receives the same three notification types without modifying CodeBuddy settings. CodeWhale, the project formerly named DeepSeek-TUI, uses its native `turn_end` and `on_error` hooks from [`~/.codewhale/config.toml`](https://github.com/Hmbown/CodeWhale); only `turn_end` payloads with `status = "completed"` become completion notifications, while errors are reported by `on_error`. Its current config resolution remains compatible with legacy `~/.deepseek/config.toml`, and it has no separate configurable approval event. An explicit `[hooks] enabled = false` remains respected; set it to `true` to activate installed CodeWhale hooks. These installers retain unrelated hooks and only update entries they can identify as owned by `vibe-pushover`.

ZCode exposes `Stop` and `PermissionRequest` through its native CLI hook configuration and also supports hook-bearing [plugins](https://zcode.z.ai/en/docs/plugin). Installation merges the two notification commands into `hooks.events`, preserves provider settings, existing hook groups, timeouts, and output limits, and enables hooks only when the `enabled` field was absent. The completion body prefers ZCode's compact `responsePreview`; an explicit `hooks.enabled = false` remains respected. ZCode runs `Stop` hooks before deciding whether another hook will continue the turn, and its payload has no later finalized signal, so a completion notification can arrive early when another `Stop` hook requests continuation.

Kilo Code installs a current module-descriptor plugin in its documented [global plugin directory](https://kilo.ai/docs/automate/extending/plugins). Its own desktop attention alerts can remain disabled when Pushover is the preferred phone/watch path. DotCraft installs user-level lifecycle hooks in the documented [`~/.craft/hooks.json`](https://www.dotcraft.net/developing/configuration), preserving unrelated hook groups. The install command reminds you to trust them in **Settings > Hooks**. Its `StopFailure` event reports a failure while processing Stop hooks, not a model-turn failure. DotCraft `Stop` supports rewake feedback, so a completion can arrive before a later continuation requested by another Stop hook; re-entered active Stop calls are filtered.

OpenHands CLI loads the installed global hook from `~/.openhands/hooks.json` for terminal, headless, and ACP sessions. A repository-level `.openhands/hooks.json` takes precedence over the global file instead of merging with it; install that project file explicitly with `--agent-config .openhands/hooks.json` when a repository already has OpenHands hooks. OpenHands exposes a stable `Stop` hook but no hook that proves a tool is waiting for manual approval, so this integration reports completion only. A different `Stop` hook can deny stopping and make the agent continue after the notification because OpenHands does not expose a later per-turn completion event.

Rovo Dev CLI uses its native [`on_complete`, `on_error`, and `on_tool_permission` event hooks](https://www.atlassian.com/blog/development/streamline-rovo-dev-cli-with-event-hooks). Installation merges commands into the existing YAML configuration, preserves third-party commands and comments, and does not read from the agent's inherited terminal input. Restart Rovo Dev after installation.

Tabnine CLI uses its native [`after-agent` and `on-error` executable hooks](https://docs.tabnine.com/main/getting-started/tabnine-cli/features/hooks). Installation enables hooks in the matching global or project `settings.json` and creates the two scripts under that same `.tabnine/hooks` root. The current documented hook surface has no event proving a tool is waiting for approval, and the documented executable examples are POSIX shell scripts, so this integration reports completion and error attention on macOS/Linux only. Existing non-`vibe-pushover` scripts are never overwritten.

Pi deliberately has no built-in permission popups, so its integration currently sends turn-complete notifications only. `vibe-pushover` does not add a confirmation policy or turn every Pi tool call into a manual approval.

Cline installs its stable `TaskComplete` hook into the operating system's real Documents directory. At the standard `~/Documents` location that one hook is discovered by both the IDE and current CLI/SDK runtime, avoiding duplicate execution. If Windows My Documents or Linux XDG Documents is redirected elsewhere, installation also writes the CLI copy under `~/.cline/hooks`; both paths are ownership-checked before either is changed. It extracts the compact result from either the IDE's `taskComplete.taskMetadata.result` or the newer CLI/SDK `turn.outputText` payload, and ignores child-agent completions identified by `parent_agent_id`. Cline exposes pre-tool hooks but no dedicated event proving that a tool is blocked waiting for manual approval, so `vibe-pushover` deliberately reports completion only instead of producing false approval alerts. Use `--agent-config PATH` with the full `TaskComplete` hook file path when intentionally targeting a custom Cline hooks directory.

Gajae Code uses its official `completion.notifyCommand` setting. The installed command reads only the bounded JSON payload supplied in `GJC_NOTIFICATION_JSON`, preserves unrelated YAML settings and comments, and runs for normal completed turns; Gajae excludes aborted and error stops from this notification surface. The setting does not provide an approval event, so this integration reports completion only and does not install a long-running SDK client.

Roo Code, Zed Agent, and GitLab Duo CLI have been audited but are not listed as supported because they do not currently expose a stable, installed user-level turn-complete or approval hook suitable for this integration. Roo Code emits task events through its programmatic extension/IPC API, but enabling that API requires process-level setup instead of a normal user hook configuration. Zed's current task hook is limited to worktree creation, and GitLab Duo's external hook experiment currently exposes only `SessionStart` even though Duo has built-in system notifications. Warp already provides its own desktop notifications for completed agents and agents that need attention, but does not expose a corresponding external lifecycle hook. `vibe-pushover` intentionally avoids log polling for these tools because it would be fragile and could leak conversation content.

## Run agents without lifecycle hooks

For a terminal agent without a stable installable hook, place its command behind `run`:

```sh
vibe-pushover run --agent continue -- cn -p "implement the next task"
vibe-pushover run --agent plandex -- plandex
vibe-pushover run --agent crush -- crush
vibe-pushover run --agent my-agent --after 2m -- my-agent --its-own-flags
```

The wrapper attaches the original stdin, stdout, and stderr directly and preserves the child process exit status, including conventional `128+signal` status on macOS/Linux. Successful sessions shorter than 30 seconds stay quiet by default; change the threshold with `--after`, including `--after 0s` to notify every success. A failure always bypasses that duration threshold and uses the attention style, even when it happens immediately; global snooze and matching silence rules still apply. Notification transport failures are warnings and never change the agent's exit status.

This fallback observes process exit, not individual turns or approval dialogs inside a long-running TUI. Use an installed native integration when one is available. Continue's documented headless `cn -p` mode is a particularly good fit. Crush already has foreground-aware local completion and permission notifications; the wrapper adds a Pushover session-exit path without scraping its logs or conversation content.

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

Preview automatically uses the default config from `setup` when it exists, including its profile, message-detail mode, event-specific sound, and every active delivery filter. It reports matching silence rules, snooze deadlines, focus deadlines, and quiet-hour windows instead of displaying a notification that would not be delivered. It still works without credentials when no config exists. Pass `--config PATH` for a custom config; an explicit `--profile` still takes precedence.

## Notification format, profiles, and icon

Notifications are intentionally compact for phones and watches:

- A completed turn uses a title such as `✓ Codex finished · vibe-pushover` and the first useful line of the agent's final message, skipping Markdown headings when a result line follows and truncating it to 180 Unicode characters. It is silent and expires after one hour.
- An approval request uses a title such as `⚠ Codex needs approval · vibe-pushover` and shows only the tool plus its command or reason, truncated to 300 Unicode characters. It is high priority, uses Pushover's `persistent` sound, and expires after 30 minutes.
- An attention notification uses a title such as `⚠ Droid needs attention · vibe-pushover`, carries a compact reason, and uses the same high-priority delivery as an approval request.
- If a hook payload supplies an HTTP(S) `url`, `session_url`, `web_url`, or `details_url`, the notification includes Pushover's supplementary `Open result` or `Open agent` action. Unsafe and local-only URL schemes are ignored.
- If a hook payload supplies a Unix or RFC3339 event timestamp, it is forwarded through Pushover's [`timestamp` parameter](https://pushover.net/api#timestamp) so concurrently delivered or delayed notifications are ordered by when the agent event actually happened, rather than when the API received it. Unix values may use seconds, milliseconds, microseconds, or nanoseconds, either as JSON numbers or digit strings; implausible values are omitted.
- By default Pushover delivers to every active device on the account. `setup` or `vibe-pushover device ...` can restrict delivery to one or more named devices without changing the agent hooks.
- `detail minimal` hides hook-provided summaries, commands, and error reasons from the body while keeping the agent/event/project title and any safe supplementary action. `detail private` additionally hides the project and removes supplementary action URLs. `detail summary` restores the compact contextual body.

Hook delivery also suppresses exact repeats for three seconds across CLI processes. The fingerprint includes the agent, event, a non-secret hash of the Pushover destination, rendered notification fields, and stable session/turn/tool IDs when available. A failed Pushover request releases its reservation so a later hook can retry; an unavailable or corrupt dedupe cache fails open and sends the notification with a warning. The private cache is stored at `~/Library/Caches/vibe-pushover/dedupe.json` on macOS or `$XDG_CACHE_HOME/vibe-pushover/dedupe.json` on Linux.

`snooze` temporarily suppresses all hook delivery. `focus` is the safer interruption-control mode: it suppresses only completion notifications while approval and attention alerts continue. `quiet-hours` applies that blocker-only behavior on a recurring local-time schedule, including windows that cross midnight. `silence` adds exact per-agent or per-project rules; its safe default suppresses completions only, while `--event all` can suppress blockers too. Temporary modes store only their expiry timestamp beside the existing local configuration and resume automatically after the deadline; `vibe-pushover test --force` remains available for an intentional end-to-end check.

Profiles control how noticeable those messages are:

| Profile | Completion | Approval and attention |
| --- | --- | --- |
| `balanced` | silent, low priority | persistent sound, high priority |
| `quiet` | silent, normal priority | silent, normal priority |
| `urgent` | suppressed | persistent sound, high priority |
| `watch` | default Pushover sound, normal priority | persistent sound, high priority |
| `on-call` | suppressed | persistent sound, emergency priority; retries every 60 seconds for up to 15 minutes or until acknowledged |

Event-specific choices from `vibe-pushover sound` replace the sound cells above while preserving each profile's delivery and priority behavior. `default` omits the API sound override so the Pushover account/device preference is used; `reset` restores the table preset. Custom sound names work after they are uploaded for the application token's owning account. `quiet` remains silent regardless of an event-specific setting.

Use the permanent `urgent` profile when completion messages should always stay off. Use the explicit `on-call` profile only when approval and attention events must repeatedly alert until acknowledged; it uses Pushover's documented [Emergency Priority](https://pushover.net/api#priority) and clears TTL because Pushover ignores TTL for emergency messages. Event-specific sound choices still apply without changing the retry schedule. Interactive setup and `profile on-call` both print the retry warning when enabling it, while `status` keeps the active schedule visible afterward. Use temporary `focus 2h` when you only need a blocker-only window for the current work session. `preview --profile on-call --event approval-required` shows the retry and expiry schedule before you enable it.

The notification icon is attached to the Pushover Application identified by the configured app token; it cannot be selected per message. To customize it, sign in to the [Pushover dashboard](https://pushover.net/), open the application whose API token you configured, and upload its icon. Pushover remains the host application, so operating-system surfaces may still show Pushover branding alongside the per-application icon.

## Development

```sh
make test
make vet
make build
```

Tests use an in-memory HTTP transport and never call the real Pushover API.
