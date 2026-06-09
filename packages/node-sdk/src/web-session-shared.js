import path from "node:path";

import { CodeKanbanValidationError } from "./errors.js";
import {
  ensureArrayOfStrings,
  ensureOptionalString,
  ensureString,
} from "./utils.js";

export const WEB_SESSION_PROTOCOL_VERSION = 1;
export const WEB_SESSION_COMMAND_WS_PATH = "/api/v1/web-sessions/ws";
export const WEB_SESSION_EVENTS_WS_PATH = "/api/v1/web-sessions/events";
export const WEB_SESSION_HEARTBEAT_KIND = "hb";

const IMAGE_MIME_BY_EXT = new Map([
  [".apng", "image/apng"],
  [".bmp", "image/bmp"],
  [".gif", "image/gif"],
  [".heic", "image/heic"],
  [".heif", "image/heif"],
  [".jpeg", "image/jpeg"],
  [".jpg", "image/jpeg"],
  [".png", "image/png"],
  [".svg", "image/svg+xml"],
  [".svgz", "image/svg+xml"],
  [".tif", "image/tiff"],
  [".tiff", "image/tiff"],
  [".webp", "image/webp"],
]);

function stringValue(value) {
  if (typeof value === "string") {
    return value;
  }
  if (value == null) {
    return "";
  }
  return String(value);
}

function trimmedString(value) {
  return stringValue(value).trim();
}

function numberValue(value, fallback = 0) {
  const normalized = Number(value);
  return Number.isFinite(normalized) ? normalized : fallback;
}

function nullableNumberValue(value) {
  const normalized = Number(value);
  return Number.isFinite(normalized) ? normalized : null;
}

function isoFromUnixMilli(value) {
  const millis = nullableNumberValue(value);
  if (millis == null) {
    return null;
  }
  return new Date(millis).toISOString();
}

function booleanValue(value) {
  return value === true;
}

function normalizeUsage(value) {
  return {
    inputTokens: numberValue(value?.in, 0),
    cachedInputTokens: numberValue(value?.cin, 0),
    outputTokens: numberValue(value?.out, 0),
    cost: numberValue(value?.cost, 0),
  };
}

function normalizeContextEstimate(value, usageFallback) {
  return {
    inputTokens: numberValue(value?.in, numberValue(usageFallback?.in, 0)),
    cachedInputTokens: numberValue(value?.cin, numberValue(usageFallback?.cin, 0)),
    outputTokens: numberValue(value?.out, numberValue(usageFallback?.out, 0)),
    usedTokens: numberValue(
      value?.usd,
      Math.max(
        0,
        numberValue(value?.in, numberValue(usageFallback?.in, 0)) +
          numberValue(value?.out, numberValue(usageFallback?.out, 0)),
      ),
    ),
  };
}

function normalizeGoal(value) {
  if (!value || typeof value !== "object") {
    return null;
  }
  const objective = trimmedString(value?.obj ?? value?.objective);
  const threadId = trimmedString(value?.tid ?? value?.threadId);
  const status = trimmedString(value?.st ?? value?.status);
  if (!objective || !threadId || !status) {
    return null;
  }
  return {
    threadId,
    objective,
    status,
    tokenBudget: nullableNumberValue(value?.tb ?? value?.tokenBudget),
    tokensUsed: numberValue(value?.tu ?? value?.tokensUsed, 0),
    timeUsedSeconds: numberValue(value?.tsu ?? value?.timeUsedSeconds, 0),
    createdAt:
      isoFromUnixMilli(value?.ca ?? value?.createdAt) || new Date().toISOString(),
    updatedAt:
      isoFromUnixMilli(value?.ua ?? value?.updatedAt) || new Date().toISOString(),
  };
}

function normalizeHistoryAttachment(value) {
  return {
    id: trimmedString(value?.id),
    name: trimmedString(value?.name),
    mime: trimmedString(value?.mime) || null,
    size: nullableNumberValue(value?.sz),
    path: trimmedString(value?.path) || null,
  };
}

function normalizeHistoryToolCommandGroup(value) {
  const id = trimmedString(value?.id);
  if (!id) {
    return null;
  }
  return {
    id,
    count: Math.max(1, numberValue(value?.count, 1)),
    firstSeq: nullableNumberValue(value?.firstSeq),
    lastSeq: nullableNumberValue(value?.lastSeq),
    latestToolId: trimmedString(value?.latestToolId) || null,
    compacted: booleanValue(value?.compacted),
  };
}

