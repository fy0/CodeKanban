import { pbkdf2Sync } from 'node:crypto';
import { mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';

import { assertCommandScope } from './runtime.js';

export const APP_NAME = 'codekanban-cli';
export const DEFAULT_BASE_URL = 'http://127.0.0.1:3007';
export const SESSION_FILE_NAME = 'session.json';

const BASE_URL_ENV_VARS = ['CODEKANBAN_BASE_URL', 'BASE_URL'];
const TOKEN_ENV_VARS = ['CODEKANBAN_TOKEN', 'TOKEN'];
const USERNAME_ENV_VARS = ['CODEKANBAN_USERNAME', 'USERNAME'];
const PASSWORD_ENV_VARS = ['CODEKANBAN_PASSWORD', 'PASSWORD'];
const COOKIE_NAME = 'codekanban_auth';
const VALUE_FLAGS = new Set([
  '--base-url',
  '--project-id',
  '--path',
  '--prompt',
  '--text',
  '--agent',
  '--claude-runtime',
  '--model',
  '--profile',
  '--sandbox',
  '--approval-policy',
  '--add-dir',
  '--attachment-id',
  '--image',
  '--extra-arg',
  '--title',
  '--working-dir',
  '--worktree-id',
  '--session-id',
  '--id',
  '--tool-use-id',
  '--reasoning-effort',
  '--workflow-mode',
  '--permission-level',
  '--permission-mode',
  '--limit',
  '--before-cursor',
  '--mode',
  '--group-id',
  '--scope-id',
  '--file',
  '--item-id',
  '--answers-json',
  '--answer-strategy',
  '--prev-session-id',
  '--next-session-id',
  '--idle-timeout-ms',
  '--max-events',
  '--interval-ms',
  '--timeout-ms',
  '--until',
  '--delete-file-before',
  '--read-file-after',
  '--project-index',
]);
const BOOLEAN_FLAGS = new Set([
  '--no-terminal',
  '--no-ai',
  '--refresh',
  '--clear-existing',
  '--raw',
  '--strict-cwd',
  '--if-exists',
]);

function readFlagValue(argv, index, flag) {
  const value = argv[index + 1];
  if (value == null || value.startsWith('--')) {
    throw new Error(`${flag} requires a value`);
  }
  return value;
}

function firstEnvValue(env, keys) {
  for (const key of keys) {
    const value = typeof env[key] === 'string' ? env[key].trim() : '';
    if (value) {
      return value;
    }
  }
  return '';
}

function normalizeBaseUrl(value) {
  const trimmed = String(value || '').trim();
  if (!trimmed) {
    return DEFAULT_BASE_URL;
  }
  return trimmed.endsWith('/') ? trimmed.slice(0, -1) : trimmed;
}

function extractPositionals(argv) {
  const positionals = [];
  for (let index = 0; index < argv.length; index += 1) {
    const token = argv[index];
    if (!token.startsWith('--')) {
      positionals.push(token);
      continue;
    }
    if (BOOLEAN_FLAGS.has(token)) {
      continue;
    }
    if (VALUE_FLAGS.has(token) && index + 1 < argv.length) {
      index += 1;
    }
  }
  return positionals;
}

export function getConfigDir({ env = process.env, homeDir = os.homedir(), platform = process.platform } = {}) {
  if (platform === 'win32') {
    const appData = typeof env.APPDATA === 'string' ? env.APPDATA.trim() : '';
    return path.join(appData || path.join(homeDir, 'AppData', 'Roaming'), APP_NAME);
  }
  const xdg = typeof env.XDG_CONFIG_HOME === 'string' ? env.XDG_CONFIG_HOME.trim() : '';
  return path.join(xdg || path.join(homeDir, '.config'), APP_NAME);
}

export function getSessionFilePath(options = {}) {
  return path.join(getConfigDir(options), SESSION_FILE_NAME);
}

function normalizeSavedSession(value) {
  if (!value || typeof value !== 'object') {
    return null;
  }
  const baseUrl = normalizeBaseUrl(value.base_url || value.baseURL || '');
  const accessToken = typeof value.access_token === 'string' ? value.access_token.trim() : '';
  const username = typeof value.username === 'string' ? value.username.trim() : '';
  const savedAt = typeof value.saved_at === 'string' ? value.saved_at.trim() : '';
  if (!baseUrl && !accessToken) {
    return null;
  }
  return {
    base_url: baseUrl,
    access_token: accessToken,
    username,
    saved_at: savedAt,
  };
}

export async function readSavedSession(options = {}) {
  try {
    const raw = await readFile(getSessionFilePath(options), 'utf8');
    return normalizeSavedSession(JSON.parse(raw));
  } catch (error) {
    if (error && (error.code === 'ENOENT' || error.name === 'SyntaxError')) {
      return null;
    }
    throw error;
  }
}

export async function writeSavedSession(session, options = {}) {
  const normalized = normalizeSavedSession(session);
  if (!normalized || !normalized.access_token) {
    throw new Error('saved session requires base_url and access_token');
  }
  const filePath = getSessionFilePath(options);
  await mkdir(path.dirname(filePath), { recursive: true });
  await writeFile(filePath, `${JSON.stringify(normalized, null, 2)}\n`, 'utf8');
  if ((options.platform || process.platform) !== 'win32') {
    await safeChmod(filePath, 0o600);
  }
  return filePath;
}

export async function clearSavedSession(options = {}) {
  const filePath = getSessionFilePath(options);
  await rm(filePath, { force: true });
  return filePath;
}

async function safeChmod(filePath, mode) {
  const fs = await import('node:fs/promises');
  try {
    await fs.chmod(filePath, mode);
  } catch {
    // Best-effort only.
  }
}

export function parseCliState(argv) {
  const passthrough = [];
  const flags = {
    help: false,
    version: false,
  };

  for (let index = 0; index < argv.length; index += 1) {
    const token = argv[index];
    switch (token) {
      case '--help':
      case '-h':
        flags.help = true;
        break;
      case '--version':
      case '-v':
        flags.version = true;
        break;
      case '--token':
        flags.token = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--token-file':
        flags.tokenFile = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--token-stdin':
        flags.tokenStdin = true;
        break;
      case '--password':
        flags.password = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--password-file':
        flags.passwordFile = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--password-stdin':
        flags.passwordStdin = true;
        break;
      case '--username':
        flags.username = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--project-name':
        flags.projectName = readFlagValue(argv, index, token);
        index += 1;
        break;
      case '--project-index':
        flags.projectIndex = readFlagValue(argv, index, token);
        index += 1;
        break;
      default:
        passthrough.push(token);
        break;
    }
  }

  return {
    flags,
    passthrough,
    positionals: extractPositionals(passthrough),
  };
}

function createRuntimeHelpText({ sessionFilePath, commandHelpText }) {
  return `codekanban-cli - installable Codex skill CLI for CodeKanban

Usage:
  codekanban-cli <scope> <action> [options]
  codekanban-cli auth save-token [options]
  codekanban-cli auth clear-token
  codekanban-cli project list
  codekanban-cli project resolve --project-name <name>

Defaults:
  Base URL: ${DEFAULT_BASE_URL}
  Session file: ${sessionFilePath}

Global auth/config options:
  --base-url <url>       Override the CodeKanban server base URL
  --token <token>        Use an auth token for this command
  --token-file <path>    Read the auth token from a file
  --token-stdin          Read the auth token from stdin
  --password <password>  Login with a password for this command
  --password-file <path> Read the password from a file
  --password-stdin       Read the password from stdin
  --username <name>      Optional label saved with auth save-token
  --project-name <name>  Resolve a project by name before running a project command
  --project-index <n>   Choose the nth matching project candidate when names are ambiguous
  --help                 Show this help text
  --version              Show the CLI version

Environment variables:
  CODEKANBAN_BASE_URL, BASE_URL
  CODEKANBAN_TOKEN, TOKEN
  CODEKANBAN_PASSWORD, PASSWORD
  CODEKANBAN_USERNAME, USERNAME

Auth helpers:
  codekanban-cli auth save-token --password-stdin
  codekanban-cli auth save-token --token-file <path>
  codekanban-cli auth clear-token

Project helpers:
  codekanban-cli project list
  codekanban-cli project resolve --project-name codekanban
  codekanban-cli web-session create --project-name codekanban --agent codex --title "Planning session"

Command surface:
${indentLines(String(commandHelpText || '').trim(), '  ')}
`;
}

function indentLines(text, prefix) {
  return String(text || '')
    .split('\n')
    .map(line => `${prefix}${line}`)
    .join('\n');
}

async function readSecretFromFile(filePath) {
  return String(await readFile(filePath, 'utf8')).trim();
}

async function readSecretFromStdin(stdin) {
  if (!stdin) {
    throw new Error('stdin is unavailable');
  }
  const chunks = [];
  for await (const chunk of stdin) {
    chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(String(chunk)));
  }
  return Buffer.concat(chunks).toString('utf8').trim();
}

