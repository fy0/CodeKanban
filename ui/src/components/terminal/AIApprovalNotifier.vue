<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue';
import { useNotification } from 'naive-ui';
import { useI18n } from 'vue-i18n';
import { useTerminalStore } from '@/stores/terminal';

const notification = useNotification();
const { t } = useI18n();
const terminalStore = useTerminalStore();

// Track already notified sessions to avoid duplicate notifications
const notifiedSessions = new Set<string>();
const NOTIFICATION_COOLDOWN = 3000; // 3 seconds cooldown

function handleAIApproval(event: any) {
  const { sessionId, sessionTitle, projectName, assistant } = event;

  // Prevent duplicate notifications for the same session within cooldown period
  const notificationKey = `${sessionId}:${Date.now()}`;
  if (notifiedSessions.has(sessionId)) {
    return;
  }

  notifiedSessions.add(sessionId);
  setTimeout(() => {
    notifiedSessions.delete(sessionId);
  }, NOTIFICATION_COOLDOWN);

  // Get assistant display name
  const assistantName = assistant?.displayName || assistant?.name || 'AI';

  // Build notification content with project name
  const content = projectName
    ? `[${projectName}] ${assistantName} ${t('terminal.isWaitingForApproval')} - ${sessionTitle}`
    : `${assistantName} ${t('terminal.isWaitingForApproval')} - ${sessionTitle}`;

  // Show warning notification for approval
  notification.warning({
    title: t('terminal.aiNeedsApproval'),
    content,
    duration: 6000, // Longer duration for approval requests
    closable: true,
  });
}

onMounted(() => {
  terminalStore.emitter.on('ai:approval-needed', handleAIApproval);
});

onUnmounted(() => {
  terminalStore.emitter.off('ai:approval-needed', handleAIApproval);
});
</script>

<template>
  <!-- This component has no UI, it only listens for events -->
</template>