function normalizeHistoryTool(value) {
  if (!value || typeof value !== "object") {
    return null;
  }
  return {
    id: trimmedString(value.id),
    name: trimmedString(value.name),
    kind: trimmedString(value.kind) || null,
    input: value.in,
    output: trimmedString(value.out) || null,
    status: trimmedString(value.st) || "running",
    meta: value.meta && typeof value.meta === "object" ? value.meta : null,
    commandGroup: normalizeHistoryToolCommandGroup(value.cg),
  };
}

function normalizeUserInputOption(value) {
  return {
    label: trimmedString(value?.label),
    description: trimmedString(value?.description),
  };
}

function normalizeUserInputQuestion(value) {
  return {
    id: trimmedString(value?.id),
    header: trimmedString(value?.header),
    question: trimmedString(value?.question),
    isOther: booleanValue(value?.isOther),
    isSecret: booleanValue(value?.isSecret),
    options: Array.isArray(value?.options)
      ? value.options.map(normalizeUserInputOption)
      : [],
  };
}

function normalizeHistoryAnswerEntry(value) {
  return {
    id: trimmedString(value?.id),
    label: trimmedString(value?.label),
    values: ensureArrayOfStrings(value?.values, "values"),
    masked: booleanValue(value?.masked),
  };
}

function normalizeHistoryDetail(value) {
  if (!value || typeof value !== "object") {
    return null;
  }
  return {
    type: trimmedString(value.type),
    prompt: trimmedString(value.prompt) || null,
    questions: Array.isArray(value.questions)
      ? value.questions.map(normalizeUserInputQuestion)
      : [],
    answers: Array.isArray(value.answers)
      ? value.answers.map(normalizeHistoryAnswerEntry)
      : [],
    action: trimmedString(value.action) || null,
  };
}

function normalizePendingInput(value) {
  const id = trimmedString(value?.id);
  if (!id) {
    return null;
  }
  const mode = trimmedString(value?.m || value?.mode);
  if (mode !== "redirect" && mode !== "queue") {
    return null;
  }
  return {
    id,
    mode,
    text: stringValue(value?.txt ?? value?.text),
    attachmentIds: ensureArrayOfStrings(
      value?.atts ?? value?.attachmentIds,
      "attachmentIds",
    ),
    createdAt:
      isoFromUnixMilli(value?.ca) ||
      (typeof value?.createdAt === "string"
        ? trimmedString(value.createdAt) || null
        : null),
  };
}

function normalizePendingUserInputState(value) {
  const itemId = trimmedString(value?.iid ?? value?.itemId);
  if (!itemId) {
    return null;
  }
  return {
    itemId,
    prompt: trimmedString(value?.txt ?? value?.prompt) || "",
    questions: Array.isArray(value?.qs ?? value?.questions)
      ? (value.qs ?? value.questions).map(normalizeUserInputQuestion)
      : [],
    requestedAt:
      isoFromUnixMilli(value?.ra) ||
      (typeof value?.requestedAt === "string"
        ? trimmedString(value.requestedAt) || null
        : null),
  };
}

export function normalizeWebSessionHistoryItem(value) {
  const updatedTimestamp =
    isoFromUnixMilli(value?.ts2) || isoFromUnixMilli(value?.obs);
  return {
    id: trimmedString(value?.id),
    sourceTurnId: trimmedString(value?.stid) || null,
    sourceItemId: trimmedString(value?.siid) || null,
    orderIndex: numberValue(value?.oi, 0),
    kind: trimmedString(value?.kd) || "system",
    itemType: trimmedString(value?.tp),
    text: stringValue(value?.txt),
    timestamp: isoFromUnixMilli(value?.ts2),
    observedAt: isoFromUnixMilli(value?.obs),
    attachments: Array.isArray(value?.atts)
      ? value.atts.map(normalizeHistoryAttachment)
      : [],
    tool: normalizeHistoryTool(value?.tl),
    level: trimmedString(value?.lvl) || null,
    done: booleanValue(value?.dn),
    detail: normalizeHistoryDetail(value?.dt),
    payload: value?.pl && typeof value.pl === "object" ? value.pl : null,
    updatedAt: updatedTimestamp,
  };
}