async function resolveSecretValue({ directValue, filePath, useStdin, stdin, label }) {
  const inlineValue = typeof directValue === 'string' ? directValue.trim() : '';
  if (inlineValue) {
    return inlineValue;
  }
  if (filePath) {
    return await readSecretFromFile(filePath);
  }
  if (useStdin) {
    const secret = await readSecretFromStdin(stdin);
    if (!secret) {
      throw new Error(`${label} stdin was empty`);
    }
    return secret;
  }
  return '';
}

export function resolveBaseUrl({ flags, env = process.env, savedSession } = {}) {
  const explicit = extractBaseUrlFromArgs(flags);
  if (explicit) {
    return explicit;
  }
  const fromEnv = firstEnvValue(env, BASE_URL_ENV_VARS);
  if (fromEnv) {
    return normalizeBaseUrl(fromEnv);
  }
  if (savedSession?.base_url) {
    return normalizeBaseUrl(savedSession.base_url);
  }
  return DEFAULT_BASE_URL;
}

function extractBaseUrlFromArgs(flags) {
  const value = typeof flags?.baseUrl === 'string' ? flags.baseUrl.trim() : '';
  return value ? normalizeBaseUrl(value) : '';
}

function parseBaseUrlFromPassthrough(argv) {
  for (let index = 0; index < argv.length; index += 1) {
    if (argv[index] === '--base-url') {
      return normalizeBaseUrl(readFlagValue(argv, index, '--base-url'));
    }
  }
  return '';
}

