import { CodeKanbanClient, buildAgentLaunchSpec } from '@codekanban/sdk';

const REMOVED_COMMAND_SCOPES = new Set(['board', 'boards', 'kanban', 'task', 'tasks']);

export function assertCommandScope(scope) {
  const normalizedScope = String(scope || '').trim().toLowerCase();
  if (REMOVED_COMMAND_SCOPES.has(normalizedScope)) {
    throw new Error(`command scope "${scope}" was removed with the Kanban task API`);
  }
}

function createJsonOutput(value) {
  return `${JSON.stringify(value, null, 2)}\n`;
}

function readFlagValue(argv, index, flag) {
  const value = argv[index + 1];
  if (value == null || value.startsWith('--')) {
    throw new Error(`${flag} requires a value`);
  }
  return value;
}

function readRawFlagValue(argv, index, flag) {
  const value = argv[index + 1];
  if (value == null) {
    throw new Error(`${flag} requires a value`);
  }
  return value;
}

function parseIntegerFlag(value, fieldName) {
  if (value == null || value === '') {
    return undefined;
  }
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) {
    throw new Error(`${fieldName} must be a number`);
  }
  return Math.trunc(parsed);
}

function parseJsonFlag(value, fieldName) {
  if (!value) {
    throw new Error(`${fieldName} is required`);
  }
  try {
    return JSON.parse(value);
  } catch (error) {
    throw new Error(`${fieldName} must be valid JSON`);
  }
}

function parseCliArgs(argv) {
  const positionals = [];
  const flags = {
    addDirs: [],
    attachmentIds: [],
    imagePaths: [],
    deleteFilesBefore: [],
    extraArgs: [],
    readFilesAfter: [],
    includeTerminal: true,
    includeAI: true,
    refresh: false,
    clearExisting: false,
    raw: false,
    strictCwd: false,
    ifExists: false,
  };

  for (let index = 0; index < argv.length; index += 1) {
    const token = argv[index];
    if (!token.startsWith('--')) {
      positionals.push(token);
      continue;
    }

    switch (token) {
      case '--base-url':
        flags.baseURL = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--project-id':
        flags.projectId = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--path':
        flags.path = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--prompt':
        flags.prompt = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--text':
        flags.text = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--agent':
        flags.agent = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--claude-runtime':
        flags.claudeRuntime = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--model':
        flags.model = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--profile':
        flags.profile = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--sandbox':
        flags.sandbox = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--approval-policy':
        flags.approvalPolicy = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--add-dir':
        flags.addDirs.push(readFlagValue(argv, index, token));
        index += 1;
        break;
      case '--attachment-id':
        flags.attachmentIds.push(readFlagValue(argv, index, token));
        index += 1;
        break;
      case '--image':
        flags.imagePaths.push(readFlagValue(argv, index, token));
        index += 1;
        break;
      case '--extra-arg':
        flags.extraArgs.push(readRawFlagValue(argv, index, token));
        index += 1;
        break;
      case '--title':
        flags.title = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--working-dir':
        flags.workingDir = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--worktree-id':
        flags.worktreeId = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--session-id':
        flags.sessionId = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--id':
        flags.id = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--tool-use-id':
        flags.toolUseId = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--reasoning-effort':
        flags.reasoningEffort = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--workflow-mode':
        flags.workflowMode = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--permission-level':
        flags.permissionLevel = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--permission-mode':
        flags.permissionMode = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--limit':
        flags.limit = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--before-cursor':
        flags.beforeCursor = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--mode':
        flags.mode = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--group-id':
        flags.groupId = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--scope-id':
        flags.scopeId = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--file':
        flags.file = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--item-id':
        flags.itemId = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--answers-json':
        flags.answersJson = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--answer-strategy':
        flags.answerStrategy = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--prev-session-id':
        flags.prevSessionId = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--next-session-id':
        flags.nextSessionId = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--idle-timeout-ms':
        flags.idleTimeoutMs = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--max-events':
        flags.maxEvents = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--interval-ms':
        flags.intervalMs = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--settle-ms':
        flags.settleMs = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--timeout-ms':
        flags.timeoutMs = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--until':
        flags.until = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--delete-file-before':
        flags.deleteFilesBefore.push(readFlagValue(argv, index, token));
        index += 1;
        break;
      case '--read-file-after':
        flags.readFilesAfter.push(readFlagValue(argv, index, token));
        index += 1;
        break;
      case '--refresh':
        flags.refresh = true;
        break;
      case '--clear-existing':
        flags.clearExisting = true;
        break;
      case '--raw':
        flags.raw = true;
        break;
      case '--strict-cwd':
        flags.strictCwd = true;
        break;
      case '--if-exists':
        flags.ifExists = true;
        break;
      case '--no-terminal':
        flags.includeTerminal = false;
        break;
      case '--no-ai':
        flags.includeAI = false;
        break;
      default:
        throw new Error(`unknown flag: ${token}`);
    }
  }

  return {
    positionals,
    flags,
  };
}