export function normalizeWebSessionHistoryWindow(value) {
  return {
    items: Array.isArray(value?.its)
      ? value.its.map(normalizeWebSessionHistoryItem)
      : [],
    hasMore: booleanValue(value?.hm),
    beforeCursor: trimmedString(value?.bc) || null,
    total: numberValue(value?.tot, 0),
  };
}

export function normalizeWebSessionSummaryFromWire(value) {
  const updatedAtMs = numberValue(value?.lu, Date.now());
  return {
    id: trimmedString(value?.id),
    projectId: trimmedString(value?.pid),
    worktreeId: trimmedString(value?.wid) || null,
    orderIndex: numberValue(value?.oi, 0),
    agent: trimmedString(value?.ag) || "codex",
    claudeRuntime: trimmedString(value?.cr) === "ccr" ? "ccr" : "claude",
    title: trimmedString(value?.ttl),
    model: trimmedString(value?.md),
    reasoningEffort: trimmedString(value?.re) || "default",
    workflowMode: trimmedString(value?.wm) || "default",
    permissionLevel: trimmedString(value?.pl) || "elevated",
    cwd: trimmedString(value?.cwd),
    nativeSessionId: trimmedString(value?.nsid) || null,
    status: trimmedString(value?.st) || "idle",
    assistantState: trimmedString(value?.ast) || null,
    hasUnread: booleanValue(value?.unr),
    archivedAt: isoFromUnixMilli(value?.aa),
    activityAt:
      isoFromUnixMilli(value?.act) || new Date(updatedAtMs).toISOString(),
    lastMessageAt: isoFromUnixMilli(value?.lma),
    assistantStateUpdatedAt: isoFromUnixMilli(value?.asu),
    sourceKind: trimmedString(value?.sk) || "codex_app_server",
    syncState: trimmedString(value?.ss) || "missing",
    lastSyncMode: trimmedString(value?.lsm) || null,
    sourceCreatedAt: isoFromUnixMilli(value?.sca),
    sourceUpdatedAt: isoFromUnixMilli(value?.sua),
    lastSyncedAt: isoFromUnixMilli(value?.lsa),
    threadPath: trimmedString(value?.tp) || null,
    threadPreview: trimmedString(value?.tpv) || null,
    turnCount: numberValue(value?.tc, 0),
    itemCount: numberValue(value?.ic, 0),
    syncError: trimmedString(value?.se) || null,
    createdAt:
      isoFromUnixMilli(value?.ca) || new Date(updatedAtMs).toISOString(),
    updatedAt: new Date(updatedAtMs).toISOString(),
    usage: {
      ...normalizeUsage(value?.usa),
      cost: numberValue(value?.cost, 0),
    },
    latestTurnUsage: value?.ltu ? normalizeContextEstimate(value.ltu, null) : null,
    contextEstimate: normalizeContextEstimate(value?.cea, value?.usa),
    contextEstimateMode: trimmedString(value?.cem) || "cumulative_total",
    lastContextCompactionAt: isoFromUnixMilli(value?.lcca),
    contextWindowTokens: nullableNumberValue(value?.cwt),
    contextWindowSource: trimmedString(value?.cws) || "unavailable",
    goal: normalizeGoal(value?.goal),
  };
}

export function normalizeWebSessionSnapshotFromWire(frame) {
  return {
    session: normalizeWebSessionSummaryFromWire(frame?.s),
    history: normalizeWebSessionHistoryWindow(frame?.h),
    pendingInputs: Array.isArray(frame?.pi)
      ? frame.pi.map(normalizePendingInput).filter(Boolean)
      : [],
    pendingUserInput: normalizePendingUserInputState(frame?.ui),
  };
}

const PROCESS_RESTART_REASON = "process_restart";
const DEFAULT_RECOVERY_MESSAGE =
  "Session runtime was interrupted. Send a new message to continue.";

function isProcessRestartPayload(payload) {
  return trimmedString(payload?.reason) === PROCESS_RESTART_REASON;
}

function getRecoveryMessage(payload) {
  return trimmedString(payload?.msg) || DEFAULT_RECOVERY_MESSAGE;
}

function normalizeChoiceText(value) {
  return trimmedString(value).toLowerCase().replace(/\s+/g, " ");
}