function parseFlagValue(argv, flagName) {
  for (let index = 0; index < argv.length; index += 1) {
    if (argv[index] === flagName) {
      return readFlagValue(argv, index, flagName);
    }
  }
  return '';
}

function hasPassthroughFlag(argv, flagName) {
  return argv.includes(flagName);
}

function withoutFlag(argv, flagName, { takesValue = false } = {}) {
  const next = [];
  for (let index = 0; index < argv.length; index += 1) {
    const token = argv[index];
    if (token !== flagName) {
      next.push(token);
      continue;
    }
    if (takesValue) {
      index += 1;
    }
  }
  return next;
}

function withFlagValue(argv, flagName, value) {
  const next = withoutFlag(argv, flagName, { takesValue: true });
  next.push(flagName, value);
  return next;
}

function isLocalBaseUrl(baseUrl) {
  try {
    const url = new URL(`${normalizeBaseUrl(baseUrl)}/`);
    return ['127.0.0.1', 'localhost', '::1'].includes(url.hostname);
  } catch {
    return false;
  }
}

function commandNeedsProjectTarget(scope, action) {
  const key = `${scope} ${action}`;
  return new Set([
    'session list',
    'workflow start',
    'web-session list',
    'web-session create',
    'file scopes',
    'file read',
    'file delete',
    'web-session snapshot',
    'web-session history',
    'web-session sync',
    'web-session state',
    'web-session answer-pending',
    'web-session execute-plan',
    'web-session wait',
    'web-session run',
    'web-session archive',
    'web-session unarchive',
    'web-session rename',
    'web-session close',
    'web-session delete',
    'web-session command-group',
    'web-session attach',
  ]).has(key);
}

function normalizeProjectSearchToken(value) {
  return String(value || '').trim().toLowerCase();
}

function pathBaseName(projectPath) {
  const normalized = String(projectPath || '').replace(/[\\/]+$/, '');
  const parts = normalized.split(/[\\/]/).filter(Boolean);
  return parts.length > 0 ? parts[parts.length - 1].toLowerCase() : '';
}

function uniqueById(items) {
  const seen = new Set();
  return items.filter(item => {
    if (!item?.id || seen.has(item.id)) {
      return false;
    }
    seen.add(item.id);
    return true;
  });
}

function describeProject(project) {
  return {
    id: project.id,
    name: project.name || '',
    path: project.path || '',
    pathBaseName: pathBaseName(project.path || ''),
  };
}