export function createHelpText(commandName = 'codekanban-cli') {
  return `${commandName} - CodeKanban command runtime

Usage:
  ${commandName} <scope> <action> [options]

Scopes:
  workflow     start, command
  session      list, conversation, tool-result
  terminal     continue
  file         scopes, read, delete
  web-session  list, create, connect, snapshot, history, sync,
               state, answer-pending, execute-plan, wait, run,
               archived, archive, unarchive, rename, close, delete,
               runtime-config, command-group, attach, send, approve,
               reject, user-input, set-model, set-reasoning,
               set-workflow, set-permission, set-agent, move, watch

Common options:
  --base-url <url>      CodeKanban server base URL
  --project-id <id>     Project identifier
  --path <path>         Local project path
  --session-id <id>     Session identifier
  --image <path>        Upload a local image for web-session send/run; repeatable
  --claude-runtime <rt> Claude launcher for workflow terminal mode: claude or ccr
  --help                Show this help text

Examples:
  ${commandName} session list --base-url http://127.0.0.1:3007 --path D:/repo
  ${commandName} web-session state --base-url http://127.0.0.1:3007 --path D:/repo --session-id <id>
  ${commandName} web-session send --session-id <id> --text "Review this reference" --image ./reference.png
  ${commandName} web-session run --base-url http://127.0.0.1:3007 --path D:/repo --agent codex --text "Create notes/123.md" --delete-file-before notes/123.md --read-file-after notes/123.md --strict-cwd
`;

}

function buildPermissions(flags) {
  const permissions = {};
  if (flags.sandbox) {
    permissions.sandbox = flags.sandbox;
  }
  if (flags.approvalPolicy) {
    permissions.approvalPolicy = flags.approvalPolicy;
  }
  if (flags.addDirs.length > 0) {
    permissions.addDirs = flags.addDirs;
  }
  return Object.keys(permissions).length > 0 ? permissions : undefined;
}

function sanitizeConnection(value) {
  if (Array.isArray(value)) {
    return value.map(item => sanitizeConnection(item));
  }
  if (!value || typeof value !== 'object') {
    return value;
  }
  const next = { ...value };
  delete next.connection;
  delete next.commandChannel;
  delete next.eventStream;
  delete next.socket;
  return next;
}

function printJson(stream, payload) {
  stream.write(createJsonOutput(payload));
}

function writeJsonLine(stream, payload) {
  stream.write(`${JSON.stringify(payload)}\n`);
}

async function withWebSessionCommandChannel(client, handler) {
  const channel = client.openWebSessionCommandChannel();
  try {
    await channel.waitForOpen();
    return await handler(channel);
  } finally {
    channel.close();
  }
}