function isExecutePlanOption(option) {
  const text = normalizeChoiceText(
    `${option?.label || ""} ${option?.description || ""}`,
  );
  const mentionsPlan = /计划|plan/.test(text);
  const mentionsExecute =
    /开始|执行|实现|实施|继续|start|execute|implement|proceed/.test(text);
  const mentionsCancel = /取消|暂不|稍后|later|cancel|dismiss|hold/.test(text);
  return mentionsExecute && (mentionsPlan || !mentionsCancel);
}

function isCancelPlanOption(option) {
  const text = normalizeChoiceText(
    `${option?.label || ""} ${option?.description || ""}`,
  );
  return /取消|暂不|稍后|later|cancel|dismiss|hold|keep planning|stay in plan/.test(
    text,
  );
}

function isPlanChoiceQuestion(question) {
  if (
    !question ||
    !Array.isArray(question.options) ||
    question.options.length !== 2
  ) {
    return false;
  }
  const hasExecute = question.options.some(isExecutePlanOption);
  const hasCancel = question.options.some(isCancelPlanOption);
  return hasExecute && hasCancel;
}

function findPendingApproval(items) {
  let pending = null;
  for (const item of items) {
    if (item?.detail?.type === "approval_request") {
      pending = {
        id: item.id,
        itemId: item.sourceItemId || item.id,
        prompt: item.detail.prompt || item.text || "",
        requestedAt: item.timestamp || item.observedAt || null,
        stale: false,
      };
      continue;
    }
    if (item?.detail?.type === "approval_response" || item?.kind === "user") {
      pending = null;
      continue;
    }
    if (
      item?.itemType === "run_abort" &&
      pending &&
      isProcessRestartPayload(item.payload || undefined)
    ) {
      pending = {
        ...pending,
        stale: true,
        recoveryReason: trimmedString(item.payload?.reason) || null,
        recoveryMessage: getRecoveryMessage(item.payload || undefined),
      };
      continue;
    }
    if (item?.itemType === "run_abort" || item?.itemType === "run_fail") {
      pending = null;
    }
  }
  return pending;
}

function findPendingUserInput(items) {
  let pending = null;
  for (const item of items) {
    if (item?.detail?.type === "user_input_request") {
      const questions = Array.isArray(item.detail.questions)
        ? item.detail.questions
        : [];
      const question = questions[0];
      const executeOption =
        questions.length === 1 && isPlanChoiceQuestion(question)
          ? question.options.find(isExecutePlanOption) || null
          : null;
      pending = {
        id: item.id,
        itemId:
          item.sourceItemId || trimmedString(item.payload?.iid) || item.id,
        prompt: item.detail.prompt || item.text || "",
        questions,
        requestedAt: item.timestamp || item.observedAt || null,
        stale: false,
        isPlanChoice: Boolean(executeOption),
        questionId: executeOption ? trimmedString(question?.id) || null : null,
        executeOptionLabel: executeOption
          ? trimmedString(executeOption.label) || null
          : null,
      };
      continue;
    }
    if (item?.detail?.type === "user_input_response" || item?.kind === "user") {
      pending = null;
      continue;
    }
    if (
      item?.itemType === "run_abort" &&
      pending &&
      isProcessRestartPayload(item.payload || undefined)
    ) {
      pending = {
        ...pending,
        stale: true,
        recoveryReason: trimmedString(item.payload?.reason) || null,
        recoveryMessage: getRecoveryMessage(item.payload || undefined),
      };
      continue;
    }
    if (item?.itemType === "run_abort" || item?.itemType === "run_fail") {
      pending = null;
    }
  }
  return pending;
}

function findLatestPlan(items) {
  const latestPlan = [...items]
    .reverse()
    .find((item) => item?.kind === "tool" && item?.tool?.kind === "plan");
  if (!latestPlan) {
    return null;
  }
  const hasUserMessageAfter = items.some(
    (item) =>
      item?.kind === "user" &&
      numberValue(item.orderIndex, 0) > numberValue(latestPlan.orderIndex, 0),
  );
  return {
    id: latestPlan.id,
    itemId: latestPlan.id,
    toolId: latestPlan.tool.id || latestPlan.id,
    output: latestPlan.tool.output || latestPlan.text || "",
    timestamp: latestPlan.timestamp || latestPlan.observedAt || null,
    orderIndex: latestPlan.orderIndex,
    hasUserMessageAfter,
    awaitingExecution: !hasUserMessageAfter,
  };
}