function parseProjectIndex(value) {
  if (value == null || value === '') {
    return null;
  }
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < 1) {
    throw new Error('--project-index must be a positive integer');
  }
  return parsed;
}

function formatProjectCandidate(project, index) {
  return `${index}) ${project.name || '(unnamed)'} [${project.id}] ${project.path || ''}`.trim();
}

function selectProjectCandidate(projectName, candidates, projectIndex, label) {
  const normalizedIndex = parseProjectIndex(projectIndex);
  if (normalizedIndex != null) {
    const selected = candidates[normalizedIndex - 1];
    if (!selected) {
      throw new Error(
        `project index ${normalizedIndex} is out of range for "${projectName}"; available candidates: ${candidates.map((project, index) => formatProjectCandidate(project, index + 1)).join('; ')}`,
      );
    }
    return selected;
  }
  throw new Error(
    `project name "${projectName}" ${label}: ${candidates.map((project, index) => formatProjectCandidate(project, index + 1)).join('; ')}. Re-run with --project-index <n> or --project-id <id>.`,
  );
}

function resolveProjectByName(projects, projectName, projectIndex) {
  const needle = normalizeProjectSearchToken(projectName);
  if (!needle) {
    throw new Error('--project-name requires a value');
  }
  const exact = uniqueById(projects.filter(project => normalizeProjectSearchToken(project.name) == needle));
  if (exact.length === 1) {
    return exact[0];
  }
  if (exact.length > 1) {
    return selectProjectCandidate(projectName, exact, projectIndex, 'matches multiple projects');
  }
  const baseMatches = uniqueById(projects.filter(project => pathBaseName(project.path) == needle));
  if (baseMatches.length === 1) {
    return baseMatches[0];
  }
  if (baseMatches.length > 1) {
    return selectProjectCandidate(projectName, baseMatches, projectIndex, 'matches multiple project paths');
  }
  const fuzzy = uniqueById(projects.filter(project => normalizeProjectSearchToken(project.name).includes(needle) || pathBaseName(project.path).includes(needle)));
  if (fuzzy.length === 1) {
    return fuzzy[0];
  }
  if (fuzzy.length > 1) {
    return selectProjectCandidate(projectName, fuzzy, projectIndex, 'is ambiguous');
  }
  throw new Error(`project name "${projectName}" was not found on ${baseUrlLabel(projects)}`);
}

function baseUrlLabel(projects) {
  return Array.isArray(projects) ? 'the target CodeKanban server' : 'the target CodeKanban server';
}

async function listProjectsFromServer({ baseUrl, headers, fetchImpl }) {
  const { response, payload } = await requestJson(baseUrl, '/api/v1/projects', {
    headers,
    fetchImpl,
  });
  if (!response.ok) {
    throw new Error(extractErrorMessage(payload, `project list request failed with ${response.status}`));
  }
  const items = Array.isArray(payload?.body?.items)
    ? payload.body.items
    : Array.isArray(payload?.items)
      ? payload.items
      : [];
  return items;
}

function ensureProjectTarget({ argv, scope, action, baseUrl, cwd }) {
  const hasProjectId = hasPassthroughFlag(argv, '--project-id');
  const hasPath = hasPassthroughFlag(argv, '--path');
  if (hasProjectId || hasPath) {
    return argv;
  }
  if (!commandNeedsProjectTarget(scope, action)) {
    return argv;
  }
  if (!isLocalBaseUrl(baseUrl)) {
    throw new Error(`remote CodeKanban commands require --project-id, --project-name, or a server-side --path for ${scope} ${action}`);
  }
  const resolvedCwd = String(cwd || process.cwd()).trim();
  if (!resolvedCwd) {
    throw new Error(`unable to resolve a working directory for ${scope} ${action}`);
  }
  return [...argv, '--path', resolvedCwd];
}

async function rewriteProjectNameToProjectId({ argv, projectName, projectIndex, baseUrl, headers, fetchImpl, stdout, scope, action }) {
  if (!projectName) {
    return argv;
  }
  const projects = await listProjectsFromServer({ baseUrl, headers, fetchImpl });
  const project = resolveProjectByName(projects, projectName, projectIndex);
  if (stdout) {
    // Keep this silent during normal command execution.
  }
  const withoutProjectFlags = withoutFlag(
    withoutFlag(argv, '--project-name', { takesValue: true }),
    '--project-index',
    { takesValue: true },
  );
  return withFlagValue(withoutProjectFlags, '--project-id', project.id);
}