async function watchWebSession(client, flags, stdout) {
  const eventStream = client.openWebSessionEventStream({
    sessionId: flags.sessionId,
  });
  const maxEvents = parseIntegerFlag(flags.maxEvents, 'maxEvents');
  const idleTimeoutMs = parseIntegerFlag(flags.idleTimeoutMs, 'idleTimeoutMs');
  let eventCount = 0;
  let idleTimer = null;
  let closedByPolicy = false;

  const armIdleTimer = () => {
    if (idleTimer) {
      clearTimeout(idleTimer);
      idleTimer = null;
    }
    if (!Number.isFinite(idleTimeoutMs) || idleTimeoutMs == null || idleTimeoutMs <= 0) {
      return;
    }
    idleTimer = setTimeout(() => {
      closedByPolicy = true;
      eventStream.close();
    }, idleTimeoutMs);
  };

  try {
    await eventStream.waitForOpen();
    armIdleTimer();

    for await (const event of eventStream) {
      if (event.type === 'open' || event.type === 'close') {
        continue;
      }

      if (event.type === 'error' && event.errorType !== 'frame') {
        throw event.error instanceof Error ? event.error : new Error(event.message || 'event stream failed');
      }

      const payload = flags.raw && event.raw ? event.raw : event;
      writeJsonLine(stdout, payload);
      eventCount += 1;
      armIdleTimer();

      if (maxEvents != null && maxEvents > 0 && eventCount >= maxEvents) {
        closedByPolicy = true;
        eventStream.close();
      }
    }

    return 0;
  } finally {
    if (idleTimer) {
      clearTimeout(idleTimer);
    }
    if (!closedByPolicy) {
      eventStream.close();
    }
  }
}

const STRICT_CWD_INSTRUCTION =
  'Stay strictly inside the current working directory. Do not search sibling directories, parent directories, or nearby repositories unless the user explicitly asks.';

function normalizeChoiceLabel(value) {
  return String(value || '').trim();
}

function buildAutoUserInputAnswers(questions = [], strategy = 'prefer-second-or-text') {
  const normalizedStrategy = String(strategy || 'prefer-second-or-text').trim().toLowerCase();
  if (!['prefer-second-or-text', 'prefer-second-or-first'].includes(normalizedStrategy)) {
    throw new Error(`unsupported answer strategy: ${strategy}`);
  }

  const answers = {};
  for (const question of Array.isArray(questions) ? questions : []) {
    const questionId = normalizeChoiceLabel(question?.id);
    if (!questionId) {
      continue;
    }
    const options = Array.isArray(question?.options)
      ? question.options.map(option => normalizeChoiceLabel(option?.label)).filter(Boolean)
      : [];
    if (options[1]) {
      answers[questionId] = [options[1]];
      continue;
    }
    if (normalizedStrategy === 'prefer-second-or-first' && options[0]) {
      answers[questionId] = [options[0]];
      continue;
    }
    answers[questionId] = [question?.isSecret ? 'redacted' : 'continue'];
  }
  return answers;
}

function normalizeUntil(value) {
  const raw = String(value || '').trim();
  if (!raw) {
    return 'done';
  }
  const phases = raw
    .split(',')
    .map(entry => entry.trim())
    .filter(Boolean);
  if (phases.length === 0) {
    return 'done';
  }
  return phases.length === 1 ? phases[0] : phases;
}

function withStrictCwdPrompt(text, strictCwd) {
  const body = String(text || '').trim();
  if (!strictCwd) {
    return body;
  }
  return body
    ? `${STRICT_CWD_INSTRUCTION}

${body}`
    : STRICT_CWD_INSTRUCTION;
}

async function answerPendingWithStrategy(client, flags) {
  const answers = flags.answersJson
    ? parseJsonFlag(flags.answersJson, 'answersJson')
    : buildAutoUserInputAnswers([], flags.answerStrategy);
  const state = await client.getWebSessionState({
    projectId: flags.projectId,
    path: flags.path,
    sessionId: flags.sessionId,
    limit: parseIntegerFlag(flags.limit, 'limit'),
  });
  if (!state.pendingUserInput) {
    throw new Error(`web session ${flags.sessionId} has no pending user input`);
  }
  const resolvedAnswers = flags.answersJson
    ? answers
    : buildAutoUserInputAnswers(
        state.pendingUserInput.questions,
        flags.answerStrategy,
      );
  return await client.answerPendingUserInput({
    projectId: flags.projectId,
    path: flags.path,
    sessionId: flags.sessionId,
    answers: resolvedAnswers,
    limit: parseIntegerFlag(flags.limit, 'limit'),
  });
}