function findLastAssistantMessage(items) {
  const lastAssistant = [...items]
    .reverse()
    .find((item) => item?.kind === "assistant");
  if (!lastAssistant) {
    return null;
  }
  return {
    id: lastAssistant.id,
    itemId: lastAssistant.id,
    text: lastAssistant.text || "",
    timestamp: lastAssistant.timestamp || lastAssistant.observedAt || null,
    orderIndex: lastAssistant.orderIndex,
  };
}

export function analyzeWebSession(snapshot) {
  const session = snapshot?.session || null;
  const history = snapshot?.history || {
    items: [],
    hasMore: false,
    beforeCursor: null,
    total: 0,
  };
  const pendingInputs = Array.isArray(snapshot?.pendingInputs)
    ? snapshot.pendingInputs.filter(Boolean)
    : [];
  const items = Array.isArray(history.items) ? history.items : [];
  const pendingApproval = findPendingApproval(items);
  const pendingUserInput =
    normalizePendingUserInputState(snapshot?.pendingUserInput) ||
    findPendingUserInput(items);
  const latestPlan = findLatestPlan(items);
  const lastAssistantMessage = findLastAssistantMessage(items);

  let phase = "idle";
  if (session?.assistantState === "waiting_input") {
    phase = "waiting_input";
  } else if (session?.assistantState === "waiting_approval") {
    phase = "waiting_approval";
  } else if (session?.assistantState === "waiting_plan_approval") {
    phase = "waiting_plan_approval";
  } else if (session?.status === "running") {
    phase = "running";
  } else if (session?.status === "done") {
    phase = "done";
  } else if (session?.status === "err") {
    phase = "error";
  } else if (session?.status === "aborting") {
    phase = "aborting";
  }

  let nextAction = null;
  if (pendingApproval) {
    nextAction = {
      type: "approval",
      prompt: pendingApproval.prompt,
      requestedAt: pendingApproval.requestedAt,
    };
  } else if (pendingUserInput?.isPlanChoice && latestPlan?.awaitingExecution) {
    nextAction = {
      type: "execute_plan",
      itemId: pendingUserInput.itemId,
      questionId: pendingUserInput.questionId,
      executeOptionLabel: pendingUserInput.executeOptionLabel,
      latestPlan,
    };
  } else if (pendingUserInput) {
    nextAction = {
      type: "answer_user_input",
      itemId: pendingUserInput.itemId,
      prompt: pendingUserInput.prompt,
      questions: pendingUserInput.questions,
      requestedAt: pendingUserInput.requestedAt,
    };
  } else if (
    phase === "waiting_plan_approval" &&
    latestPlan?.awaitingExecution
  ) {
    nextAction = {
      type: "execute_plan",
      latestPlan,
    };
  }

  const canSend =
    phase === "idle" ||
    phase === "done" ||
    phase === "error" ||
    phase === "waiting_plan_approval";

  return {
    phase,
    canSend,
    needsAction: nextAction != null,
    nextAction,
    pendingApproval,
    pendingUserInput,
    pendingInputs,
    latestPlan,
    lastAssistantMessage,
    session,
    snapshot: {
      session,
      history,
      pendingInputs,
      pendingUserInput,
    },
  };
}

export function decodeWebSessionSocketMessage(raw) {
  if (typeof raw === "string") {
    return JSON.parse(raw);
  }
  if (raw instanceof ArrayBuffer) {
    return JSON.parse(Buffer.from(raw).toString("utf8"));
  }
  if (ArrayBuffer.isView(raw)) {
    return JSON.parse(
      Buffer.from(raw.buffer, raw.byteOffset, raw.byteLength).toString("utf8"),
    );
  }
  return JSON.parse(String(raw ?? ""));
}

