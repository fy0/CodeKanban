import {
  CodeKanbanConfigError,
  CodeKanbanError,
  CodeKanbanValidationError,
} from "./errors.js";
import {
  WEB_SESSION_PROTOCOL_VERSION,
  buildWebSessionCommandFrame,
  buildWebSessionHeartbeatFrame,
  decodeWebSessionSocketMessage,
  isWebSessionHeartbeatFrame,
  normalizeWebSessionFrame,
} from "./web-session-shared.js";
import {
  ensureArrayOfStrings,
  ensureOptionalString,
  ensureString,
} from "./utils.js";

const SOCKET_OPEN = 1;
const DEFAULT_OPEN_TIMEOUT_MS = 8000;
const ACK_SETTLE_DELAY_MS = 10;

function createRequestId() {
  return `web_ws_${Date.now()}_${Math.random().toString(36).slice(2, 10)}`;
}

function normalizeHistoryLimit(value) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return 80;
  }
  return Math.max(1, Math.trunc(parsed));
}

export class WebSessionCommandChannel {
  constructor({ url, WebSocketImpl, webSocketOptions } = {}) {
    const resolvedUrl = ensureString(url, "url");
    const Socket = WebSocketImpl || globalThis.WebSocket;
    if (!Socket) {
      throw new CodeKanbanConfigError(
        "WebSocket implementation is unavailable",
      );
    }

    this.url = resolvedUrl;
    this.socket = webSocketOptions ? new Socket(resolvedUrl, webSocketOptions) : new Socket(resolvedUrl);
    this.protocolVersion = WEB_SESSION_PROTOCOL_VERSION;
    this._pendingRequests = new Map();
    this._pendingFollowUp = null;
    this._commandQueue = Promise.resolve();
    this._openPromise = new Promise((resolve, reject) => {
      this._resolveOpen = resolve;
      this._rejectOpen = reject;
    });
    this._closed = false;

    this.socket.addEventListener("open", () => {
      this._resolveOpen?.();
    });

    this.socket.addEventListener("message", (event) => {
      this._handleMessage(event.data);
    });

    this.socket.addEventListener("error", (event) => {
      const error = new CodeKanbanError("web session command websocket error", {
        event,
      });
      this._rejectOpen?.(error);
      this._rejectAll(error);
    });

    this.socket.addEventListener("close", (event) => {
      this._closed = true;
      const error = new CodeKanbanError(
        "web session command websocket closed",
        { event },
      );
      this._rejectOpen?.(error);
      this._rejectAll(error);
    });
  }

  async waitForOpen(timeoutMs = DEFAULT_OPEN_TIMEOUT_MS) {
    if (this.socket.readyState === SOCKET_OPEN) {
      return;
    }
    await Promise.race([
      this._openPromise,
      new Promise((_, reject) => {
        setTimeout(
          () =>
            reject(
              new CodeKanbanValidationError(
                `web session command channel did not open within ${timeoutMs}ms`,
              ),
            ),
          timeoutMs,
        );
      }),
    ]);
  }

  async list(projectId) {
    const ack = await this._executeCommand({
      operation: "list",
      payload: {
        pid: ensureString(projectId, "projectId"),
      },
    });
    return Array.isArray(ack.payload?.items) ? ack.payload.items : [];
  }

  async create(input = {}) {
    const { frame } = await this._executeCommand({
      operation: "create",
      payload: {
        pid: ensureString(input.projectId, "projectId"),
        wid: ensureOptionalString(input.worktreeId),
        ag: ensureString(input.agent, "agent"),
        cr: ensureOptionalString(input.claudeRuntime),
        md: ensureOptionalString(input.model),
        re: ensureOptionalString(input.reasoningEffort),
        wm: ensureOptionalString(input.workflowMode),
        pl: ensureOptionalString(input.permissionLevel),
        pm: ensureOptionalString(input.permissionMode),
        ttl: ensureOptionalString(input.title),
      },
      expectType: "snapshot",
    });
    return frame.snapshot;
  }

  async connect(sessionId) {
    const { frame } = await this._executeCommand({
      operation: "connect",
      sessionId,
      expectType: "snapshot",
    });
    return frame.snapshot;
  }

  async history(sessionId, options = {}) {
    const { frame } = await this._executeCommand({
      operation: "hist",
      sessionId,
      payload: {
        lim: normalizeHistoryLimit(options.limit),
        bc: ensureOptionalString(options.beforeCursor),
      },
      expectType: "historyPage",
    });
    return frame.history;
  }

