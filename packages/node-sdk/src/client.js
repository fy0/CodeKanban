import { readFile } from "node:fs/promises";

import { AGENTS, buildAgentLaunchSpec } from "./command-builder.js";
import {
  CodeKanbanConfigError,
  CodeKanbanHttpError,
  CodeKanbanValidationError,
} from "./errors.js";
import { TerminalConnection } from "./terminal-connection.js";
import { WebSessionCommandChannel } from "./web-session-command-channel.js";
import { WebSessionEventStream } from "./web-session-event-stream.js";
import {
  WEB_SESSION_COMMAND_WS_PATH,
  WEB_SESSION_EVENTS_WS_PATH,
  analyzeWebSession,
  ensureImageMimeType,
  normalizePiTreeCreateResult,
  normalizePiTreeNavigateResult,
  normalizePiTreeSnapshot,
  normalizeWebSessionAttachment,
  normalizeWebSessionRuntimeConfig,
} from "./web-session-shared.js";
import {
  ensureArrayOfStrings,
  ensureOptionalString,
  ensureString,
  normalizeBaseUrl,
  normalizeFsPath,
  normalizeTerminalEnter,
  pathBasename,
  sleep,
  toWsUrl,
} from "./utils.js";

const DEFAULT_REQUEST_RETRY = Object.freeze({
  attempts: 2,
  baseDelayMs: 250,
  maxDelayMs: 1000,
});

const RETRYABLE_HTTP_STATUS_CODES = new Set([
  408, 425, 429, 500, 502, 503, 504,
]);

const DEFAULT_WEB_SESSION_AUTO_RETRY_SCOPE = "network_only";
const DEFAULT_WEB_SESSION_AUTO_RETRY_PRESET = "gentle_stop";

function defaultWebSessionWorkflowMode(permissionMode) {
  return permissionMode === "plan" ? "plan" : "default";
}

function normalizeWebSessionAutoRetryScope(scope) {
  const normalized = ensureOptionalString(scope);
  if (
    ["network_only", "network_and_rate_limit", "all_failures"].includes(
      normalized,
    )
  ) {
    return normalized;
  }
  return DEFAULT_WEB_SESSION_AUTO_RETRY_SCOPE;
}

function normalizeWebSessionAutoRetryPreset(preset) {
  const normalized = ensureOptionalString(preset);
  if (["gentle_stop", "aggressive_stop", "sustain_60s"].includes(normalized)) {
    return normalized;
  }
  return DEFAULT_WEB_SESSION_AUTO_RETRY_PRESET;
}

function normalizeWebSessionAutoRetryPolicyMode(mode) {
  const normalized = ensureOptionalString(mode).toLowerCase();
  if (!normalized) {
    return "";
  }
  return normalized === "custom" ? "custom" : "default";
}

function normalizeWebSessionAutoRetryMaxAttempts(value) {
  if (!Number.isFinite(value)) {
    return undefined;
  }
  return Math.min(100, Math.max(0, Math.trunc(value)));
}

function normalizeRequestRetryConfig(value = {}) {
  const attempts = Number.isFinite(value.attempts)
    ? Math.max(1, Math.trunc(value.attempts))
    : DEFAULT_REQUEST_RETRY.attempts;
  const baseDelayMs = Number.isFinite(value.baseDelayMs)
    ? Math.max(1, Math.trunc(value.baseDelayMs))
    : DEFAULT_REQUEST_RETRY.baseDelayMs;
  const maxDelayMs = Number.isFinite(value.maxDelayMs)
    ? Math.max(baseDelayMs, Math.trunc(value.maxDelayMs))
    : DEFAULT_REQUEST_RETRY.maxDelayMs;
  return {
    attempts,
    baseDelayMs,
    maxDelayMs,
  };
}

function resolveRequestRetryConfig(method, defaults, override) {
  if (override === false) {
    return null;
  }

  const normalizedMethod = String(method || "GET").toUpperCase();
  const source = override && typeof override === "object" ? override : {};
  const enabled =
    typeof source.enabled === "boolean"
      ? source.enabled
      : ["GET", "HEAD", "OPTIONS"].includes(normalizedMethod);
  if (!enabled) {
    return null;
  }

  return normalizeRequestRetryConfig({
    ...defaults,
    ...source,
  });
}

function isRetryableRequestError(error) {
  if (error instanceof CodeKanbanHttpError) {
    return RETRYABLE_HTTP_STATUS_CODES.has(error.status);
  }
  if (
    error instanceof CodeKanbanValidationError ||
    error instanceof CodeKanbanConfigError
  ) {
    return false;
  }
  if (error instanceof SyntaxError) {
    return false;
  }
  if (error?.name === "AbortError") {
    return true;
  }
  if (error instanceof TypeError) {
    return true;
  }
  return false;
}

function getRetryDelayMs(retryConfig, attemptIndex) {
  return Math.min(
    retryConfig.maxDelayMs,
    retryConfig.baseDelayMs * 2 ** attemptIndex,
  );
}

function normalizeWebSessionQuestionChoice(value) {
  return String(value || "").trim();
}

function buildAutoWebSessionAnswers(
  questions = [],
  strategy = "prefer-second-or-text",
) {
  const normalizedStrategy = String(
    strategy || "prefer-second-or-text",
  ).trim().toLowerCase();
  if (
    !["prefer-second-or-text", "prefer-second-or-first"].includes(
      normalizedStrategy,
    )
  ) {
    throw new CodeKanbanValidationError(
      `unsupported answer strategy: ${strategy}`,
    );
  }

  const answers = {};
  for (const question of Array.isArray(questions) ? questions : []) {
    const questionId = normalizeWebSessionQuestionChoice(question?.id);
    if (!questionId) {
      continue;
    }
    const options = Array.isArray(question?.options)
      ? question.options
          .map((option) => normalizeWebSessionQuestionChoice(option?.label))
          .filter(Boolean)
      : [];
    if (options[1]) {
      answers[questionId] = [options[1]];
      continue;
    }
    if (normalizedStrategy === "prefer-second-or-first" && options[0]) {
      answers[questionId] = [options[0]];
      continue;
    }
    answers[questionId] = [question?.isSecret ? "redacted" : "continue"];
  }
  return answers;
}

function normalizeWebSessionStateMatcher(until) {
  if (!until) {
    throw new CodeKanbanValidationError("until is required");
  }
  return typeof until === "function"
    ? until
    : Array.isArray(until)
      ? (state) => until.includes(state.phase)
      : (state) => state.phase === until;
}

function getWebSessionPauseReason(state, untilMatcher) {
  if (!state) {
    return null;
  }
  if (state.phase === "done") {
    return "done";
  }
  if (state.phase === "error") {
    return "error";
  }
  if (typeof untilMatcher === "function" && untilMatcher(state)) {
    return "until";
  }
  if (state.nextAction?.type === "approval") {
    return "approval";
  }
  if (state.nextAction?.type === "answer_user_input") {
    return "user_input";
  }
  if (
    state.nextAction?.type === "execute_plan" ||
    (state.latestPlan?.awaitingExecution && state.canSend)
  ) {
    return "execute_plan";
  }
  return null;
}

function isDebouncedPauseReason(reason) {
  return reason === "done" || reason === "error" || reason === "until";
}

function hasOwnField(value, fieldName) {
  return Boolean(
    value &&
      typeof value === "object" &&
      Object.prototype.hasOwnProperty.call(value, fieldName),
  );
}

function extractPayloadItem(payload) {
  if (hasOwnField(payload?.body, "item")) {
    return payload.body.item;
  }
  if (hasOwnField(payload, "item")) {
    return payload.item;
  }
  return null;
}

