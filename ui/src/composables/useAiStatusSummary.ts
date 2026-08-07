import { computed } from 'vue';
import {
  EMPTY_AI_STATUS_SUMMARY,
  createAiStatusSummary,
  getWebSessionStatusBucket,
} from '@/composables/aiStatusSummary';
import { useProjectStore } from '@/stores/project';
import { useWebSessionStore } from '@/stores/webSession';
import type { AiStatusSummary } from '@/composables/aiStatusSummary';

export {
  EMPTY_AI_STATUS_SUMMARY,
  createAiStatusSummary,
  formatAiStatusTripletWithTotal,
  formatAiStatusTitle,
  formatAiStatusTriplet,
  getAiStatusSummaryTotal,
  getWebSessionStatusBucket,
  hasAiStatusSummary,
  summarizeWebSessions,
  type AiStatusSummary,
} from '@/composables/aiStatusSummary';

type StatusBucket = keyof AiStatusSummary;

const BUCKET_PRIORITY: Record<StatusBucket, number> = {
  unreadCompleted: 0,
  working: 1,
  blocking: 2,
};

function ensureProjectBuckets(
  projectBuckets: Map<string, Map<string, StatusBucket>>,
  projectId: string
) {
  let sessionBuckets = projectBuckets.get(projectId);
  if (!sessionBuckets) {
    sessionBuckets = new Map<string, StatusBucket>();
    projectBuckets.set(projectId, sessionBuckets);
  }
  return sessionBuckets;
}

function rememberSessionBucket(
  projectBuckets: Map<string, Map<string, StatusBucket>>,
  projectId: string | undefined,
  sessionKey: string,
  bucket: StatusBucket
) {
  if (!projectId || !sessionKey) {
    return;
  }

  const sessionBuckets = ensureProjectBuckets(projectBuckets, projectId);
  const currentBucket = sessionBuckets.get(sessionKey);
  if (currentBucket && BUCKET_PRIORITY[currentBucket] >= BUCKET_PRIORITY[bucket]) {
    return;
  }
  sessionBuckets.set(sessionKey, bucket);
}

function summarizeProjectBuckets(sessionBuckets?: Map<string, StatusBucket>): AiStatusSummary {
  if (!sessionBuckets) {
    return createAiStatusSummary();
  }

  const summary = createAiStatusSummary();
  sessionBuckets.forEach(bucket => {
    summary[bucket] += 1;
  });
  return summary;
}

export function useAiStatusSummary() {
  const projectStore = useProjectStore();
  const webSessionStore = useWebSessionStore();

  const projectBucketMap = computed(() => {
    const buckets = new Map<string, Map<string, StatusBucket>>();

    projectStore.projects.forEach(project => {
      webSessionStore.getSessions(project.id).forEach(session => {
        const bucket = getWebSessionStatusBucket(session, webSessionStore.getLiveState(session.id));
        if (!bucket) {
          return;
        }
        rememberSessionBucket(buckets, project.id, `web:${session.id}`, bucket);
      });
    });

    return buckets;
  });

  const projectSummaries = computed<Record<string, AiStatusSummary>>(() => {
    const summaries: Record<string, AiStatusSummary> = {};
    projectBucketMap.value.forEach((sessionBuckets, projectId) => {
      summaries[projectId] = summarizeProjectBuckets(sessionBuckets);
    });
    return summaries;
  });

  const totalSummary = computed(() => {
    const summary = createAiStatusSummary();
    projectBucketMap.value.forEach(sessionBuckets => {
      const projectSummary = summarizeProjectBuckets(sessionBuckets);
      summary.working += projectSummary.working;
      summary.blocking += projectSummary.blocking;
      summary.unreadCompleted += projectSummary.unreadCompleted;
    });
    return summary;
  });

  function getProjectSummary(projectId: string) {
    return projectSummaries.value[projectId] ?? EMPTY_AI_STATUS_SUMMARY;
  }

  return {
    projectSummaries,
    totalSummary,
    getProjectSummary,
  };
}