  async sendMessage(sessionId, input = {}) {
    const text = ensureOptionalString(input.text);
    const attachmentIds = ensureArrayOfStrings(
      input.attachmentIds,
      "attachmentIds",
    );
    const mode = ensureOptionalString(input.mode);
    if (!text && attachmentIds.length === 0) {
      throw new CodeKanbanValidationError("text or attachmentIds is required");
    }
    return await this._executeCommand({
      operation: "send",
      sessionId,
      payload: {
        txt: text,
        atts: attachmentIds,
        ...(mode ? { mode } : {}),
      },
    });
  }

  async removePendingInput(sessionId, input = {}) {
    return await this._executeCommand({
      operation: "pending_del",
      sessionId,
      payload: {
        id: ensureString(input.pendingId, "pendingId"),
      },
    });
  }

  async updatePendingInput(sessionId, input = {}) {
    const pendingId = ensureString(input.pendingId, "pendingId");
    const hasText = input.text !== undefined;
    const hasPaused = typeof input.paused === "boolean";
    if (!hasText && !hasPaused) {
      throw new CodeKanbanValidationError("text or paused is required");
    }
    return await this._executeCommand({
      operation: "pending_update",
      sessionId,
      payload: {
        id: pendingId,
        ...(hasText ? { txt: ensureString(input.text, "text") } : {}),
        ...(hasPaused ? { paused: input.paused } : {}),
      },
    });
  }

  async abort(sessionId) {
    return await this._executeCommand({ operation: "abort", sessionId });
  }

  async approve(sessionId) {
    return await this._executeCommand({ operation: "approve", sessionId });
  }

  async reject(sessionId) {
    return await this._executeCommand({ operation: "reject", sessionId });
  }

  async answerUserInput(sessionId, input = {}) {
    return await this._executeCommand({
      operation: "user_input",
      sessionId,
      payload: {
        iid: ensureString(input.itemId, "itemId"),
        ans:
          input.answers && typeof input.answers === "object"
            ? input.answers
            : {},
      },
    });
  }

  async rename(sessionId, input = {}) {
    return await this._executeCommand({
      operation: "rename",
      sessionId,
      payload: {
        ttl: ensureString(input.title, "title"),
      },
    });
  }

  async updateModel(sessionId, input = {}) {
    return await this._executeCommand({
      operation: "set_md",
      sessionId,
      payload: {
        md: ensureString(input.model, "model"),
      },
    });
  }

  async updateClaudeRuntime(sessionId, input = {}) {
    return await this._executeCommand({
      operation: "set_cr",
      sessionId,
      payload: {
        cr: ensureString(input.claudeRuntime, "claudeRuntime"),
      },
    });
  }

  async updateReasoningEffort(sessionId, input = {}) {
    return await this._executeCommand({
      operation: "set_re",
      sessionId,
      payload: {
        re: ensureString(input.reasoningEffort, "reasoningEffort"),
      },
    });
  }

  async updateWorkflowMode(sessionId, input = {}) {
    return await this._executeCommand({
      operation: "set_wm",
      sessionId,
      payload: {
        wm: ensureString(input.workflowMode, "workflowMode"),
      },
    });
  }

  async getGoal(sessionId) {
    return await this._executeCommand({
      operation: "goal_get",
      sessionId,
    });
  }

  async setGoal(sessionId, input = {}) {
    return await this._executeCommand({
      operation: "goal_set",
      sessionId,
      payload: {
        obj: ensureString(input.objective, "objective"),
        st: ensureOptionalString(input.status),
      },
    });
  }

  async bootstrapGoal(sessionId, input = {}) {
    return await this._executeCommand({
      operation: "goal_bootstrap",
      sessionId,
      payload: {
        obj: ensureString(input.objective, "objective"),
        st: ensureOptionalString(input.status),
      },
    });
  }

  async pauseGoal(sessionId) {
    return await this._executeCommand({
      operation: "goal_pause",
      sessionId,
    });
  }

  async resumeGoal(sessionId) {
    return await this._executeCommand({
      operation: "goal_resume",
      sessionId,
    });
  }

  async clearGoal(sessionId) {
    return await this._executeCommand({
      operation: "goal_clear",
      sessionId,
    });
  }

