---
name: codekanban-cli
description: Operate CodeKanban workflows, terminal sessions, and web sessions through the installable `codekanban-cli` command. Use when the user wants to create, inspect, control, watch, or continue CodeKanban AI work from a project path, project ID, or project name without relying on repository-local helper scripts. Prefer `web-session` for structured interactive work, and use `workflow start` or `terminal continue` only when the user explicitly wants PTY-style terminal behavior.
---

# CodeKanban CLI

Use this skill after installing the `codekanban-cli` package and copying this packaged skill into the user's Codex skills directory.

## When to use this skill

- Use it for CodeKanban `project`, `web-session`, `workflow`, `session`, and `terminal` operations.
- Prefer `web-session` for structured interactive AI work.
- Use `workflow start` only when the user explicitly wants a PTY-style startup flow.
- Use `terminal continue` only when the user explicitly wants to continue an existing terminal session.

## Prerequisites

- `codekanban-cli` is installed and available on `PATH`
- The CodeKanban service is reachable
- Default service URL is `http://127.0.0.1:3007`
- If auth is enabled, save a token once with `codekanban-cli auth save-token`

## First-time initialization

If the service is open and does not require auth, you can start immediately:

```bash
codekanban-cli project list
```

If the service requires auth, save a token once:

```bash
printf '%s' '<PASSWORD>' | codekanban-cli auth save-token --password-stdin
```

You can also save a token directly:

```bash
codekanban-cli auth save-token --token-file <TOKEN_FILE>
```

Saved session data lives in a user-level config directory, not in a repository:

- Windows: `%APPDATA%\codekanban-cli\session.json`
- macOS/Linux: `$XDG_CONFIG_HOME/codekanban-cli/session.json` or `~/.config/codekanban-cli/session.json`

## Project targeting rules

Target selection priority:

1. `--project-id`
2. `--project-name`
3. local current directory fallback for local servers only
4. explicit `--path`

Use these project helpers first when the user refers to a project by name:

```bash
codekanban-cli project list
codekanban-cli project resolve --project-name codekanban
```

Then operate on the resolved project:

```bash
codekanban-cli web-session create --project-name codekanban --agent codex --workflow-mode plan --permission-level elevated --title "Planning session"
codekanban-cli session list --project-name codekanban
```

If the user says something like "create a web session for project codekanban and pull git updates", follow this pattern:

1. Resolve the project by name
2. Create the web session for that project
3. Send the requested instruction into the new session
4. Watch or poll the session if the user expects completion status

For the common structured flow "create a session, answer user-input questions, execute the plan, and wait for done", prefer the built-in orchestration command:

```bash
codekanban-cli web-session run --project-name codekanban --agent codex --text "Create a concise plan first, then implement it." --strict-cwd
```

## Image inputs

Use repeatable `--image <path>` options when the initial task or a follow-up message needs local image references:

```bash
codekanban-cli web-session run --project-name codekanban --agent codex --text "Match the implementation to these references." --image ./desktop.png --image ./mobile.png --strict-cwd
codekanban-cli web-session send --session-id <SESSION_ID> --text "Also account for this state." --image ./reference.png
```

- `--image` is supported by `web-session run` and `web-session send`.
- Image paths are local to the machine running `codekanban-cli`, even when the CodeKanban service is remote.
- The option is repeatable and can be combined with existing `--attachment-id` values.
- Web-session attachments currently support images only and are subject to the server attachment size limit.
- Use `web-session attach --file <path>` followed by `--attachment-id <id>` only when an uploaded image must be reused across commands.

## Remote web-session test loop (minimal SDK)

Use this when you want a short programmable remote verification loop without stitching websocket events by hand.

- Prefer SDK polling helpers over websocket event streams for this flow.
- `runWebSessionUntilDone(...)` already handles:
  - ordinary structured user-input prompts
  - latest-plan execution
  - terminal-state debounce through `settleMs`
- It still stops and returns control on approvals or timeouts.
- Keep this template focused on session orchestration; do file verification separately if you need it.