export function normalizeWebSessionFrame(rawFrame) {
  const timestampMs = nullableNumberValue(rawFrame?.ts);
  const base = {
    requestId: trimmedString(rawFrame?.rid) || null,
    sessionId: trimmedString(rawFrame?.sid) || null,
    timestampMs,
    timestamp: timestampMs == null ? null : new Date(timestampMs).toISOString(),
    operation: trimmedString(rawFrame?.op) || null,
    raw: rawFrame,
  };

  if (rawFrame?.k === "ack") {
    return {
      ...base,
      type: "ack",
      ok: rawFrame?.ok === 1 || rawFrame?.ok === true,
      payload: rawFrame?.p ?? null,
    };
  }

  if (rawFrame?.k === "err") {
    return {
      ...base,
      type: "error",
      code: trimmedString(rawFrame?.code) || "unknown_error",
      message: trimmedString(rawFrame?.msg) || "Unknown websocket error",
      retry: booleanValue(rawFrame?.retry),
    };
  }

  if (rawFrame?.k === WEB_SESSION_HEARTBEAT_KIND) {
    return {
      ...base,
      type: "heartbeat",
      heartbeatType: trimmedString(rawFrame?.op) || null,
    };
  }

  if (rawFrame?.k === "snap") {
    return {
      ...base,
      type: "snapshot",
      snapshot: normalizeWebSessionSnapshotFromWire(rawFrame),
    };
  }

  if (rawFrame?.k === "evt") {
    if (rawFrame?.op === "session" && rawFrame?.s) {
      return {
        ...base,
        type: "session",
        session: normalizeWebSessionSummaryFromWire(rawFrame.s),
      };
    }
    if (rawFrame?.op === "hist_page" && rawFrame?.h) {
      return {
        ...base,
        type: "historyPage",
        history: normalizeWebSessionHistoryWindow(rawFrame.h),
      };
    }
    if (rawFrame?.op === "hist_item" && rawFrame?.i) {
      return {
        ...base,
        type: "historyItem",
        item: normalizeWebSessionHistoryItem(rawFrame.i),
        session: rawFrame?.s
          ? normalizeWebSessionSummaryFromWire(rawFrame.s)
          : null,
      };
    }
    if (rawFrame?.op === "pending") {
      return {
        ...base,
        type: "pending",
        pendingInputs: Array.isArray(rawFrame?.pi)
          ? rawFrame.pi.map(normalizePendingInput).filter(Boolean)
          : [],
      };
    }
    return {
      ...base,
      type: "event",
      payload: rawFrame?.p ?? null,
    };
  }

  return {
    ...base,
    type: "unknown",
    payload: rawFrame?.p ?? null,
  };
}

export function buildWebSessionCommandFrame({
  requestId,
  sessionId,
  operation,
  payload,
}) {
  return {
    v: WEB_SESSION_PROTOCOL_VERSION,
    k: "cmd",
    rid: ensureString(requestId, "requestId"),
    sid: ensureOptionalString(sessionId) || undefined,
    op: ensureString(operation, "operation"),
    p: payload ?? {},
  };
}

export function buildWebSessionHeartbeatFrame(operation) {
  return {
    v: WEB_SESSION_PROTOCOL_VERSION,
    k: WEB_SESSION_HEARTBEAT_KIND,
    ts: Date.now(),
    op: ensureString(operation, "operation"),
  };
}

export function isWebSessionHeartbeatFrame(rawFrame) {
  return rawFrame?.k === WEB_SESSION_HEARTBEAT_KIND;
}

export function normalizeWebSessionAttachment(value) {
  if (!value || typeof value !== "object") {
    return null;
  }
  return {
    id: trimmedString(value.id),
    name: trimmedString(value.name),
    mime: trimmedString(value.mime),
    size: numberValue(value.size, 0),
    path: trimmedString(value.path),
    createdAt: stringValue(value.createdAt || ""),
  };
}

export function inferWebSessionAttachmentMimeType(fileName) {
  const extension = path.extname(trimmedString(fileName)).toLowerCase();
  return IMAGE_MIME_BY_EXT.get(extension) || "";
}

export function ensureImageMimeType(value, fileName) {
  const explicit = ensureOptionalString(value);
  const inferred = inferWebSessionAttachmentMimeType(fileName);
  const mimeType = explicit || inferred;
  if (!mimeType || !mimeType.startsWith("image/")) {
    throw new CodeKanbanValidationError(
      "web session attachments must be image files",
    );
  }
  return mimeType;
}

export function shouldEmitWebSessionFrame(frame, sessionIdFilter) {
  const normalizedFilter = ensureOptionalString(sessionIdFilter);
  if (!normalizedFilter) {
    return frame?.type !== "heartbeat";
  }
  if (!frame || typeof frame !== "object") {
    return false;
  }
  if (
    frame.type === "open" ||
    frame.type === "close" ||
    frame.type === "error"
  ) {
    return true;
  }
  if (frame.type === "heartbeat") {
    return false;
  }
  return ensureOptionalString(frame.sessionId) === normalizedFilter;
}