  async updatePermissionLevel(sessionId, input = {}) {
    return await this._executeCommand({
      operation: "set_pl",
      sessionId,
      payload: {
        pl: ensureString(input.permissionLevel, "permissionLevel"),
      },
    });
  }

  async updateAgent(sessionId, input = {}) {
    return await this._executeCommand({
      operation: "set_ag",
      sessionId,
      payload: {
        ag: ensureString(input.agent, "agent"),
      },
    });
  }

  async move(sessionId, input = {}) {
    return await this._executeCommand({
      operation: "move",
      sessionId,
      payload: {
        prv: ensureOptionalString(input.prevSessionId),
        nxt: ensureOptionalString(input.nextSessionId),
      },
    });
  }

  async delete(sessionId) {
    return await this._executeCommand({ operation: "del", sessionId });
  }

  close() {
    this._closed = true;
    this.socket.close();
  }

  _enqueue(task) {
    const next = this._commandQueue.then(task, task);
    this._commandQueue = next.catch(() => {});
    return next;
  }

  async _executeCommand({
    operation,
    sessionId,
    payload = {},
    expectType = null,
  }) {
    return await this._enqueue(async () => {
      await this.waitForOpen();
      if (this._closed || this.socket.readyState !== SOCKET_OPEN) {
        throw new CodeKanbanError("web session command channel is not open");
      }

      const requestId = createRequestId();
      const ackPromise = new Promise((resolve, reject) => {
        this._pendingRequests.set(requestId, {
          resolve,
          reject,
          ack: null,
          ackTimer: null,
        });
      });

      const followUpPromise =
        expectType == null
          ? null
          : new Promise((resolve, reject) => {
              this._pendingFollowUp = {
                expectedType: expectType,
                sessionId: ensureOptionalString(sessionId) || null,
                resolve,
                reject,
              };
            });

      this.socket.send(
        JSON.stringify(
          buildWebSessionCommandFrame({
            requestId,
            sessionId,
            operation,
            payload,
          }),
        ),
      );

      const ack = await ackPromise;
      if (!followUpPromise) {
        return ack;
      }
      const frame = await followUpPromise;
      return { ack, frame };
    });
  }

  _handleMessage(data) {
    const rawFrame = decodeWebSessionSocketMessage(data);
    if (isWebSessionHeartbeatFrame(rawFrame)) {
      if (rawFrame?.op === "ping" && this.socket.readyState === SOCKET_OPEN) {
        this.socket.send(JSON.stringify(buildWebSessionHeartbeatFrame("pong")));
      }
      return;
    }
    const frame = normalizeWebSessionFrame(rawFrame);

    if (
      frame.type === "error" &&
      frame.requestId &&
      this._pendingRequests.has(frame.requestId)
    ) {
      const pending = this._pendingRequests.get(frame.requestId);
      this._clearAckTimer(pending);
      this._pendingRequests.delete(frame.requestId);
      pending?.reject(
        new CodeKanbanError(frame.message, {
          name: "CodeKanbanWebSessionCommandError",
          code: frame.code,
          retry: frame.retry,
          frame,
        }),
      );
      return;
    }

    if (
      frame.type === "ack" &&
      frame.requestId &&
      this._pendingRequests.has(frame.requestId)
    ) {
      const pending = this._pendingRequests.get(frame.requestId);
      if (!pending) {
        return;
      }
      pending.ack = frame;
      pending.ackTimer = setTimeout(() => {
        this._pendingRequests.delete(frame.requestId);
        pending.resolve(frame);
      }, ACK_SETTLE_DELAY_MS);
      return;
    }

    if (
      this._pendingFollowUp &&
      frame.type === this._pendingFollowUp.expectedType &&
      (!this._pendingFollowUp.sessionId ||
        frame.sessionId === this._pendingFollowUp.sessionId)
    ) {
      const pendingFollowUp = this._pendingFollowUp;
      this._pendingFollowUp = null;
      pendingFollowUp.resolve(frame);
    }
  }

  _clearAckTimer(pending) {
    if (pending?.ackTimer) {
      clearTimeout(pending.ackTimer);
      pending.ackTimer = null;
    }
  }

  _rejectAll(error) {
    this._pendingRequests.forEach((pending) => {
      this._clearAckTimer(pending);
      pending.reject(error);
    });
    this._pendingRequests.clear();

    if (this._pendingFollowUp) {
      this._pendingFollowUp.reject(error);
      this._pendingFollowUp = null;
    }
  }
}
