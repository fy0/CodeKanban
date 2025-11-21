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

function handleAICompletion(event: any) {
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
    ? `[${projectName}] ${assistantName} ${t('terminal.hasCompletedExecution')} - ${sessionTitle}`
    : `${assistantName} ${t('terminal.hasCompletedExecution')} - ${sessionTitle}`;

  // Show notification
  notification.success({
    title: t('terminal.aiCompleted'),
    content,
    duration: 4000,
    closable: true,
  });

  // Play completion sound
  playCompletionSound();
}

// Play a subtle completion sound
function playCompletionSound() {
  try {
    const audioContext = new (window.AudioContext || (window as any).webkitAudioContext)();
    const oscillator = audioContext.createOscillator();
    const gainNode = audioContext.createGain();

    oscillator.connect(gainNode);
    gainNode.connect(audioContext.destination);

    // Pleasant notification sound (C major chord)
    oscillator.frequency.value = 523.25; // C5
    oscillator.type = 'sine';

    gainNode.gain.setValueAtTime(0.1, audioContext.currentTime);
    gainNode.gain.exponentialRampToValueAtTime(0.01, audioContext.currentTime + 0.5);

    oscillator.start(audioContext.currentTime);
    oscillator.stop(audioContext.currentTime + 0.5);
  } catch (error) {
    console.warn('Failed to play completion sound:', error);
  }
}

onMounted(() => {
  terminalStore.emitter.on('ai:completed', handleAICompletion);
});

onUnmounted(() => {
  terminalStore.emitter.off('ai:completed', handleAICompletion);
});
</script>

<template>
  <!-- This component has no UI, it only listens for events -->
</template>