function extractPayloadItems(payload) {
  if (Array.isArray(payload?.body?.items)) {
    return payload.body.items;
  }
  if (Array.isArray(payload?.items)) {
    return payload.items;
  }
  return [];
}

function extractPayloadMessage(payload) {
  if (typeof payload?.body?.message === "string") {
    return payload.body.message;
  }
  if (typeof payload?.message === "string") {
    return payload.message;
  }
  return "";
}

function normalizeProjectSearchToken(value) {
  return ensureOptionalString(value).toLowerCase();
}

function uniqueProjectsById(projects) {
  const seen = new Set();
  return projects.filter((project) => {
    const id = ensureOptionalString(project?.id);
    if (!id || seen.has(id)) {
      return false;
    }
    seen.add(id);
    return true;
  });
}

function projectPathBaseName(project) {
  const projectPath = ensureOptionalString(project?.path);
  return projectPath ? normalizeProjectSearchToken(pathBasename(projectPath)) : "";
}

function normalizeProjectIndex(projectIndex) {
  if (projectIndex == null || projectIndex === "") {
    return null;
  }
  const normalized = Number(projectIndex);
  if (!Number.isInteger(normalized) || normalized < 1) {
    throw new CodeKanbanValidationError(
      "projectIndex must be a positive integer",
    );
  }
  return normalized;
}

function selectProjectCandidate(
  projectName,
  candidates,
  projectIndex,
  reason,
  matchedBy,
) {
  const normalizedProjectIndex = normalizeProjectIndex(projectIndex);
  if (normalizedProjectIndex != null) {
    const selected = candidates[normalizedProjectIndex - 1];
    if (!selected) {
      throw new CodeKanbanValidationError(
        `projectIndex ${normalizedProjectIndex} is out of range for projectName ${projectName}`,
      );
    }
    return {
      project: selected,
      matchedBy,
    };
  }
  throw new CodeKanbanValidationError(
    `projectName ${projectName} ${reason}; provide projectIndex or projectId`,
  );
}

function resolveProjectByName(projects, projectName, projectIndex) {
  const needle = normalizeProjectSearchToken(projectName);
  if (!needle) {
    throw new CodeKanbanValidationError("projectName is required");
  }

  const exactName = uniqueProjectsById(
    projects.filter(
      (project) => normalizeProjectSearchToken(project?.name) === needle,
    ),
  );
  if (exactName.length === 1) {
    return {
      project: exactName[0],
      matchedBy: "projectName",
    };
  }
  if (exactName.length > 1) {
    return selectProjectCandidate(
      projectName,
      exactName,
      projectIndex,
      "matches multiple projects",
      "projectName",
    );
  }

  const exactBaseName = uniqueProjectsById(
    projects.filter((project) => projectPathBaseName(project) === needle),
  );
  if (exactBaseName.length === 1) {
    return {
      project: exactBaseName[0],
      matchedBy: "projectPathBaseName",
    };
  }
  if (exactBaseName.length > 1) {
    return selectProjectCandidate(
      projectName,
      exactBaseName,
      projectIndex,
      "matches multiple project paths",
      "projectPathBaseName",
    );
  }

  const fuzzy = uniqueProjectsById(
    projects.filter((project) => {
      const normalizedName = normalizeProjectSearchToken(project?.name);
      const normalizedBaseName = projectPathBaseName(project);
      return (
        normalizedName.includes(needle) || normalizedBaseName.includes(needle)
      );
    }),
  );
  if (fuzzy.length === 1) {
    return {
      project: fuzzy[0],
      matchedBy: "projectNameFuzzy",
    };
  }
  if (fuzzy.length > 1) {
    return selectProjectCandidate(
      projectName,
      fuzzy,
      projectIndex,
      "is ambiguous",
      "projectNameFuzzy",
    );
  }

  throw new CodeKanbanValidationError(
    `no CodeKanban project is registered for projectName: ${projectName}`,
  );
}

export class CodeKanbanClient {
  constructor(options = {}) {
    this.baseURL = normalizeBaseUrl(options.baseURL);
    this.headers = { ...(options.headers || {}) };
    this.fetchImpl = options.fetchImpl || globalThis.fetch;
    this.WebSocketImpl = options.WebSocketImpl || globalThis.WebSocket;
    this.webSocketOptions = options.webSocketOptions || null;
    this.requestRetry = normalizeRequestRetryConfig(options.requestRetry);
    if (!this.fetchImpl) {
      throw new CodeKanbanConfigError("fetch implementation is unavailable");
    }
  }

  async requestJson(path, options = {}) {
    const method = String(options.method || "GET").toUpperCase();
    const headers = {
      Accept: "application/json",
      ...this.headers,
      ...(options.headers || {}),
    };
    const request = {
      method,
      headers,
    };
    if (options.body !== undefined) {
      request.body = JSON.stringify(options.body);
      request.headers["Content-Type"] = "application/json";
    }

    const retryConfig = resolveRequestRetryConfig(
      method,
      this.requestRetry,
      options.retry,
    );
    const maxAttempts = retryConfig?.attempts ?? 1;

    for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
      try {
        const response = await this.fetchImpl(
          new URL(path, this.baseURL),
          request,
        );
        const text = await response.text();
        const body = text ? JSON.parse(text) : null;
        if (!response.ok) {
          throw new CodeKanbanHttpError(
            `request failed with ${response.status}`,
            {
              status: response.status,
              method,
              path,
              body,
            },
          );
        }
        return body;
      } catch (error) {
        const canRetry =
          retryConfig &&
          attempt + 1 < maxAttempts &&
          isRetryableRequestError(error);
        if (!canRetry) {
          throw error;
        }
        await sleep(getRetryDelayMs(retryConfig, attempt));
      }
    }