async function handleProjectCommand({ action, flags, baseUrl, headers, fetchImpl, stdout }) {
  const projects = await listProjectsFromServer({ baseUrl, headers, fetchImpl });
  if (action == 'list') {
    printJson(stdout, {
      baseUrl,
      items: projects.map(describeProject),
    });
    return 0;
  }
  if (action == 'resolve') {
    const project = resolveProjectByName(projects, flags.projectName, flags.projectIndex);
    printJson(stdout, {
      baseUrl,
      item: describeProject(project),
    });
    return 0;
  }
  throw new Error(`unsupported project command: ${action}`);
}

export function createAuthHeaders(token) {
  const trimmed = typeof token === 'string' ? token.trim() : '';
  if (!trimmed) {
    return {};
  }
  return {
    Authorization: `Bearer ${trimmed}`,
    Cookie: `${COOKIE_NAME}=${encodeURIComponent(trimmed)}`,
  };
}

function extractItem(payload) {
  if (payload?.body?.item) {
    return payload.body.item;
  }
  if (payload?.item) {
    return payload.item;
  }
  return payload ?? null;
}

function extractErrorMessage(payload, fallback) {
  if (payload?.detail) {
    return String(payload.detail);
  }
  if (payload?.error?.message) {
    return String(payload.error.message);
  }
  if (payload?.body?.message) {
    return String(payload.body.message);
  }
  if (payload?.message) {
    return String(payload.message);
  }
  return fallback;
}

async function requestJson(baseUrl, pathname, { method = 'GET', headers = {}, body, fetchImpl = globalThis.fetch } = {}) {
  if (!fetchImpl) {
    throw new Error('fetch implementation is unavailable');
  }
  const response = await fetchImpl(new URL(pathname, `${normalizeBaseUrl(baseUrl)}/`), {
    method,
    headers: {
      Accept: 'application/json',
      ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}),
      ...headers,
    },
    ...(body !== undefined ? { body: JSON.stringify(body) } : {}),
  });
  const raw = await response.text();
  let payload = null;
  if (raw) {
    try {
      payload = JSON.parse(raw);
    } catch {
      payload = { raw };
    }
  }
  return { response, payload };
}

function deriveClientHash(password, salt, rounds) {
  return pbkdf2Sync(password, salt, Math.max(1, Number(rounds) || 1), 64, 'sha512').toString('hex');
}

function extractSetCookieHeaders(response) {
  if (!response?.headers) {
    return [];
  }
  if (typeof response.headers.getSetCookie === 'function') {
    return response.headers.getSetCookie();
  }
  const combined = response.headers.get('set-cookie');
  return combined ? [combined] : [];
}

function extractAuthTokenFromResponse(response) {
  const pattern = new RegExp(`(?:^|\\s|,)${COOKIE_NAME}=([^;]+)`);
  for (const raw of extractSetCookieHeaders(response)) {
    const match = String(raw).match(pattern);
    if (match?.[1]) {
      return decodeURIComponent(match[1]);
    }
  }
  return '';
}

export async function fetchAuthStatus({ baseUrl, token = '', fetchImpl = globalThis.fetch } = {}) {
  const headers = createAuthHeaders(token);
  const { response, payload } = await requestJson(baseUrl, '/api/v1/auth/status', {
    headers,
    fetchImpl,
  });
  if (!response.ok) {
    throw new Error(extractErrorMessage(payload, `auth status request failed with ${response.status}`));
  }
  return extractItem(payload);
}

export async function validateToken({ baseUrl, token, fetchImpl = globalThis.fetch } = {}) {
  const status = await fetchAuthStatus({ baseUrl, token, fetchImpl });
  if (!status?.enabled) {
    throw new Error('password protection is disabled on this server, so there is no token to save');
  }
  if (!status?.authenticated) {
    throw new Error('token validation failed');
  }
  return status;
}

