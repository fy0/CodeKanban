import type { WebSessionSummary, WebSessionTokenSpeedRecord } from '@/types/models';
import type { PromptCacheRenewalSample } from '@/api/webSession';

// OpenAI's default minimum retention for GPT-5.6 and later. Codex does not
// report the request's retention setting or the server's actual expiry.
// https://developers.openai.com/api/docs/guides/prompt-caching#cache-lifetime
export const CODEX_PROMPT_CACHE_ESTIMATE_MS = 30 * 60_000;

export type PromptCacheSession = Pick<
  WebSessionSummary,
  | 'agent'
  | 'model'
  | 'contextEstimate'
  | 'contextEstimateMode'
  | 'latestTokenCountUpdatedAt'
  | 'lastContextCompactionAt'
>;

function promptCacheRecordUsage(record?: WebSessionTokenSpeedRecord) {
  if (
    record?.agent !== 'codex' ||
    record.inputTokens == null ||
    !Number.isFinite(record.inputTokens) ||
    record.inputTokens < 1024 ||
    record.cachedInputTokens == null ||
    !Number.isFinite(record.cachedInputTokens) ||
    record.cachedInputTokens < 0
  )
    return null;
  return {
    inputTokens: record.inputTokens,
    cachedTokens: Math.min(record.inputTokens, record.cachedInputTokens),
  };
}

export function promptCacheRecordEvent(
  record: WebSessionTokenSpeedRecord,
  previous?: WebSessionTokenSpeedRecord
) {
  const usage = promptCacheRecordUsage(record);
  if (!usage) return null;
  const previousUsage = previous?.model === record.model ? promptCacheRecordUsage(previous) : null;
  if (previousUsage && usage.cachedTokens < previousUsage.cachedTokens) {
    // Compare absolute reuse, since a larger input alone can lower the hit rate.
    // These labels are inferred from usage; Codex does not report cache resets.
    return usage.cachedTokens <= previousUsage.cachedTokens / 2
      ? ('rebuilt' as const)
      : ('suspected' as const);
  }
  return usage.cachedTokens > 0 ? ('hit' as const) : ('miss' as const);
}

export function estimateWebSessionPromptCache(
  session: PromptCacheSession | null,
  now: number,
  history: WebSessionTokenSpeedRecord[] = [],
  sample?: PromptCacheRenewalSample
) {
  if (session?.agent !== 'codex') return null;

  let timestamp = Date.parse(session.latestTokenCountUpdatedAt ?? '');
  const compactedAt = Date.parse(session.lastContextCompactionAt ?? '');
  let hasSnapshot =
    session.contextEstimateMode === 'latest_token_count' && !(compactedAt > timestamp);
  let inputTokens = hasSnapshot ? Math.max(0, session.contextEstimate.inputTokens) : 0;
  let cachedTokens = hasSnapshot
    ? Math.max(0, Math.min(inputTokens, session.contextEstimate.cachedInputTokens))
    : 0;
  let event: ReturnType<typeof promptCacheRecordEvent> = null;
  let observationKey = Number.isFinite(timestamp) ? `snapshot:${timestamp}` : '';
  let observationSource: 'request' | 'renewal' = 'request';
  const latest = history[0];
  if (latest && latest.model === session.model && promptCacheRecordEvent(latest)) {
    const endedAt = Date.parse(latest.endedAt);
    const matchesSnapshot =
      hasSnapshot &&
      inputTokens === latest.inputTokens &&
      cachedTokens === latest.cachedInputTokens;
    if (
      !(compactedAt > endedAt) &&
      Number.isFinite(endedAt) &&
      (!hasSnapshot || !Number.isFinite(timestamp) || endedAt >= timestamp || matchesSnapshot)
    ) {
      timestamp =
        matchesSnapshot && Number.isFinite(timestamp) ? Math.max(timestamp, endedAt) : endedAt;
      inputTokens = latest.inputTokens!;
      cachedTokens = Math.min(inputTokens, latest.cachedInputTokens!);
      hasSnapshot = true;
      event = promptCacheRecordEvent(latest, history[1]);
      observationKey = `${latest.runId}:${latest.sampleId}`;
    }
  }
  const sampledAt = Date.parse(sample?.observedAt ?? '');
  if (
    sample?.model === session.model &&
    Number.isFinite(sampledAt) &&
    sampledAt > 0 &&
    !(compactedAt > sampledAt) &&
    (!hasSnapshot || !Number.isFinite(timestamp) || sampledAt > timestamp) &&
    sample.inputTokens != null &&
    Number.isFinite(sample.inputTokens) &&
    sample.inputTokens >= 0 &&
    sample.cachedInputTokens != null &&
    Number.isFinite(sample.cachedInputTokens) &&
    sample.cachedInputTokens >= 0
  ) {
    timestamp = sampledAt;
    inputTokens = sample.inputTokens;
    cachedTokens = Math.min(inputTokens, sample.cachedInputTokens);
    hasSnapshot = true;
    // A sample uses a different suffix from a normal request. Comparing their
    // cached token counts would incorrectly report a rebuild.
    event = inputTokens >= 1024 ? (cachedTokens > 0 ? 'hit' : 'miss') : null;
    observationKey = `renewal:${timestamp}`;
    observationSource = 'renewal';
  }
  const hasObservation = hasSnapshot && Number.isFinite(timestamp) && timestamp > 0;
  const version = /^gpt-(\d+)(?:\.(\d+))?(?:[-.]|$)/i.exec(session.model.trim());
  const supportsDefaultWindow = Boolean(
    version && (Number(version[1]) > 5 || (Number(version[1]) === 5 && Number(version[2]) >= 6))
  );
  const windowMs = supportsDefaultWindow ? CODEX_PROMPT_CACHE_ESTIMATE_MS : null;
  // Small requests can have no cacheable prefix; a request alone is not proof
  // of a cache write. The UI labels every countdown as an estimate.
  const remainingMs =
    hasObservation && inputTokens >= 1024 && windowMs !== null
      ? Math.max(0, windowMs - Math.max(0, now - timestamp))
      : null;
  const expiredMs =
    remainingMs === 0 && hasObservation && windowMs !== null
      ? Math.max(0, now - timestamp - windowMs)
      : null;
  return {
    observationKey: hasObservation ? observationKey : '',
    observationSource,
    event,
    previousCachedTokens:
      observationSource === 'request' && (event === 'rebuilt' || event === 'suspected')
        ? promptCacheRecordUsage(history[1])!.cachedTokens
        : null,
    observedAt: hasObservation ? timestamp : null,
    inputTokens,
    cachedTokens,
    hitPercent: inputTokens > 0 ? Math.round((cachedTokens / inputTokens) * 100) : null,
    windowMs,
    remainingMs,
    expiredMs,
    remainingRatio: remainingMs !== null && windowMs ? remainingMs / windowMs : 0,
    state:
      remainingMs === null
        ? ('unknown' as const)
        : remainingMs === 0
          ? ('elapsed' as const)
          : remainingMs <= 5 * 60_000
            ? ('ending' as const)
            : ('estimated' as const),
  };
}

export function formatPromptCacheCountdown(remainingMs: number) {
  const seconds = Math.max(0, Math.ceil(remainingMs / 1000));
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`;
}
