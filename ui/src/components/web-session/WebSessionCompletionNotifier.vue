<script setup lang="ts">
import { h, onMounted, onUnmounted } from 'vue';
import { NButton, useNotification, type NotificationReactive } from 'naive-ui';
import { useI18n } from 'vue-i18n';
import { useRoute, useRouter } from 'vue-router';
import { useProjectStore } from '@/stores/project';
import { useWebSessionStore, type WebSessionAIEvent } from '@/stores/webSession';
import { openWebSessionNotificationTarget } from '@/components/web-session/webSessionNotificationTarget';

const notification = useNotification();
const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const projectStore = useProjectStore();
const webSessionStore = useWebSessionStore();

const notifiedSessions = new Set<string>();
const activeNotifications = new Map<string, NotificationReactive>();
const NOTIFICATION_COOLDOWN = 3000;
const LOG_PREFIX = '[Web Session Completion Notifier]';

function resolveProjectName(projectId?: string) {
  const normalizedProjectId = String(projectId || '').trim();
  if (!normalizedProjectId) {
    return '';
  }
  if (projectStore.currentProject?.id === normalizedProjectId && projectStore.currentProject.name) {
    return projectStore.currentProject.name;
  }
  return (
    projectStore.projects.find(project => project.id === normalizedProjectId)?.name ||
    projectStore.recentProjects.find(project => project.id === normalizedProjectId)?.name ||
    ''
  );
}

function clearNotification(sessionId?: string) {
  const normalizedSessionId = String(sessionId || '').trim();
  if (!normalizedSessionId) {
    return;
  }
  const instance = activeNotifications.get(normalizedSessionId);
  if (!instance) {
    return;
  }
  instance.destroy();
  activeNotifications.delete(normalizedSessionId);
}

async function openSessionNotificationTarget(event: WebSessionAIEvent) {
  try {
    await openWebSessionNotificationTarget({
      event,
      query: route.query,
      addRecentProject: projectStore.addRecentProject,
      push: location => router.push(location),
    });
  } catch (error) {
    console.error(`${LOG_PREFIX} Failed to open session notification target`, error);
  }
}

function renderNotificationAction(event: WebSessionAIEvent) {
  const projectId = String(event.projectId || '').trim();
  const sessionId = String(event.sessionId || '').trim();
  if (!projectId || !sessionId) {
    return undefined;
  }

  return () =>
    h(
      NButton,
      {
        text: true,
        size: 'small',
        onClick: (clickEvent: MouseEvent) => {
          clickEvent.stopPropagation();
          void openSessionNotificationTarget(event);
        },
      },
      {
        default: () => t('webSession.openSessionNotificationAction'),
      }
    );
}

function playCompletionSound() {
  try {
    const audioContext = new (window.AudioContext ||
      (
        window as Window &
          typeof globalThis & {
            webkitAudioContext?: typeof AudioContext;
          }
      ).webkitAudioContext)();
    const oscillator = audioContext.createOscillator();
    const gainNode = audioContext.createGain();

    oscillator.connect(gainNode);
    gainNode.connect(audioContext.destination);

    oscillator.frequency.value = 523.25;
    oscillator.type = 'sine';

    gainNode.gain.setValueAtTime(0.1, audioContext.currentTime);
    gainNode.gain.exponentialRampToValueAtTime(0.01, audioContext.currentTime + 0.5);

    oscillator.start(audioContext.currentTime);
    oscillator.stop(audioContext.currentTime + 0.5);
  } catch (error) {
    console.warn('[Web Session Completion Notifier] Failed to play sound', error);
  }
}

function handleCompletion(event: WebSessionAIEvent) {
  const sessionId = String(event?.sessionId || '').trim();
  if (!sessionId || notifiedSessions.has(sessionId)) {
    return;
  }

  notifiedSessions.add(sessionId);
  window.setTimeout(() => {
    notifiedSessions.delete(sessionId);
  }, NOTIFICATION_COOLDOWN);

  const projectName = resolveProjectName(event.projectId);
  const title = projectName
    ? `${t('terminal.aiCompleted')} - ${projectName}`
    : t('terminal.aiCompleted');
  const content = `${event.assistant.displayName} - ${event.sessionTitle}`;

  const instance = notification.success({
    title,
    content,
    action: renderNotificationAction(event),
    duration: 4000,
    closable: true,
    onClose: () => {
      activeNotifications.delete(sessionId);
    },
    onAfterLeave: () => {
      activeNotifications.delete(sessionId);
    },
  });

  activeNotifications.set(sessionId, instance);
  playCompletionSound();
}

function handleWorking(event: WebSessionAIEvent) {
  clearNotification(event?.sessionId);
}

function handleViewed(event: { sessionId?: string }) {
  clearNotification(event?.sessionId);
}

onMounted(() => {
  webSessionStore.emitter.on('ai:completed', handleCompletion);
  webSessionStore.emitter.on('ai:working', handleWorking);
  webSessionStore.emitter.on('ai:closed', handleWorking);
  webSessionStore.emitter.on('web-session:viewed', handleViewed);
});

onUnmounted(() => {
  webSessionStore.emitter.off('ai:completed', handleCompletion);
  webSessionStore.emitter.off('ai:working', handleWorking);
  webSessionStore.emitter.off('ai:closed', handleWorking);
  webSessionStore.emitter.off('web-session:viewed', handleViewed);

  activeNotifications.forEach(instance => instance.destroy());
  activeNotifications.clear();
});
</script>