export async function loginWithPassword({ baseUrl, password, fetchImpl = globalThis.fetch } = {}) {
  const status = await fetchAuthStatus({ baseUrl, fetchImpl });
  if (!status?.enabled) {
    return {
      enabled: false,
      token: '',
      status,
    };
  }
  const clientHash = deriveClientHash(password, status.frontendSalt || '', status.frontendPBKDF2Rounds || 20000);
  const { response, payload } = await requestJson(baseUrl, '/api/v1/auth/login', {
    method: 'POST',
    body: { clientHash },
    fetchImpl,
  });
  if (!response.ok) {
    throw new Error(extractErrorMessage(payload, `login failed with ${response.status}`));
  }
  const token = extractAuthTokenFromResponse(response);
  if (!token) {
    throw new Error('login succeeded but no auth session token was returned');
  }
  const validated = await validateToken({ baseUrl, token, fetchImpl });
  return {
    enabled: true,
    token,
    status: validated,
  };
}

function resolveUsername({ flags, env = process.env, savedSession, baseUrl }) {
  const explicit = typeof flags.username === 'string' ? flags.username.trim() : '';
  if (explicit) {
    return explicit;
  }
  const fromEnv = firstEnvValue(env, USERNAME_ENV_VARS);
  if (fromEnv) {
    return fromEnv;
  }
  if (savedSession?.base_url === baseUrl && savedSession?.username) {
    return savedSession.username;
  }
  return '';
}

async function resolveRuntimeAuth({ flags, env = process.env, savedSession, baseUrl, stdin, fetchImpl = globalThis.fetch } = {}) {
  const explicitToken = await resolveSecretValue({
    directValue: flags.token,
    label: 'token',
  });
  if (explicitToken) {
    return { token: explicitToken, source: 'arg' };
  }

  const explicitPassword = await resolveSecretValue({
    directValue: flags.password,
    label: 'password',
  });
  if (explicitPassword) {
    const login = await loginWithPassword({ baseUrl, password: explicitPassword, fetchImpl });
    return { token: login.token, source: 'arg-password', enabled: login.enabled };
  }

  const tokenFromAltInput = await resolveSecretValue({
    filePath: flags.tokenFile,
    useStdin: flags.tokenStdin,
    stdin,
    label: 'token',
  });
  if (tokenFromAltInput) {
    return { token: tokenFromAltInput, source: 'input' };
  }

  const passwordFromAltInput = await resolveSecretValue({
    filePath: flags.passwordFile,
    useStdin: flags.passwordStdin,
    stdin,
    label: 'password',
  });
  if (passwordFromAltInput) {
    const login = await loginWithPassword({ baseUrl, password: passwordFromAltInput, fetchImpl });
    return { token: login.token, source: 'input-password', enabled: login.enabled };
  }

  const envToken = firstEnvValue(env, TOKEN_ENV_VARS);
  if (envToken) {
    return { token: envToken, source: 'env' };
  }

  const envPassword = firstEnvValue(env, PASSWORD_ENV_VARS);
  if (envPassword) {
    const login = await loginWithPassword({ baseUrl, password: envPassword, fetchImpl });
    return { token: login.token, source: 'env-password', enabled: login.enabled };
  }

  if (savedSession?.access_token && savedSession?.base_url === baseUrl) {
    return { token: savedSession.access_token, source: 'session' };
  }

  return { token: '', source: 'none' };
}

async function buildSaveTokenPayload({ flags, env, savedSession, baseUrl, stdin, fetchImpl, now }) {
  const username = resolveUsername({ flags, env, savedSession, baseUrl });
  const runtimeAuth = await resolveRuntimeAuth({
    flags,
    env,
    savedSession,
    baseUrl,
    stdin,
    fetchImpl,
  });
  if (!runtimeAuth.token) {
    if (runtimeAuth.source.includes('password') && runtimeAuth.enabled === false) {
      throw new Error('password protection is disabled on this server, so there is no token to save');
    }
    throw new Error('auth save-token requires a token or password from args, stdin, env, or an existing saved session');
  }
  const validated = await validateToken({
    baseUrl,
    token: runtimeAuth.token,
    fetchImpl,
  });
  return {
    base_url: baseUrl,
    access_token: runtimeAuth.token,
    username,
    saved_at: (now || new Date()).toISOString(),
    authenticated: validated.authenticated === true,
  };
}

function printJson(stream, payload) {
  stream.write(`${JSON.stringify(payload, null, 2)}\n`);
}

