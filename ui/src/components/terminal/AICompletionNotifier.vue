<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue';
import { useNotification } from 'naive-ui';
import { useI18n } from 'vue-i18n';
import { useTerminalStore } from '@/stores/terminal';
import type { NotificationReactive } from 'naive-ui';

const notification = useNotification();
const { t } = useI18n();
const terminalStore = useTerminalStore();

// Track already notified sessions to avoid duplicate notifications
const notifiedSessions = new Set<string>();
const NOTIFICATION_COOLDOWN = 3000; // 3 seconds cooldown

// Track active notifications by session ID so we can destroy them when agent closes
const activeNotifications = new Map<string, NotificationReactive>();

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

  // Show notification and track it
  const notificationInstance = notification.success({
    title: t('terminal.aiCompleted'),
    content,
    duration: 4000,
    closable: true,
    onClose: () => {
      // Clean up when notification is closed
      activeNotifications.delete(sessionId);
    },
    onLeave: () => {
      // Clean up when notification animation completes
      activeNotifications.delete(sessionId);
    },
  });

  // Store notification instance so we can destroy it later
  activeNotifications.set(sessionId, notificationInstance);

  // Play completion sound
  playCompletionSound();
}

function handleAIClosed(event: any) {
  const { sessionId } = event;

  // Destroy the notification for this session if it exists
  const notificationInstance = activeNotifications.get(sessionId);
  if (notificationInstance) {
    notificationInstance.destroy();
    activeNotifications.delete(sessionId);
    console.log(`[AICompletionNotifier] Cleared notification for session ${sessionId}`);
  }
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
  terminalStore.emitter.on('ai:closed', handleAIClosed);
});

onUnmounted(() => {
  terminalStore.emitter.off('ai:completed', handleAICompletion);
  terminalStore.emitter.off('ai:closed', handleAIClosed);

  // Clean up all active notifications when component is unmounted
  activeNotifications.forEach(notification => {
    notification.destroy();
  });
  activeNotifications.clear();
});
</script>

<template>
  <!-- This component has no UI, it only listens for events -->
</template>