```bash
node --input-type=module <<'EOF'
import { CodeKanbanClient } from '@codekanban/sdk';

const baseURL = 'http://10.128.128.111:6112';
const projectPath = '/home/dev/test1';
const initialTask = 'Use the current directory as the project root. Keep the task short, ask structured questions only if needed, and do not do unrelated work.';

const client = new CodeKanbanClient({ baseURL });

const health = await fetch(`${baseURL}/api/v1/health`);
if (!health.ok) {
  throw new Error(`health check failed: ${health.status}`);
}

const { project } = await client.resolveProject({
  path: projectPath,
  ensureProject: true,
});

const session = await client.createWebSession({
  projectId: project.id,
  agent: 'codex',
  workflowMode: 'plan',
  permissionLevel: 'elevated',
  reasoningEffort: 'low',
  title: 'remote test loop',
});

await client.sendWebSessionMessage({
  sessionId: session.id,
  text: initialTask,
});

const result = await client.runWebSessionUntilDone({
  projectId: project.id,
  sessionId: session.id,
  intervalMs: 1500,
  timeoutMs: 120000,
  settleMs: 2000,
});

console.log(JSON.stringify({
  projectId: project.id,
  sessionId: session.id,
  stopReason: result.stopReason,
  finalPhase: result.finalState?.phase ?? null,
  lastAssistant: result.finalState?.lastAssistantMessage?.text ?? null,
}, null, 2));
EOF
```

## Remote server rules

When the service is remote, do not assume the local Codex machine path matches the server path.

- Prefer `--project-name` or `--project-id`
- Use `--path` only when you know the path is valid on the machine running CodeKanban
- If project-name matching is ambiguous, use `project resolve --project-name <name>` first; if it lists multiple candidates, retry with `--project-index <n>`

Read `references/project-targeting.md` when you need a decision guide for local vs remote targeting or ambiguous project names.

## Base URL rules

Base URL resolution order:

1. `--base-url`
2. `CODEKANBAN_BASE_URL` or `BASE_URL`
3. saved session file
4. `http://127.0.0.1:3007`

If the service is not running on the default address:

```bash
codekanban-cli --base-url http://192.168.1.50:3007 project list
```

## Common commands

```bash
codekanban-cli project list
codekanban-cli project resolve --project-name codekanban --project-index 2
codekanban-cli web-session state --project-name codekanban --session-id <SESSION_ID>
codekanban-cli web-session answer-pending --project-name codekanban --session-id <SESSION_ID>
codekanban-cli web-session execute-plan --project-name codekanban --session-id <SESSION_ID>
codekanban-cli web-session wait --project-name codekanban --session-id <SESSION_ID> --until done --settle-ms 2000
codekanban-cli web-session send --session-id <SESSION_ID> --text "Review this reference." --image ./reference.png
codekanban-cli file read --project-name codekanban --file notes/123.md
codekanban-cli web-session run --project-name codekanban --agent codex --text "Create a concise plan first, then implement it." --image ./reference.png --strict-cwd
codekanban-cli workflow start --project-name codekanban --profile plan --prompt "Inspect the repository and respond with a plan"
codekanban-cli terminal continue --session-id <TERMINAL_SESSION_ID> --prompt "Continue from the last step"
```

## Natural-language mapping

- If the user names a project, resolve by project name first.
- If the user asks to "open/start/create a CodeKanban session", default to `web-session create` unless they explicitly want PTY-style terminal behavior.
- If the user asks to continue or reply in an existing web session, use `web-session send`.
- If the user provides local image references, pass each one with `--image` on `web-session run` or `web-session send`.
- If the user asks for progress or completion state, use `web-session state`, `web-session wait`, or `web-session watch`.
- If the user asks for the whole structured plan / answer / execute / wait flow, use `web-session run`.
- If the user wants a programmable remote loop instead of a pure CLI sequence, use the minimal SDK template with `runWebSessionUntilDone(...)`.
- If the user asks to verify or clear a generated file, use `file read` or `file delete`.
- If the user says to stay inside the current directory only, add `--strict-cwd`.
- If the user asks for a terminal-style launch, use `workflow start`.

## Important note

This is the only shipped CodeKanban Codex skill.
After installing this skill, restart Codex so it can discover the new skill directory.