async function handleAuthCommand({ action, flags, env, stdout, stderr, stdin, savedSession, fetchImpl, now, pathOptions }) {
  const baseUrl = resolveBaseUrl({ flags: { ...flags, baseUrl: flags.baseUrl }, env, savedSession });

  if (action === 'save-token') {
    const session = await buildSaveTokenPayload({
      flags,
      env,
      savedSession,
      baseUrl,
      stdin,
      fetchImpl,
      now,
    });
    const filePath = await writeSavedSession(session, pathOptions);
    printJson(stdout, {
      message: 'token saved',
      sessionFile: filePath,
      session,
    });
    return 0;
  }

  if (action === 'clear-token') {
    const filePath = await clearSavedSession(pathOptions);
    printJson(stdout, {
      message: 'saved token cleared',
      sessionFile: filePath,
    });
    return 0;
  }

  if (action === 'status') {
    const runtimeAuth = await resolveRuntimeAuth({
      flags,
      env,
      savedSession,
      baseUrl,
      stdin,
      fetchImpl,
    });
    const status = await fetchAuthStatus({
      baseUrl,
      token: runtimeAuth.token,
      fetchImpl,
    });
    printJson(stdout, {
      baseUrl,
      authSource: runtimeAuth.source,
      status,
    });
    return 0;
  }

  stderr.write(`unsupported auth command: ${action}\n`);
  return 1;
}

export async function runCodeKanbanCliWithRuntime(argv, runtimeBindings, options = {}) {
  const stdout = options.stdout || process.stdout;
  const stderr = options.stderr || process.stderr;
  const stdin = options.stdin || process.stdin;
  const env = options.env || process.env;
  const fetchImpl = options.fetchImpl || globalThis.fetch;
  const runtimeRunner = options.runner || runtimeBindings?.runCli;
  const pathOptions = {
    env,
    homeDir: options.homeDir,
    platform: options.platform,
  };
  const sessionFilePath = getSessionFilePath(pathOptions);

  try {
    const state = parseCliState(argv);
    const baseUrlFromArgs = parseBaseUrlFromPassthrough(state.passthrough);
    state.flags.baseUrl = baseUrlFromArgs;

    if (state.flags.version) {
      const version = options.version || '0.2.0';
      stdout.write(`${APP_NAME} ${version}\n`);
      return 0;
    }

    if (state.flags.help || state.positionals.length === 0) {
      stdout.write(createRuntimeHelpText({ sessionFilePath, commandHelpText: runtimeBindings?.createHelpText?.(APP_NAME) }));
      return 0;
    }

    const [scope, action] = state.positionals;
    assertCommandScope(scope);
    const savedSession = await readSavedSession(pathOptions);

    if (scope === 'auth') {
      return await handleAuthCommand({
        action,
        flags: state.flags,
        env,
        stdout,
        stderr,
        stdin,
        savedSession,
        fetchImpl,
        now: options.now,
        pathOptions,
      });
    }

    const baseUrl = resolveBaseUrl({ flags: state.flags, env, savedSession });
    const runtimeAuth = await resolveRuntimeAuth({
      flags: state.flags,
      env,
      savedSession,
      baseUrl,
      stdin,
      fetchImpl,
    });
    const headers = createAuthHeaders(runtimeAuth.token);
    const webSocketOptions = Object.keys(headers).length > 0 ? { headers } : undefined;

    if (scope === 'project') {
      return await handleProjectCommand({
        action,
        flags: state.flags,
        baseUrl,
        headers,
        fetchImpl,
        stdout,
      });
    }

    let argvForSdk = state.passthrough;
    argvForSdk = await rewriteProjectNameToProjectId({
      argv: argvForSdk,
      projectName: state.flags.projectName,
      projectIndex: state.flags.projectIndex,
      baseUrl,
      headers,
      fetchImpl,
    });
    argvForSdk = ensureProjectTarget({
      argv: argvForSdk,
      scope,
      action,
      baseUrl,
      cwd: options.cwd,
    });

    return await runtimeRunner(argvForSdk, {
      stdout,
      stderr,
      commandName: APP_NAME,
      defaultBaseURL: baseUrl,
      clientOptions: {
        headers,
        fetchImpl,
        webSocketOptions,
      },
    });
  } catch (error) {
    printJson(stderr, {
      error: {
        name: error instanceof Error ? error.name : 'Error',
        message: error instanceof Error ? error.message : String(error),
      },
    });
    return 1;
  }
}