async function maybeDeleteFilesBefore(client, flags, sessionProjectId) {
  const deleted = [];
  for (const filePath of flags.deleteFilesBefore) {
    const result = await client.deleteProjectFiles({
      projectId: sessionProjectId || flags.projectId,
      path: sessionProjectId ? undefined : flags.path,
      scopeId: flags.scopeId,
      paths: [filePath],
    });
    deleted.push({ path: filePath, result });
  }
  return deleted;
}

async function readFilesAfter(client, flags, sessionProjectId) {
  const files = [];
  for (const filePath of flags.readFilesAfter) {
    const item = await client.readProjectFileText({
      projectId: sessionProjectId || flags.projectId,
      path: sessionProjectId ? undefined : flags.path,
      scopeId: flags.scopeId,
      filePath,
    });
    files.push(item);
  }
  return files;
}

async function uploadWebSessionImages(client, imagePaths, target = {}) {
  const attachments = [];
  for (const filePath of imagePaths) {
    const attachment = await client.uploadWebSessionAttachment({
      projectId: target.projectId,
      path: target.path,
      filePath,
    });
    if (!attachment?.id) {
      throw new Error(`image upload did not return an attachment id: ${filePath}`);
    }
    attachments.push(attachment);
  }
  return attachments;
}

function mergeAttachmentIds(attachmentIds, uploadedAttachments) {
  return [
    ...attachmentIds,
    ...uploadedAttachments.map(attachment => attachment.id),
  ];
}

async function runWebSessionFlow(client, flags) {
  const intervalMs = parseIntegerFlag(flags.intervalMs, 'intervalMs') || 2000;
  const timeoutMs = parseIntegerFlag(flags.timeoutMs, 'timeoutMs') || 120000;
  const settleMs = parseIntegerFlag(flags.settleMs, 'settleMs') || 2000;

  let session = null;
  let sessionId = flags.sessionId;
  let sessionProjectId = flags.projectId;
  const initialPrompt = withStrictCwdPrompt(flags.text || flags.prompt, flags.strictCwd);
  const uploadedAttachments = await uploadWebSessionImages(client, flags.imagePaths, {
    projectId: flags.projectId,
    path: flags.path,
  });
  const attachmentIds = mergeAttachmentIds(flags.attachmentIds, uploadedAttachments);

  if (!sessionId) {
    if (!initialPrompt && attachmentIds.length === 0) {
      throw new Error('web-session run requires --session-id or an initial --text/--prompt/--image/--attachment-id');
    }
    session = await client.createWebSession({
      projectId: flags.projectId,
      path: flags.path,
      worktreeId: flags.worktreeId,
      agent: flags.agent || 'codex',
      model: flags.model,
      reasoningEffort: flags.reasoningEffort,
      workflowMode: flags.workflowMode || 'plan',
      permissionLevel: flags.permissionLevel,
      permissionMode: flags.permissionMode,
      title: flags.title,
    });
    sessionId = session?.id;
    sessionProjectId = session?.projectId || sessionProjectId;
  }

  if (!sessionId) {
    throw new Error('unable to resolve a web session id');
  }

  const deletedBefore = await maybeDeleteFilesBefore(client, flags, sessionProjectId);

  if (initialPrompt || attachmentIds.length > 0) {
    await client.sendWebSessionMessage({
      sessionId,
      text: initialPrompt,
      attachmentIds,
      mode: flags.mode,
    });
  }

  const flow = await client.runWebSessionUntilDone({
    projectId: sessionProjectId || flags.projectId,
    path: sessionProjectId ? undefined : flags.path,
    sessionId,
    until: normalizeUntil(flags.until),
    intervalMs,
    timeoutMs,
    limit: parseIntegerFlag(flags.limit, 'limit'),
    settleMs,
    answerStrategy: flags.answerStrategy || 'prefer-second-or-text',
    autoExecutePlan: true,
    executePlanPrompt: withStrictCwdPrompt('Implement the plan.', flags.strictCwd),
  });

  if (flow.stopReason === 'needs_approval') {
    throw new Error('web-session run stopped on a pending approval; use web-session approve or reject explicitly');
  }
  if (flow.stopReason === 'needs_user_input') {
    throw new Error('web-session run stopped on a pending user input; use web-session answer-pending explicitly');
  }
  if (flow.stopReason === 'needs_execute_plan') {
    throw new Error('web-session run stopped before executing the latest plan; use web-session execute-plan explicitly');
  }
  if (flow.stopReason === 'timeout') {
    throw new Error(`web-session run timed out after ${timeoutMs}ms`);
  }

  const filesAfter = await readFilesAfter(client, flags, sessionProjectId);
  return {
    session: session || { id: sessionId, projectId: sessionProjectId || flags.projectId },
    deletedBefore,
    actions: flow.actions,
    finalState: flow.finalState,
    filesAfter,
  };
}