    throw new CodeKanbanValidationError(
      `request retry loop exited unexpectedly for ${method} ${path}`,
    );
  }

  async requestText(path, options = {}) {
    const method = String(options.method || "GET").toUpperCase();
    const headers = {
      ...this.headers,
      ...(options.headers || {}),
    };
    const request = {
      method,
      headers,
    };

    const retryConfig = resolveRequestRetryConfig(
      method,
      this.requestRetry,
      options.retry,
    );
    const maxAttempts = retryConfig?.attempts ?? 1;

    for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
      try {
        const response = await this.fetchImpl(
          new URL(path, this.baseURL),
          request,
        );
        const text = await response.text();
        if (!response.ok) {
          let body = null;
          try {
            body = text ? JSON.parse(text) : null;
          } catch {
            body = text || null;
          }
          throw new CodeKanbanHttpError(
            `request failed with ${response.status}`,
            {
              status: response.status,
              method,
              path,
              body,
            },
          );
        }
        return {
          text,
          contentType:
            typeof response.headers?.get === "function"
              ? response.headers.get("content-type") || ""
              : "",
        };
      } catch (error) {
        const canRetry =
          retryConfig &&
          attempt + 1 < maxAttempts &&
          isRetryableRequestError(error);
        if (!canRetry) {
          throw error;
        }
        await sleep(getRetryDelayMs(retryConfig, attempt));
      }
    }

    throw new CodeKanbanValidationError(
      `request retry loop exited unexpectedly for ${method} ${path}`,
    );
  }

  async resolveProjectReference({
    projectId,
    projectName,
    projectIndex,
    path,
    ensureProject = true,
  }) {
    const { project } = await this.resolveProject({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject,
    });
    return project;
  }

  async resolveProjectId({
    projectId,
    projectName,
    projectIndex,
    path,
    ensureProject = true,
  }) {
    const resolvedProjectId = ensureOptionalString(projectId);
    if (resolvedProjectId) {
      return resolvedProjectId;
    }
    const { project } = await this.resolveProject({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject,
    });
    return ensureString(project?.id, "projectId");
  }

  async listProjects() {
    const response = await this.requestJson("/api/v1/projects");
    return extractPayloadItems(response);
  }

  async getProject(projectId) {
    ensureString(projectId, "projectId");
    const response = await this.requestJson(`/api/v1/projects/${projectId}`);
    return extractPayloadItem(response);
  }

  async createProject({
    path,
    name,
    description = "",
    worktreeBasePath,
    hidePath,
  }) {
    ensureString(path, "path");
    ensureString(name, "name");
    const response = await this.requestJson("/api/v1/projects/create", {
      method: "POST",
      body: {
        path,
        name,
        description,
        worktreeBasePath,
        hidePath,
      },
    });
    return extractPayloadItem(response);
  }

  async getProjectPiTrust(input = {}) {
    const projectId = await this.resolveProjectId({
      projectId: input.projectId,
      projectName: input.projectName,
      projectIndex: input.projectIndex,
      path: input.path,
      ensureProject: input.ensureProject !== false,
    });
    const response = await this.requestJson(
      `/api/v1/projects/${projectId}/agent-trust/pi`,
    );
    return extractPayloadItem(response);
  }

  async trustProjectForPi(input = {}) {
    const projectId = await this.resolveProjectId({
      projectId: input.projectId,
      projectName: input.projectName,
      projectIndex: input.projectIndex,
      path: input.path,
      ensureProject: input.ensureProject !== false,
    });
    const response = await this.requestJson(
      `/api/v1/projects/${projectId}/agent-trust/pi`,
      { method: "POST" },
    );
    return extractPayloadItem(response);
  }

  async revokeProjectPiTrust(input = {}) {
    const projectId = await this.resolveProjectId({
      projectId: input.projectId,
      projectName: input.projectName,
      projectIndex: input.projectIndex,
      path: input.path,
      ensureProject: input.ensureProject !== false,
    });
    const response = await this.requestJson(
      `/api/v1/projects/${projectId}/agent-trust/pi`,
      { method: "DELETE" },
    );
    return extractPayloadItem(response);
  }

  async listWorktrees(projectId) {
    ensureString(projectId, "projectId");
    const response = await this.requestJson(
      `/api/v1/projects/${projectId}/worktrees`,
    );
    return extractPayloadItems(response);
  }

  async listTerminalSessions(projectId) {
    ensureString(projectId, "projectId");
    const response = await this.requestJson(
      `/api/v1/projects/${projectId}/terminals`,
    );
    return extractPayloadItems(response);
  }

  async listAISessionsByProject(projectId) {
    ensureString(projectId, "projectId");
    const response = await this.requestJson(
      `/api/v1/projects/${projectId}/ai-sessions`,
    );
    return extractPayloadItem(response);
  }

  async listAISessionsByPath(projectPath) {
    ensureString(projectPath, "path");
    const response = await this.requestJson("/api/v1/ai-sessions/by-path", {
      method: "POST",
      body: { path: projectPath },
    });
    return extractPayloadItem(response);
  }

  async getAISessionConversation({ id, sessionId, refresh = false }) {
    const dbId = ensureOptionalString(id);
    const rawSessionId = ensureOptionalString(sessionId);
    if (!dbId && !rawSessionId) {
      throw new CodeKanbanValidationError("id or sessionId is required");
    }
    if (refresh && !dbId) {
      throw new CodeKanbanValidationError(
        "refresh currently requires a database id",
      );
    }

    if (dbId) {
      const path = refresh
        ? `/api/v1/ai-sessions/${dbId}/refresh`
        : `/api/v1/ai-sessions/${dbId}/conversation`;
      const response = await this.requestJson(path, {
        method: refresh ? "POST" : "GET",
      });
      return extractPayloadItem(response);
    }

    const response = await this.requestJson(
      `/api/v1/ai-sessions/by-session-id/${rawSessionId}/conversation`,
    );
    return extractPayloadItem(response);
  }

  async getAISessionToolResult({ id, sessionId, toolUseId }) {
    const dbId = ensureOptionalString(id);
    const rawSessionId = ensureOptionalString(sessionId);
    const resolvedToolUseId = ensureString(toolUseId, "toolUseId");
    if (!dbId && !rawSessionId) {
      throw new CodeKanbanValidationError("id or sessionId is required");
    }
    const path = dbId
      ? `/api/v1/ai-sessions/${dbId}/conversation/tool-results/${resolvedToolUseId}`
      : `/api/v1/ai-sessions/by-session-id/${rawSessionId}/conversation/tool-results/${resolvedToolUseId}`;
    const response = await this.requestJson(path);
    return extractPayloadItem(response);
  }

  async createTerminalSession({
    projectId,
    worktreeId,
    workingDir = "",
    title = "",
    rows = 0,
    cols = 0,
  }) {
    ensureString(projectId, "projectId");
    ensureString(worktreeId, "worktreeId");
    const response = await this.requestJson(
      `/api/v1/projects/${projectId}/worktrees/${worktreeId}/terminals`,
      {
        method: "POST",
        body: {
          workingDir,
          title,
          rows,
          cols,
        },
      },
    );
    return extractPayloadItem(response);
  }

  async resolveProject({
    projectId,
    projectName,
    projectIndex,
    path,
    ensureProject = true,
  }) {
    const resolvedProjectId = ensureOptionalString(projectId);
    const resolvedProjectName = ensureOptionalString(projectName);
    const resolvedPath = ensureOptionalString(path);

    if (resolvedProjectId) {
      const project = await this.getProject(resolvedProjectId);
      return {
        project,
        matchedBy: "projectId",
      };
    }

    const projects =
      resolvedProjectName || resolvedPath ? await this.listProjects() : [];

    if (resolvedProjectName) {
      return resolveProjectByName(projects, resolvedProjectName, projectIndex);
    }

    if (!resolvedPath) {
      throw new CodeKanbanValidationError(
        "projectId, projectName, or path is required",
      );
    }

    const target = normalizeFsPath(resolvedPath);
    const project = projects.find(
      (item) => normalizeFsPath(item.path) === target,
    );
    if (project) {
      return {
        project,
        matchedBy: "path",
      };
    }

    if (!ensureProject) {
      throw new CodeKanbanValidationError(
        `no CodeKanban project is registered for path: ${resolvedPath}`,
      );
    }

    const created = await this.createProject({
      path: resolvedPath,
      name: pathBasename(resolvedPath),
      description: "",
    });
    return {
      project: created,
      matchedBy: "created",
    };
  }

  async resolveWorktree({ projectId, worktreeId }) {
    const project = ensureString(projectId, "projectId");
    const preferredWorktreeId = ensureOptionalString(worktreeId);
    const worktrees = await this.listWorktrees(project);
    if (worktrees.length === 0) {
      throw new CodeKanbanValidationError(
        `no worktrees are available for project ${project}`,
      );
    }
    if (preferredWorktreeId) {
      const direct = worktrees.find((item) => item.id === preferredWorktreeId);
      if (!direct) {
        throw new CodeKanbanValidationError(
          `worktree ${preferredWorktreeId} was not found in project ${project}`,
        );
      }
      return direct;
    }
    return worktrees.find((item) => item.isMain) || worktrees[0];
  }

  connectTerminal({ sessionId, wsPath, wsUrl }) {
    const resolvedSessionId = ensureString(sessionId, "sessionId");
    const resolvedPath =
      ensureOptionalString(wsUrl) ||
      ensureOptionalString(wsPath) ||
      `/api/v1/terminal/ws?sessionId=${resolvedSessionId}`;
    const url =
      resolvedPath.startsWith("ws://") || resolvedPath.startsWith("wss://")
        ? resolvedPath
        : toWsUrl(this.baseURL, resolvedPath);

    return new TerminalConnection({
      sessionId: resolvedSessionId,
      url,
      WebSocketImpl: this.WebSocketImpl,
      webSocketOptions: this.webSocketOptions,
    });
  }

  openWebSessionCommandChannel() {
    return new WebSessionCommandChannel({
      url: toWsUrl(this.baseURL, WEB_SESSION_COMMAND_WS_PATH),
      WebSocketImpl: this.WebSocketImpl,
      webSocketOptions: this.webSocketOptions,
    });
  }

  openWebSessionEventStream(options = {}) {
    return new WebSessionEventStream({
      url: toWsUrl(this.baseURL, WEB_SESSION_EVENTS_WS_PATH),
      sessionId: ensureOptionalString(options.sessionId),
      WebSocketImpl: this.WebSocketImpl,
      webSocketOptions: this.webSocketOptions,
    });
  }

  async withWebSessionCommandChannel(handler) {
    const channel = this.openWebSessionCommandChannel();
    try {
      await channel.waitForOpen();
      return await handler(channel);
    } finally {
      channel.close();
    }
  }

  async listSessions({
    projectId,
    projectName,
    projectIndex,
    path,
    includeTerminal = true,
    includeAI = true,
    ensureProject = true,
  }) {
    const { project, matchedBy } = await this.resolveProject({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject,
    });

    const [terminalSessions, aiSessions] = await Promise.all([
      includeTerminal
        ? this.listTerminalSessions(project.id)
        : Promise.resolve([]),
      includeAI
        ? this.listAISessionsByProject(project.id)
        : Promise.resolve({
            hasClaudeCode: false,
            hasCodex: false,
            hasPi: false,
            claudeSessions: [],
            codexSessions: [],
            piSessions: [],
          }),
    ]);

    return {
      project,
      matchedBy,
      terminalSessions,
      aiSessions,
    };
  }

  async listProjectFileScopes({ projectId, projectName, projectIndex, path, ensureProject = true } = {}) {
    const resolvedProjectId = await this.resolveProjectId({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject,
    });
    const response = await this.requestJson(
      `/api/v1/projects/${resolvedProjectId}/files/scopes`,
    );
    return extractPayloadItems(response);
  }

  async readProjectFileText({
    projectId,
    projectName,
    projectIndex,
    path,
    scopeId,
    filePath,
    ensureProject = true,
  } = {}) {
    const resolvedProjectId = await this.resolveProjectId({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject,
    });
    const params = new URLSearchParams();
    if (ensureOptionalString(scopeId)) {
      params.set("scopeId", ensureOptionalString(scopeId));
    }
    params.set("path", ensureString(filePath, "filePath"));
    params.set("disposition", "inline");
    const { text, contentType } = await this.requestText(
      `/api/v1/projects/${resolvedProjectId}/files/content?${params.toString()}`,
    );
    return {
      projectId: resolvedProjectId,
      scopeId: ensureOptionalString(scopeId) || null,
      path: ensureString(filePath, "filePath"),
      contentType,
      text,
    };
  }

  async deleteProjectFiles({
    projectId,
    projectName,
    projectIndex,
    path,
    scopeId,
    paths,
    ensureProject = true,
  } = {}) {
    const resolvedProjectId = await this.resolveProjectId({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject,
    });
    const normalizedPaths = ensureArrayOfStrings(paths, "paths");
    if (normalizedPaths.length === 0) {
      throw new CodeKanbanValidationError("paths is required");
    }
    const response = await this.requestJson(
      `/api/v1/projects/${resolvedProjectId}/files/delete`,
      {
        method: "POST",
        body: {
          ...(ensureOptionalString(scopeId)
            ? { scopeId: ensureOptionalString(scopeId) }
            : {}),
          paths: normalizedPaths,
        },
      },
    );
    return extractPayloadItem(response);
  }

  async listWebSessions({ projectId, projectName, projectIndex, path, ensureProject = true } = {}) {
    const resolvedProjectId = await this.resolveProjectId({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject,
    });
    const response = await this.requestJson(
      `/api/v1/projects/${resolvedProjectId}/web-sessions`,
    );
    return extractPayloadItems(response);
  }

  async listWebSessionImportSources({ projectId, projectName, projectIndex, path, refresh = false } = {}) {
    const resolvedProjectId = await this.resolveProjectId({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject: true,
    });
    const query = refresh === true ? "?refresh=true" : "";
    const response = await this.requestJson(
      `/api/v1/projects/${resolvedProjectId}/web-sessions/import-sources${query}`,
    );
    return extractPayloadItem(response) || { items: [], scanPhase: "complete" };
  }

  async importWebSession(input = {}) {
    const projectId = await this.resolveProjectId({
      projectId: input.projectId,
      projectName: input.projectName,
      projectIndex: input.projectIndex,
      path: input.path,
      ensureProject: true,
    });
    const agent = ensureOptionalString(input.agent) || "codex";
    if (agent !== "codex" && agent !== "pi") {
      throw new CodeKanbanValidationError("agent must be codex or pi");
    }
    const sessionId = ensureOptionalString(input.sessionId);
    const aiSessionId = ensureOptionalString(input.aiSessionId);
    if (!sessionId && !aiSessionId) {
      throw new CodeKanbanValidationError("sessionId or aiSessionId is required");
    }
    const mode = ensureOptionalString(input.mode);
    if (mode && mode !== "fast" && mode !== "deep") {
      throw new CodeKanbanValidationError("mode must be fast or deep");
    }
    const response = await this.requestJson(
      `/api/v1/projects/${projectId}/web-sessions/import`,
      {
        method: "POST",
        body: {
          agent,
          ...(sessionId ? { sessionId } : {}),
          ...(aiSessionId ? { aiSessionId } : {}),
          ...(mode ? { mode } : {}),
        },
      },
    );
    return extractPayloadItem(response);
  }

  async createWebSession(input = {}) {
    const projectId = await this.resolveProjectId({
      projectId: input.projectId,
      projectName: input.projectName,
      projectIndex: input.projectIndex,
      path: input.path,
      ensureProject: true,
    });

    const agent = ensureString(input.agent, "agent");
    if (!AGENTS.includes(agent)) {
      throw new CodeKanbanValidationError(`agent must be one of: ${AGENTS.join(", ")}`);
    }
    const permissionMode = ensureOptionalString(
      input.permissionMode,
    ).toLowerCase();
    const model = ensureOptionalString(input.model);
    const reasoningEffort = ensureOptionalString(input.reasoningEffort);
    const permissionLevel = ensureOptionalString(input.permissionLevel);
    const autoRetryPolicyMode = normalizeWebSessionAutoRetryPolicyMode(
      input.autoRetryPolicyMode,
    );
    const autoRetryScope = ensureOptionalString(input.autoRetryScope);
    const autoRetryPreset = ensureOptionalString(input.autoRetryPreset);
    const autoRetryMaxAttempts = normalizeWebSessionAutoRetryMaxAttempts(
      input.autoRetryMaxAttempts,
    );
    const worktree = await this.resolveWorktree({
      projectId,
      worktreeId: input.worktreeId,
    });

    const response = await this.requestJson(
      `/api/v1/projects/${projectId}/web-sessions`,
      {
        method: "POST",
        body: {
          worktreeId: ensureString(worktree?.id, "worktreeId"),
          agent,
          claudeRuntime: ensureOptionalString(input.claudeRuntime) || "claude",
          ...(model ? { model } : {}),
          ...(reasoningEffort ? { reasoningEffort } : {}),
          workflowMode:
            ensureOptionalString(input.workflowMode) ||
            defaultWebSessionWorkflowMode(permissionMode),
          ...(permissionLevel ? { permissionLevel } : {}),
          autoRetryEnabled: input.autoRetryEnabled === true,
          ...(autoRetryPolicyMode ? { autoRetryPolicyMode } : {}),
          ...(autoRetryScope
            ? { autoRetryScope: normalizeWebSessionAutoRetryScope(autoRetryScope) }
            : {}),
          ...(autoRetryPreset
            ? { autoRetryPreset: normalizeWebSessionAutoRetryPreset(autoRetryPreset) }
            : {}),
          ...(autoRetryMaxAttempts !== undefined
            ? { autoRetryMaxAttempts }
            : {}),
          ...(typeof input.autoRetryDispatchPendingOnFailure === "boolean"
            ? {
                autoRetryDispatchPendingOnFailure:
                  input.autoRetryDispatchPendingOnFailure,
              }
            : {}),
          permissionMode,
          title: ensureOptionalString(input.title),
        },
      },
    );
    return extractPayloadItem(response);
  }

  async getWebSessionSnapshot({ projectId, projectName, projectIndex, path, sessionId, limit = 80 }) {
    const resolvedProjectId = await this.resolveProjectId({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject: true,
    });
    const resolvedSessionId = ensureString(sessionId, "sessionId");
    const normalizedLimit = Number.isFinite(limit)
      ? Math.max(1, Math.trunc(limit))
      : 80;
    const response = await this.requestJson(
      `/api/v1/projects/${resolvedProjectId}/web-sessions/${resolvedSessionId}/snapshot?limit=${normalizedLimit}`,
    );
    return extractPayloadItem(response);
  }

  async getWebSessionTree({ projectId, projectName, projectIndex, path, sessionId }) {
    const resolvedProjectId = await this.resolveProjectId({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject: true,
    });
    const resolvedSessionId = ensureString(sessionId, "sessionId");
    const response = await this.requestJson(
      `/api/v1/projects/${resolvedProjectId}/web-sessions/${resolvedSessionId}/tree`,
    );
    return normalizePiTreeSnapshot(extractPayloadItem(response));
  }

  async navigateWebSessionTree({
    projectId,
    projectName,
    projectIndex,
    path,
    sessionId,
    targetId,
    revision,
    summarize = false,
  }) {
    const resolvedProjectId = await this.resolveProjectId({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject: true,
    });
    const resolvedSessionId = ensureString(sessionId, "sessionId");
    const response = await this.requestJson(
      `/api/v1/projects/${resolvedProjectId}/web-sessions/${resolvedSessionId}/tree/navigate`,
      {
        method: "POST",
        body: {
          targetId: ensureString(targetId, "targetId"),
          revision: ensureString(revision, "revision"),
          summarize: summarize === true,
        },
      },
    );
    return normalizePiTreeNavigateResult(extractPayloadItem(response));
  }

  async forkWebSessionTree({
    projectId,
    projectName,
    projectIndex,
    path,
    sessionId,
    targetId,
    revision,
  }) {
    const resolvedProjectId = await this.resolveProjectId({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject: true,
    });
    const resolvedSessionId = ensureString(sessionId, "sessionId");
    const response = await this.requestJson(
      `/api/v1/projects/${resolvedProjectId}/web-sessions/${resolvedSessionId}/tree/fork`,
      {
        method: "POST",
        body: {
          targetId: ensureString(targetId, "targetId"),
          revision: ensureString(revision, "revision"),
        },
      },
    );
    return normalizePiTreeCreateResult(extractPayloadItem(response));
  }

  async cloneWebSessionTree({
    projectId,
    projectName,
    projectIndex,
    path,
    sessionId,
    revision,
  }) {
    const resolvedProjectId = await this.resolveProjectId({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject: true,
    });
    const resolvedSessionId = ensureString(sessionId, "sessionId");
    const response = await this.requestJson(
      `/api/v1/projects/${resolvedProjectId}/web-sessions/${resolvedSessionId}/tree/clone`,
      {
        method: "POST",
        body: {
          revision: ensureString(revision, "revision"),
        },
      },
    );
    return normalizePiTreeCreateResult(extractPayloadItem(response));
  }

  async getWebSessionHistory({
    projectId,
    projectName,
    projectIndex,
    path,
    sessionId,
    beforeCursor,
    limit = 80,
  }) {
    const resolvedProjectId = await this.resolveProjectId({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject: true,
    });
    const resolvedSessionId = ensureString(sessionId, "sessionId");
    const params = new URLSearchParams();
    if (ensureOptionalString(beforeCursor)) {
      params.set("beforeCursor", ensureOptionalString(beforeCursor));
    }
    if (Number.isFinite(limit)) {
      params.set("limit", String(Math.max(1, Math.trunc(limit))));
    }
    const suffix = params.toString();
    const response = await this.requestJson(
      `/api/v1/projects/${resolvedProjectId}/web-sessions/${resolvedSessionId}/history${suffix ? `?${suffix}` : ""}`,
    );
    return extractPayloadItem(response);
  }

  async syncWebSession({
    projectId,
    projectName,
    projectIndex,
    path,
    sessionId,
    mode,
    clearExisting = false,
  }) {
    const resolvedProjectId = await this.resolveProjectId({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject: true,
    });
    const resolvedSessionId = ensureString(sessionId, "sessionId");
    const response = await this.requestJson(
      `/api/v1/projects/${resolvedProjectId}/web-sessions/${resolvedSessionId}/sync`,
      {
        method: "POST",
        body: {
          ...(ensureOptionalString(mode)
            ? { mode: ensureOptionalString(mode) }
            : {}),
          clearExisting: clearExisting === true,
        },
      },
    );
    return extractPayloadItem(response);
  }

  async archiveWebSession({ projectId, projectName, projectIndex, path, sessionId }) {
    const resolvedProjectId = await this.resolveProjectId({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject: true,
    });
    const resolvedSessionId = ensureString(sessionId, "sessionId");
    const response = await this.requestJson(
      `/api/v1/projects/${resolvedProjectId}/web-sessions/${resolvedSessionId}/archive`,
      {
        method: "POST",
      },
    );
    return extractPayloadItem(response);
  }

  async unarchiveWebSession({ projectId, projectName, projectIndex, path, sessionId }) {
    const resolvedProjectId = await this.resolveProjectId({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject: true,
    });
    const resolvedSessionId = ensureString(sessionId, "sessionId");
    const response = await this.requestJson(
      `/api/v1/projects/${resolvedProjectId}/web-sessions/${resolvedSessionId}/unarchive`,
      {
        method: "POST",
      },
    );
    return extractPayloadItem(response);
  }

  async renameWebSession({ projectId, projectName, projectIndex, path, sessionId, title }) {
    const resolvedProjectId = await this.resolveProjectId({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject: true,
    });
    const resolvedSessionId = ensureString(sessionId, "sessionId");
    const response = await this.requestJson(
      `/api/v1/projects/${resolvedProjectId}/web-sessions/${resolvedSessionId}/rename`,
      {
        method: "POST",
        body: {
          title: ensureString(title, "title"),
        },
      },
    );
    return extractPayloadItem(response);
  }

  async closeWebSession({ projectId, projectName, projectIndex, path, sessionId }) {
    const resolvedProjectId = await this.resolveProjectId({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject: true,
    });
    const resolvedSessionId = ensureString(sessionId, "sessionId");
    const response = await this.requestJson(
      `/api/v1/projects/${resolvedProjectId}/web-sessions/${resolvedSessionId}/close`,
      {
        method: "POST",
      },
    );
    return {
      message: extractPayloadMessage(response) || "session aborted",
    };
  }

  async deleteWebSession({ projectId, projectName, projectIndex, path, sessionId }) {
    const resolvedProjectId = await this.resolveProjectId({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject: true,
    });
    const resolvedSessionId = ensureString(sessionId, "sessionId");
    const response = await this.requestJson(
      `/api/v1/projects/${resolvedProjectId}/web-sessions/${resolvedSessionId}`,
      {
        method: "DELETE",
      },
    );
    return {
      message: extractPayloadMessage(response) || "session deleted",
    };
  }

  async queryArchivedWebSessions({ projectIds, offset = 0, limit = 100 }) {
    const response = await this.requestJson(
      "/api/v1/web-sessions/query",
      {
        method: "POST",
        body: {
          projectIds: Array.isArray(projectIds) ? projectIds : [],
          archived: true,
          offset: Number.isFinite(offset) ? Math.max(0, Math.trunc(offset)) : 0,
          limit: Number.isFinite(limit) ? Math.max(1, Math.trunc(limit)) : 100,
        },
      },
    );
    return (
      extractPayloadItem(response) || { items: [], total: 0, hasMore: false, nextOffset: 0 }
    );
  }

  async searchWebSessions({
    projectIds,
    query,
    includeArchived = false,
    includeBody = true,
    cursor = "",
    scanLimit = 50,
  }) {
    const response = await this.requestJson("/api/v1/web-sessions/search", {
      method: "POST",
      body: {
        projectIds: Array.isArray(projectIds) ? projectIds : [],
        query: ensureString(query, "query"),
        includeArchived: includeArchived === true,
        includeBody: includeBody !== false,
        cursor: typeof cursor === "string" ? cursor : "",
        scanLimit: Number.isFinite(scanLimit)
          ? Math.max(1, Math.min(100, Math.trunc(scanLimit)))
          : 50,
      },
    });
    return (
      extractPayloadItem(response) || {
        items: [],
        nextCursor: "",
        done: true,
        scanned: 0,
        total: 0,
      }
    );
  }

  async getWebSessionCommandGroup({ projectId, projectName, projectIndex, path, sessionId, groupId }) {
    const resolvedProjectId = await this.resolveProjectId({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject: true,
    });
    const resolvedSessionId = ensureString(sessionId, "sessionId");
    const resolvedGroupId = ensureString(groupId, "groupId");
    const response = await this.requestJson(
      `/api/v1/projects/${resolvedProjectId}/web-sessions/${resolvedSessionId}/command-groups/${resolvedGroupId}`,
    );
    return extractPayloadItem(response);
  }

  async getWebSessionRuntimeConfig() {
    const response = await this.requestJson(
      "/api/v1/web-sessions/runtime-config",
    );
    return normalizeWebSessionRuntimeConfig(extractPayloadItem(response));
  }

  async uploadWebSessionAttachment({
    projectId,
    projectName,
    projectIndex,
    path,
    filePath,
    fileName,
    mimeType,
  }) {
    const resolvedProjectId = await this.resolveProjectId({
      projectId,
      projectName,
      projectIndex,
      path,
      ensureProject: true,
    });
    const resolvedFilePath = ensureString(filePath, "filePath");
    const resolvedFileName =
      ensureOptionalString(fileName) || pathBasename(resolvedFilePath);
    const resolvedMimeType = ensureImageMimeType(mimeType, resolvedFileName);
    const fileBuffer = await readFile(resolvedFilePath);
    const formData = new FormData();
    formData.append(
      "file",
      new File([fileBuffer], resolvedFileName, { type: resolvedMimeType }),
    );

    const headers = {
      Accept: "application/json",
      ...this.headers,
    };
    delete headers["Content-Type"];

    const response = await this.fetchImpl(
      new URL(
        `/api/v1/projects/${resolvedProjectId}/web-sessions/attachments`,
        this.baseURL,
      ),
      {
        method: "POST",
        headers,
        body: formData,
      },
    );
    const text = await response.text();
    const body = text ? JSON.parse(text) : null;
    if (!response.ok) {
      throw new CodeKanbanHttpError(`request failed with ${response.status}`, {
        status: response.status,
        method: "POST",
        path: `/api/v1/projects/${resolvedProjectId}/web-sessions/attachments`,
        body,
      });
    }
    return normalizeWebSessionAttachment(extractPayloadItem(body));
  }

  analyzeWebSession(snapshot) {
    return analyzeWebSession(snapshot);
  }

  async getWebSessionState({ projectId, projectName, projectIndex, path, sessionId, limit = 120 }) {
    const snapshot = await this.getWebSessionSnapshot({
      projectId,
      projectName,
      projectIndex,
      path,
      sessionId,
      limit,
    });
    return this.analyzeWebSession(snapshot);
  }

  async sendWebSessionMessage({ sessionId, text, attachmentIds = [], mode }) {
    return await this.withWebSessionCommandChannel((channel) =>
      channel.sendMessage(sessionId, {
        text,
        attachmentIds,
        mode: ensureOptionalString(mode),
      }),
    );
  }

  async compactWebSession({ sessionId }) {
    return await this.withWebSessionCommandChannel((channel) =>
      channel.compact(sessionId),
    );
  }

  async removeWebSessionPendingInput({ sessionId, pendingId }) {
    return await this.withWebSessionCommandChannel((channel) =>
      channel.removePendingInput(sessionId, {
        pendingId,
      }),
    );
  }

  async updateWebSessionPendingInput({ sessionId, pendingId, text, paused }) {
    return await this.withWebSessionCommandChannel((channel) =>
      channel.updatePendingInput(sessionId, {
        pendingId,
        ...(text !== undefined ? { text } : {}),
        ...(typeof paused === "boolean" ? { paused } : {}),
      }),
    );
  }

  async updateWebSessionWorkflowMode({ sessionId, workflowMode }) {
    return await this.withWebSessionCommandChannel((channel) =>
      channel.updateWorkflowMode(sessionId, {
        workflowMode,
      }),
    );
  }

  async answerWebSessionUserInput({ sessionId, itemId, answers }) {
    return await this.withWebSessionCommandChannel((channel) =>
      channel.answerUserInput(sessionId, {
        itemId,
        answers,
      }),
    );
  }

  async approveWebSession({ sessionId }) {
    return await this.withWebSessionCommandChannel((channel) =>
      channel.approve(sessionId),
    );
  }

  async rejectWebSession({ sessionId }) {
    return await this.withWebSessionCommandChannel((channel) =>
      channel.reject(sessionId),
    );
  }

  async getWebSessionGoal({ sessionId }) {
    return await this.withWebSessionCommandChannel((channel) =>
      channel.getGoal(sessionId),
    );
  }

  async setWebSessionGoal({ sessionId, objective, status }) {
    return await this.withWebSessionCommandChannel((channel) =>
      channel.setGoal(sessionId, {
        objective,
        status: ensureOptionalString(status),
      }),
    );
  }

  async bootstrapWebSessionGoal({ sessionId, objective, status }) {
    return await this.withWebSessionCommandChannel((channel) =>
      channel.bootstrapGoal(sessionId, {
        objective,
        status: ensureOptionalString(status),
      }),
    );
  }

  async pauseWebSessionGoal({ sessionId }) {
    return await this.withWebSessionCommandChannel((channel) =>
      channel.pauseGoal(sessionId),
    );
  }

  async resumeWebSessionGoal({ sessionId }) {
    return await this.withWebSessionCommandChannel((channel) =>
      channel.resumeGoal(sessionId),
    );
  }

  async clearWebSessionGoal({ sessionId }) {
    return await this.withWebSessionCommandChannel((channel) =>
      channel.clearGoal(sessionId),
    );
  }

  async answerPendingUserInput({
    projectId,
    projectName,
    projectIndex,
    path,
    sessionId,
    answers,
    limit = 120,
  }) {
    const state = await this.getWebSessionState({
      projectId,
      projectName,
      projectIndex,
      path,
      sessionId,
      limit,
    });
    if (!state.pendingUserInput?.itemId) {
      throw new CodeKanbanValidationError(
        `web session ${sessionId} has no pending user input`,
      );
    }
    const ack = await this.answerWebSessionUserInput({
      sessionId,
      itemId: state.pendingUserInput.itemId,
      answers,
    });
    return {
      sessionId,
      itemId: state.pendingUserInput.itemId,
      prompt: state.pendingUserInput.prompt,
      ack,
      state,
    };
  }

  async approvePending({ projectId, projectName, projectIndex, path, sessionId, limit = 120 }) {
    const state = await this.getWebSessionState({
      projectId,
      projectName,
      projectIndex,
      path,
      sessionId,
      limit,
    });
    if (!state.pendingApproval) {
      throw new CodeKanbanValidationError(
        `web session ${sessionId} has no pending approval`,
      );
    }
    const ack = await this.approveWebSession({ sessionId });
    return {
      sessionId,
      prompt: state.pendingApproval.prompt,
      ack,
      state,
    };
  }

  async rejectPending({ projectId, projectName, projectIndex, path, sessionId, limit = 120 }) {
    const state = await this.getWebSessionState({
      projectId,
      projectName,
      projectIndex,
      path,
      sessionId,
      limit,
    });
    if (!state.pendingApproval) {
      throw new CodeKanbanValidationError(
        `web session ${sessionId} has no pending approval`,
      );
    }
    const ack = await this.rejectWebSession({ sessionId });
    return {
      sessionId,
      prompt: state.pendingApproval.prompt,
      ack,
      state,
    };
  }

  async executeLatestPlan({
    projectId,
    projectName,
    projectIndex,
    path,
    sessionId,
    prompt = "Implement the plan.",
    limit = 120,
  }) {
    const state = await this.getWebSessionState({
      projectId,
      projectName,
      projectIndex,
      path,
      sessionId,
      limit,
    });
    if (!state.latestPlan) {
      throw new CodeKanbanValidationError(
        `web session ${sessionId} has no latest plan to execute`,
      );
    }
    if (!state.canSend && state.nextAction?.type !== "execute_plan") {
      throw new CodeKanbanValidationError(
        `web session ${sessionId} is not ready to execute the latest plan`,
      );
    }

    return await this.withWebSessionCommandChannel(async (channel) => {
      if (state.session?.workflowMode === "plan") {
        await channel.updateWorkflowMode(sessionId, {
          workflowMode: "default",
        });
      }

      if (
        state.pendingUserInput?.isPlanChoice &&
        state.pendingUserInput.itemId &&
        state.pendingUserInput.questionId &&
        state.pendingUserInput.executeOptionLabel
      ) {
        const ack = await channel.answerUserInput(sessionId, {
          itemId: state.pendingUserInput.itemId,
          answers: {
            [state.pendingUserInput.questionId]: [
              state.pendingUserInput.executeOptionLabel,
            ],
          },
        });
        return {
          sessionId,
          mode: "plan_choice",
          latestPlan: state.latestPlan,
          ack,
          state,
        };
      }

      const ack = await channel.sendMessage(sessionId, {
        text: prompt,
        attachmentIds: [],
      });
      return {
        sessionId,
        mode: "followup_message",
        prompt,
        latestPlan: state.latestPlan,
        ack,
        state,
      };
    });
  }

  async waitForWebSessionState({
    projectId,
    projectName,
    projectIndex,
    path,
    sessionId,
    until,
    intervalMs = 5000,
    timeoutMs = 60000,
    limit = 120,
    settleMs = 0,
  }) {
    const matches = normalizeWebSessionStateMatcher(until);
    const normalizedIntervalMs = Math.max(1, Math.trunc(intervalMs));
    const normalizedSettleMs = Number.isFinite(settleMs)
      ? Math.max(0, Math.trunc(settleMs))
      : 0;

    const startedAt = Date.now();
    let lastRetryableError = null;
    let matchedAt = null;
    while (Date.now() - startedAt <= timeoutMs) {
      try {
        const state = await this.getWebSessionState({
          projectId,
          projectName,
          projectIndex,
          path,
          sessionId,
          limit,
        });
        lastRetryableError = null;
        if (matches(state)) {
          if (normalizedSettleMs <= 0) {
            return state;
          }
          if (matchedAt == null) {
            matchedAt = Date.now();
          }
          if (Date.now() - matchedAt >= normalizedSettleMs) {
            return state;
          }
        } else {
          matchedAt = null;
        }
      } catch (error) {
        if (!isRetryableRequestError(error)) {
          throw error;
        }
        lastRetryableError = error;
        matchedAt = null;
      }

      const remainingSettleMs =
        matchedAt == null || normalizedSettleMs <= 0
          ? normalizedIntervalMs
          : Math.max(1, normalizedSettleMs - (Date.now() - matchedAt));
      await sleep(Math.min(normalizedIntervalMs, remainingSettleMs));
    }

    if (lastRetryableError) {
      throw new CodeKanbanValidationError(
        `web session ${sessionId} did not reach the requested state within ${timeoutMs}ms (last transient error: ${lastRetryableError.message})`,
        { cause: lastRetryableError, reason: "timeout" },
      );
    }

    throw new CodeKanbanValidationError(
      `web session ${sessionId} did not reach the requested state within ${timeoutMs}ms`,
      { reason: "timeout" },
    );
  }

  async waitForWebSessionPause({
    projectId,
    projectName,
    projectIndex,
    path,
    sessionId,
    until,
    intervalMs = 5000,
    timeoutMs = 60000,
    limit = 120,
    settleMs = 0,
  }) {
    const untilMatcher = until ? normalizeWebSessionStateMatcher(until) : null;
    const normalizedIntervalMs = Math.max(1, Math.trunc(intervalMs));
    const normalizedSettleMs = Number.isFinite(settleMs)
      ? Math.max(0, Math.trunc(settleMs))
      : 0;

    const startedAt = Date.now();
    let lastRetryableError = null;
    let settledReason = null;
    let settledAt = null;
    while (Date.now() - startedAt <= timeoutMs) {
      try {
        const state = await this.getWebSessionState({
          projectId,
          projectName,
          projectIndex,
          path,
          sessionId,
          limit,
        });
        lastRetryableError = null;
        const reason = getWebSessionPauseReason(state, untilMatcher);
        if (reason) {
          if (
            normalizedSettleMs > 0 &&
            isDebouncedPauseReason(reason)
          ) {
            if (settledReason !== reason) {
              settledReason = reason;
              settledAt = Date.now();
            }
            if (Date.now() - settledAt >= normalizedSettleMs) {
              return { reason, state };
            }
          } else {
            return { reason, state };
          }
        } else {
          settledReason = null;
          settledAt = null;
        }
      } catch (error) {
        if (!isRetryableRequestError(error)) {
          throw error;
        }
        lastRetryableError = error;
        settledReason = null;
        settledAt = null;
      }

      const remainingSettleMs =
        settledAt == null || normalizedSettleMs <= 0
          ? normalizedIntervalMs
          : Math.max(1, normalizedSettleMs - (Date.now() - settledAt));
      await sleep(Math.min(normalizedIntervalMs, remainingSettleMs));
    }

    if (lastRetryableError) {
      throw new CodeKanbanValidationError(
        `web session ${sessionId} did not reach a pause state within ${timeoutMs}ms (last transient error: ${lastRetryableError.message})`,
        { cause: lastRetryableError, reason: "timeout" },
      );
    }

    throw new CodeKanbanValidationError(
      `web session ${sessionId} did not reach a pause state within ${timeoutMs}ms`,
      { reason: "timeout" },
    );
  }

  async runWebSessionUntilDone({
    projectId,
    projectName,
    projectIndex,
    path,
    sessionId,
    until,
    intervalMs = 2000,
    timeoutMs = 120000,
    limit = 120,
    settleMs = 2000,
    terminalDebounceMs,
    answerStrategy = "prefer-second-or-text",
    autoExecutePlan = true,
    executePlanPrompt = "Implement the plan.",
  }) {
    const normalizedTerminalDebounceMs = Number.isFinite(terminalDebounceMs)
      ? Math.max(0, Math.trunc(terminalDebounceMs))
      : Number.isFinite(settleMs)
        ? Math.max(0, Math.trunc(settleMs))
        : 2000;
    const actions = [];
    const answeredItemIds = new Set();
    const executedPlanIds = new Set();
    let lastExecuteMode = null;
    let lastState = null;
    const startedAt = Date.now();

    while (Date.now() - startedAt <= timeoutMs) {
      const remainingTimeoutMs = Math.max(1, timeoutMs - (Date.now() - startedAt));
      let pause = null;
      try {
        pause = await this.waitForWebSessionPause({
          projectId,
          projectName,
          projectIndex,
          path,
          sessionId,
          until,
          intervalMs,
          timeoutMs: remainingTimeoutMs,
          limit,
          settleMs: normalizedTerminalDebounceMs,
        });
      } catch (error) {
        if (error?.reason !== "timeout") {
          throw error;
        }
        try {
          lastState = await this.getWebSessionState({
            projectId,
            projectName,
            projectIndex,
            path,
            sessionId,
            limit,
          });
        } catch {
          // Ignore a best-effort state refresh on timeout.
        }
        return {
          stopReason: "timeout",
          finalState: lastState,
          actions,
          lastExecuteMode,
        };
      }

      lastState = pause.state;
      if (pause.reason === "done" || pause.reason === "error" || pause.reason === "until") {
        return {
          stopReason: pause.reason,
          finalState: pause.state,
          actions,
          lastExecuteMode,
        };
      }
      if (pause.reason === "approval") {
        return {
          stopReason: "needs_approval",
          finalState: pause.state,
          actions,
          lastExecuteMode,
        };
      }
      if (pause.reason === "user_input") {
        const itemId = pause.state.pendingUserInput?.itemId || null;
        if (!itemId || answeredItemIds.has(itemId) || !answerStrategy) {
          return {
            stopReason: "needs_user_input",
            finalState: pause.state,
            actions,
            lastExecuteMode,
          };
        }
        const answers = buildAutoWebSessionAnswers(
          pause.state.pendingUserInput?.questions,
          answerStrategy,
        );
        await this.answerPendingUserInput({
          projectId,
          projectName,
          projectIndex,
          path,
          sessionId,
          answers,
          limit,
        });
        answeredItemIds.add(itemId);
        actions.push({
          type: "answer_user_input",
          at: new Date().toISOString(),
          itemId,
          answers,
        });
        continue;
      }
      if (pause.reason === "execute_plan") {
        const planId = pause.state.latestPlan?.itemId || pause.state.latestPlan?.id || null;
        if (!autoExecutePlan || (planId && executedPlanIds.has(planId))) {
          return {
            stopReason: "needs_execute_plan",
            finalState: pause.state,
            actions,
            lastExecuteMode,
          };
        }
        const result = await this.executeLatestPlan({
          projectId,
          projectName,
          projectIndex,
          path,
          sessionId,
          prompt: executePlanPrompt,
          limit,
        });
        if (planId) {
          executedPlanIds.add(planId);
        }
        lastExecuteMode = result.mode;
        actions.push({
          type: "execute_plan",
          at: new Date().toISOString(),
          itemId: planId,
          mode: result.mode,
        });
      }
    }

    return {
      stopReason: "timeout",
      finalState: lastState,
      actions,
      lastExecuteMode,
    };
  }

  async startWorkflow(input = {}) {
    const launch = buildAgentLaunchSpec(input);
    const { project, matchedBy } = await this.resolveProject({
      projectId: input.projectId,
      projectName: input.projectName,
      projectIndex: input.projectIndex,
      path: input.path,
      ensureProject: true,
    });
    const worktree = await this.resolveWorktree({
      projectId: project.id,
      worktreeId: input.worktreeId,
    });

    const terminal = await this.createTerminalSession({
      projectId: project.id,
      worktreeId: worktree.id,
      workingDir: ensureOptionalString(input.workingDir) || worktree.path,
      title:
        ensureOptionalString(input.title) ||
        ensureOptionalString(input.prompt) ||
        "AI workflow",
      rows: Number.isFinite(input.rows) ? input.rows : 0,
      cols: Number.isFinite(input.cols) ? input.cols : 0,
    });

    const connection = this.connectTerminal({
      sessionId: terminal.id,
      wsPath: terminal.wsPath,
      wsUrl: terminal.wsUrl,
    });

    await connection.waitForReady();
    connection.sendInput(normalizeTerminalEnter(launch.command));
    await sleep(500);
    connection.sendInput(normalizeTerminalEnter(launch.prompt));
    return {
      project,
      matchedBy,
      worktree,
      terminalSession: terminal,
      agent: launch.agent,
      claudeRuntime: launch.claudeRuntime,
      profile: launch.profile,
      command: launch.command,
      prompt: launch.prompt,
      promptAccepted: true,
      connection,
    };
  }

  async continueTerminalSession({ projectId, projectName, projectIndex, path, sessionId, prompt }) {
    const resolvedSessionId = ensureString(sessionId, "sessionId");
    const resolvedPrompt = ensureString(prompt, "prompt");

    let project;
    if (projectId || projectName || path) {
      ({ project } = await this.resolveProject({
        projectId,
        projectName,
        projectIndex,
        path,
        ensureProject: true,
      }));
      const sessions = await this.listTerminalSessions(project.id);
      const exists = sessions.some((item) => item.id === resolvedSessionId);
      if (!exists) {
        throw new CodeKanbanValidationError(
          `terminal session ${resolvedSessionId} does not belong to project ${project.id}`,
        );
      }
    }

    const connection = this.connectTerminal({ sessionId: resolvedSessionId });
    await connection.waitForReady();
    connection.sendInput(normalizeTerminalEnter(resolvedPrompt));
    return {
      project,
      sessionId: resolvedSessionId,
      prompt: resolvedPrompt,
      promptAccepted: true,
      connection,
    };
  }
}
