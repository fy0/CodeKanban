import { CodeKanbanValidationError } from './errors.js';
import { ensureArrayOfStrings, ensureOptionalString, toCommandString } from './utils.js';

export const SANDBOX_MODES = ['read-only', 'workspace-write', 'danger-full-access'];
export const APPROVAL_POLICIES = ['untrusted', 'on-request', 'never'];
export const WORKFLOW_PROFILES = ['plan', 'standard', 'yolo'];
export const AGENTS = ['codex', 'claude', 'pi'];
export const CLAUDE_RUNTIMES = ['claude', 'ccr'];

const KNOWN_STRUCTURED_FLAGS = new Set([
  '-s',
  '--sandbox',
  '-a',
  '--ask-for-approval',
  '--add-dir',
  '--dangerously-bypass-approvals-and-sandbox',
]);

export const PLAN_PROMPT_PREAMBLE = [
  'You are starting in planning mode.',
  'Inspect the project first, summarize the goal, and propose a concrete plan before making changes.',
  'Do not mutate files until the user confirms execution or explicitly asks you to proceed immediately.',
  'If additional directories or permissions are needed, call them out explicitly.',
].join(' ');

function validateEnum(value, allowed, fieldName) {
  if (!value) {
    return undefined;
  }
  if (!allowed.includes(value)) {
    throw new CodeKanbanValidationError(`${fieldName} must be one of: ${allowed.join(', ')}`);
  }
  return value;
}

function detectStructuredFlagConflicts(extraArgs) {
  const args = ensureArrayOfStrings(extraArgs, 'extraArgs');
  const conflicts = [];
  for (const arg of args) {
    if (KNOWN_STRUCTURED_FLAGS.has(arg)) {
      conflicts.push(arg);
    }
  }
  return conflicts;
}

export function composeWorkflowPrompt({ profile = 'standard', prompt }) {
  const userPrompt = String(prompt || '').trim();
  if (!userPrompt) {
    throw new CodeKanbanValidationError('prompt is required');
  }
  if (profile === 'plan') {
    return `${PLAN_PROMPT_PREAMBLE}\n\nUser request:\n${userPrompt}`;
  }
  return userPrompt;
}

export function buildAgentLaunchSpec(options = {}) {
  const agent = validateEnum(options.agent || 'codex', AGENTS, 'agent') || 'codex';
  const profile = validateEnum(options.profile || 'standard', WORKFLOW_PROFILES, 'profile') || 'standard';
  const extraArgs = ensureArrayOfStrings(options.extraArgs, 'extraArgs');

  if (agent === 'claude') {
    const claudeRuntime =
      validateEnum(options.claudeRuntime || 'claude', CLAUDE_RUNTIMES, 'claudeRuntime') || 'claude';
    if (profile !== 'standard') {
      throw new CodeKanbanValidationError('claude only supports the standard profile in v1');
    }
    if (options.permissions) {
      throw new CodeKanbanValidationError('structured permissions are only supported for codex in v1');
    }
    const argv = claudeRuntime === 'ccr' ? ['ccr', 'code', ...extraArgs] : ['claude', ...extraArgs];
    return {
      agent,
      claudeRuntime,
      profile,
      argv,
      command: toCommandString(argv),
      prompt: composeWorkflowPrompt({ profile, prompt: options.prompt }),
    };
  }

  if (agent === 'pi') {
    if (options.permissions) {
      throw new CodeKanbanValidationError('structured permissions are not supported for pi');
    }
    const argv = ['pi', ...extraArgs];
    return {
      agent,
      profile,
      argv,
      command: toCommandString(argv),
      prompt: composeWorkflowPrompt({ profile, prompt: options.prompt }),
    };
  }

  const permissions = options.permissions || {};
  const conflicts = detectStructuredFlagConflicts(extraArgs);
  if (
    conflicts.length > 0 &&
    (permissions.sandbox ||
      permissions.approvalPolicy ||
      (permissions.addDirs && permissions.addDirs.length > 0) ||
      profile === 'yolo')
  ) {
    throw new CodeKanbanValidationError(`extraArgs conflicts with structured permissions: ${conflicts.join(', ')}`);
  }

  if (profile === 'yolo') {
    if (permissions.sandbox || permissions.approvalPolicy || (permissions.addDirs && permissions.addDirs.length > 0)) {
      throw new CodeKanbanValidationError('yolo does not accept structured sandbox, approval, or addDirs overrides');
    }
    const argv = ['codex', '--dangerously-bypass-approvals-and-sandbox', ...extraArgs];
    return {
      agent,
      profile,
      argv,
      command: toCommandString(argv),
      prompt: composeWorkflowPrompt({ profile, prompt: options.prompt }),
    };
  }

  const sandbox =
    validateEnum(ensureOptionalString(permissions.sandbox) || 'workspace-write', SANDBOX_MODES, 'permissions.sandbox') ||
    'workspace-write';
  const approvalPolicy =
    validateEnum(
      ensureOptionalString(permissions.approvalPolicy) || 'on-request',
      APPROVAL_POLICIES,
      'permissions.approvalPolicy',
    ) || 'on-request';
  const addDirs = ensureArrayOfStrings(permissions.addDirs, 'permissions.addDirs');

  const argv = ['codex', '-s', sandbox, '-a', approvalPolicy];
  for (const dir of addDirs) {
    argv.push('--add-dir', dir);
  }
  argv.push(...extraArgs);

  return {
    agent,
    profile,
    argv,
    command: toCommandString(argv),
    prompt: composeWorkflowPrompt({ profile, prompt: options.prompt }),
  };
}