export async function runCli(argv, options = {}) {
  const stdout = options.stdout || process.stdout;
  const stderr = options.stderr || process.stderr;
  const commandName = options.commandName || 'codekanban-cli';

  try {
    if (argv.includes('--help') || argv.includes('-h')) {
      stdout.write(createHelpText(commandName));
      return 0;
    }

    const { positionals, flags } = parseCliArgs(argv);
    const [scope, action] = positionals;
    if (!scope || !action) {
      stdout.write(createHelpText(commandName));
      return 0;
    }
    assertCommandScope(scope);

    if (scope === 'workflow' && action === 'command') {
      const result = buildAgentLaunchSpec({
        agent: flags.agent,
        claudeRuntime: flags.claudeRuntime,
        profile: flags.profile,
        permissions: buildPermissions(flags),
        extraArgs: flags.extraArgs,
        prompt: flags.prompt || 'Inspect the project and respond.',
      });
      printJson(stdout, sanitizeConnection(result));
      return 0;
    }

    if (!flags.baseURL && options.defaultBaseURL) {
      flags.baseURL = options.defaultBaseURL;
    }

    if (!flags.baseURL) {
      throw new Error('--base-url is required');
    }

    const client =
      options.clientFactory?.({ baseURL: flags.baseURL, flags }) ||
      new CodeKanbanClient({
        baseURL: flags.baseURL,
        ...(options.clientOptions || {}),
      });
    let result;

    if (scope === 'workflow' && action === 'start') {
      result = await client.startWorkflow({
        projectId: flags.projectId,
        path: flags.path,
        worktreeId: flags.worktreeId,
        prompt: flags.prompt,
        agent: flags.agent,
        claudeRuntime: flags.claudeRuntime,
        profile: flags.profile,
        permissions: buildPermissions(flags),
        extraArgs: flags.extraArgs,
        title: flags.title,
        workingDir: flags.workingDir,
      });
    } else if (scope === 'session' && action === 'list') {
      result = await client.listSessions({
        projectId: flags.projectId,
        path: flags.path,
        includeTerminal: flags.includeTerminal,
        includeAI: flags.includeAI,
      });
    } else if (scope === 'session' && action === 'conversation') {
      result = await client.getAISessionConversation({
        id: flags.id,
        sessionId: flags.sessionId,
        refresh: flags.refresh,
      });
    } else if (scope === 'session' && action === 'tool-result') {
      result = await client.getAISessionToolResult({
        id: flags.id,
        sessionId: flags.sessionId,
        toolUseId: flags.toolUseId,
      });
    } else if (scope === 'terminal' && action === 'continue') {
      result = await client.continueTerminalSession({
        projectId: flags.projectId,
        path: flags.path,
        sessionId: flags.sessionId,
        prompt: flags.prompt,
      });
    } else if (scope === 'file' && action === 'scopes') {
      result = await client.listProjectFileScopes({
        projectId: flags.projectId,
        path: flags.path,
      });
    } else if (scope === 'file' && action === 'read') {
      result = await client.readProjectFileText({
        projectId: flags.projectId,
        path: flags.path,
        scopeId: flags.scopeId,
        filePath: flags.file,
      });
    } else if (scope === 'file' && action === 'delete') {
      result = await client.deleteProjectFiles({
        projectId: flags.projectId,
        path: flags.path,
        scopeId: flags.scopeId,
        paths: [flags.file],
      });
    } else if (scope === 'web-session' && action === 'list') {
      result = await client.listWebSessions({
        projectId: flags.projectId,
        path: flags.path,
      });
    } else if (scope === 'web-session' && action === 'create') {
      result = await client.createWebSession({
        projectId: flags.projectId,
        path: flags.path,
        worktreeId: flags.worktreeId,
        agent: flags.agent,
        model: flags.model,
        reasoningEffort: flags.reasoningEffort,
        workflowMode: flags.workflowMode,
        permissionLevel: flags.permissionLevel,
        permissionMode: flags.permissionMode,
        title: flags.title,
      });
    } else if (scope === 'web-session' && action === 'connect') {
      result = await withWebSessionCommandChannel(client, channel => channel.connect(flags.sessionId));
    } else if (scope === 'web-session' && action === 'snapshot') {
      result = await client.getWebSessionSnapshot({
        projectId: flags.projectId,
        path: flags.path,
        sessionId: flags.sessionId,
        limit: parseIntegerFlag(flags.limit, 'limit'),
      });
    } else if (scope === 'web-session' && action === 'history') {
      result = await client.getWebSessionHistory({
        projectId: flags.projectId,
        path: flags.path,
        sessionId: flags.sessionId,
        beforeCursor: flags.beforeCursor,
        limit: parseIntegerFlag(flags.limit, 'limit'),
      });
    } else if (scope === 'web-session' && action === 'sync') {
      result = await client.syncWebSession({
        projectId: flags.projectId,
        path: flags.path,
        sessionId: flags.sessionId,
        mode: flags.mode,
        clearExisting: flags.clearExisting,
      });
    } else if (scope === 'web-session' && action === 'state') {
      result = await client.getWebSessionState({
        projectId: flags.projectId,
        path: flags.path,
        sessionId: flags.sessionId,
        limit: parseIntegerFlag(flags.limit, 'limit'),
      });
    } else if (scope === 'web-session' && action === 'answer-pending') {
      result = await answerPendingWithStrategy(client, flags);
    } else if (scope === 'web-session' && action === 'execute-plan') {
      result = await client.executeLatestPlan({
        projectId: flags.projectId,
        path: flags.path,
        sessionId: flags.sessionId,
        prompt: withStrictCwdPrompt('Implement the plan.', flags.strictCwd),
        limit: parseIntegerFlag(flags.limit, 'limit'),
      });
    } else if (scope === 'web-session' && action === 'wait') {
      result = await client.waitForWebSessionState({
        projectId: flags.projectId,
        path: flags.path,
        sessionId: flags.sessionId,
        until: normalizeUntil(flags.until),
        intervalMs: parseIntegerFlag(flags.intervalMs, 'intervalMs') || 2000,
        timeoutMs: parseIntegerFlag(flags.timeoutMs, 'timeoutMs') || 120000,
        limit: parseIntegerFlag(flags.limit, 'limit'),
        settleMs: parseIntegerFlag(flags.settleMs, 'settleMs') || 0,
      });
    } else if (scope === 'web-session' && action === 'run') {
      result = await runWebSessionFlow(client, flags);
    } else if (scope === 'web-session' && action === 'archive') {
      result = await client.archiveWebSession({
        projectId: flags.projectId,
        path: flags.path,
        sessionId: flags.sessionId,
      });
    } else if (scope === 'web-session' && action === 'unarchive') {
      result = await client.unarchiveWebSession({
        projectId: flags.projectId,
        path: flags.path,
        sessionId: flags.sessionId,
      });
    } else if (scope === 'web-session' && action === 'rename') {
      result = await client.renameWebSession({
        projectId: flags.projectId,
        path: flags.path,
        sessionId: flags.sessionId,
        title: flags.title,
      });
    } else if (scope === 'web-session' && action === 'close') {
      result = await client.closeWebSession({
        projectId: flags.projectId,
        path: flags.path,
        sessionId: flags.sessionId,
      });
    } else if (scope === 'web-session' && action === 'delete') {
      result = await client.deleteWebSession({
        projectId: flags.projectId,
        path: flags.path,
        sessionId: flags.sessionId,
      });
    } else if (scope === 'web-session' && action === 'runtime-config') {
      result = await client.getWebSessionRuntimeConfig();
    } else if (scope === 'web-session' && action === 'command-group') {
      result = await client.getWebSessionCommandGroup({
        projectId: flags.projectId,
        path: flags.path,
        sessionId: flags.sessionId,
        groupId: flags.groupId,
      });
    } else if (scope === 'web-session' && action === 'attach') {
      result = await client.uploadWebSessionAttachment({
        projectId: flags.projectId,
        path: flags.path,
        filePath: flags.file,
      });
    } else if (scope === 'web-session' && action === 'send') {
      result = await withWebSessionCommandChannel(client, async channel => {
        let uploadedAttachments = [];
        if (flags.imagePaths.length > 0) {
          const snapshot = await channel.connect(flags.sessionId);
          const projectId = snapshot?.session?.projectId || flags.projectId;
          if (!projectId) {
            throw new Error(`unable to resolve the project for web session ${flags.sessionId}`);
          }
          uploadedAttachments = await uploadWebSessionImages(client, flags.imagePaths, {
            projectId,
          });
        }
        return await channel.sendMessage(flags.sessionId, {
          text: withStrictCwdPrompt(flags.text || flags.prompt, flags.strictCwd),
          attachmentIds: mergeAttachmentIds(flags.attachmentIds, uploadedAttachments),
          mode: flags.mode,
        });
      });
    } else if (scope === 'web-session' && action === 'approve') {
      result = await withWebSessionCommandChannel(client, channel => channel.approve(flags.sessionId));
    } else if (scope === 'web-session' && action === 'reject') {
      result = await withWebSessionCommandChannel(client, channel => channel.reject(flags.sessionId));
    } else if (scope === 'web-session' && action === 'user-input') {
      result = await withWebSessionCommandChannel(client, channel =>
        channel.answerUserInput(flags.sessionId, {
          itemId: flags.itemId,
          answers: parseJsonFlag(flags.answersJson, 'answersJson'),
        }),
      );
    } else if (scope === 'web-session' && action === 'set-model') {
      result = await withWebSessionCommandChannel(client, channel =>
        channel.updateModel(flags.sessionId, { model: flags.model }),
      );
    } else if (scope === 'web-session' && action === 'set-reasoning') {
      result = await withWebSessionCommandChannel(client, channel =>
        channel.updateReasoningEffort(flags.sessionId, {
          reasoningEffort: flags.reasoningEffort,
        }),
      );
    } else if (scope === 'web-session' && action === 'set-workflow') {
      result = await withWebSessionCommandChannel(client, channel =>
        channel.updateWorkflowMode(flags.sessionId, {
          workflowMode: flags.workflowMode,
        }),
      );
    } else if (scope === 'web-session' && action === 'set-permission') {
      result = await withWebSessionCommandChannel(client, channel =>
        channel.updatePermissionLevel(flags.sessionId, {
          permissionLevel: flags.permissionLevel,
        }),
      );
    } else if (scope === 'web-session' && action === 'set-agent') {
      result = await withWebSessionCommandChannel(client, channel =>
        channel.updateAgent(flags.sessionId, {
          agent: flags.agent,
        }),
      );
    } else if (scope === 'web-session' && action === 'move') {
      result = await withWebSessionCommandChannel(client, channel =>
        channel.move(flags.sessionId, {
          prevSessionId: flags.prevSessionId,
          nextSessionId: flags.nextSessionId,
        }),
      );
    } else if (scope === 'web-session' && action === 'watch') {
      return await watchWebSession(client, flags, stdout);
    } else {
      throw new Error(`unsupported command: ${scope} ${action}`);
    }

    printJson(stdout, sanitizeConnection(result));
    return 0;
  } catch (error) {
    const payload = {
      error: {
        name: error instanceof Error ? error.name : 'Error',
        message: error instanceof Error ? error.message : String(error),
      },
    };
    printJson(stderr, payload);
    return 1;
  }
}
