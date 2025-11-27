<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import Apis from '@/api';
import { useTerminalStore } from '@/stores/terminal';
import { useProjectStore } from '@/stores/project';
import { getAssistantIconByType, getAssistantColorByType } from '@/utils/assistantIcon';

const { t } = useI18n();
const terminalStore = useTerminalStore();
const projectStore = useProjectStore();

// 通知开关状态
const NOTIFICATIONS_STORAGE_KEY = 'kanban-ai-notifications-enabled';
const CLICKED_NOTIFICATIONS_STORAGE_KEY = 'kanban-ai-notifications-clicked';
const notificationsEnabled = ref(true);
const clickedNotifications = ref<Set<string>>(new Set());

// 从 localStorage 加载设置
function loadNotificationSettings() {
  try {
    const stored = localStorage.getItem(NOTIFICATIONS_STORAGE_KEY);
    if (stored !== null) {
      notificationsEnabled.value = stored === 'true';
    }
  } catch (error) {
    console.warn('[AI Notification] Failed to load notification settings', error);
  }
}

// 加载已点击的通知记录
function loadClickedNotifications() {
  try {
    const stored = localStorage.getItem(CLICKED_NOTIFICATIONS_STORAGE_KEY);
    if (stored) {
      const parsed = JSON.parse(stored);
      clickedNotifications.value = new Set(Array.isArray(parsed) ? parsed : []);
    }
  } catch (error) {
    console.warn('[AI Notification] Failed to load clicked notifications', error);
  }
}

// 保存设置到 localStorage
function saveNotificationSettings() {
  try {
    localStorage.setItem(NOTIFICATIONS_STORAGE_KEY, String(notificationsEnabled.value));
  } catch (error) {
    console.warn('[AI Notification] Failed to save notification settings', error);
  }
}

// 保存已点击的通知记录
function saveClickedNotifications() {
  try {
    localStorage.setItem(
      CLICKED_NOTIFICATIONS_STORAGE_KEY,
      JSON.stringify(Array.from(clickedNotifications.value))
    );
  } catch (error) {
    console.warn('[AI Notification] Failed to save clicked notifications', error);
  }
}

function markNotificationsAsRead(notificationIds: string[]) {
  let changed = false;
  notificationIds.forEach((id) => {
    if (id && !clickedNotifications.value.has(id)) {
      clickedNotifications.value.add(id);
      changed = true;
    }
  });
  if (changed) {
    saveClickedNotifications();
  }
}

function clearReadStateForNotifications(notificationIds: string[]) {
  let changed = false;
  notificationIds.forEach((id) => {
    if (id && clickedNotifications.value.delete(id)) {
      changed = true;
    }
  });
  if (changed) {
    saveClickedNotifications();
  }
}

function markSessionCompletionNotificationsAsRead(sessionId: string) {
  if (!sessionId) {
    return;
  }
  const ids = notifications.value
    .filter((notification) => notification.type === 'completion' && notification.sessionId === sessionId)
    .map((notification) => notification.id);
  if (ids.length) {
    markNotificationsAsRead(ids);
  }
}

// 切换通知开关
function toggleNotifications() {
  notificationsEnabled.value = !notificationsEnabled.value;
  saveNotificationSettings();
}

// 检查通知是否被点击过
function isNotificationClicked(notificationId: string): boolean {
  return clickedNotifications.value.has(notificationId);
}

interface NotificationItem {
  id: string;
  recordId: string;
  type: 'completion' | 'approval';
  sessionId: string;
  projectId: string;
  projectName?: string;
  worktreeId?: string;
  branchName?: string;
  title: string;
  assistantName: string;
  assistantType?: string;
  assistantIcon?: string;
  assistantColor?: string;
  timestamp: Date;
  state?: 'completed' | 'working';
}

type NotificationType = 'completion' | 'approval';

interface AssistantInfo {
  type?: string;
  name?: string;
  displayName?: string;
}

interface CompletionRecordResponse {
  id: string;
  sessionId: string;
  projectId: string;
  projectName?: string;
  title: string;
  assistant?: AssistantInfo;
  completedAt?: string;
  dismissed?: boolean;
  state?: 'completed' | 'working';
}

interface ApprovalRecordResponse {
  id: string;
  sessionId: string;
  projectId: string;
  projectName?: string;
  title: string;
  assistant?: AssistantInfo;
  requestedAt?: string;
  dismissed?: boolean;
}

const defaultAssistantIcon = getAssistantIconByType();
const defaultAssistantColor = getAssistantColorByType();
const worktreeBranchCache = new Map<string, { branchName: string; projectId?: string }>();
const router = useRouter();
const currentRoute = useRoute();
const notifications = ref<NotificationItem[]>([]);
const isFetchingCompletions = ref(false);
const isFetchingApprovals = ref(false);

// 根据开关状态过滤通知
const visibleNotifications = computed(() => {
  return notificationsEnabled.value ? notifications.value : [];
});

watch(
  () =>
    projectStore.worktrees.map((worktree) => ({
      id: worktree.id,
      branchName: worktree.branchName,
      projectId: worktree.projectId,
    })),
  (entries) => {
    entries.forEach(({ id, branchName, projectId }) => {
      if (id && branchName) {
        worktreeBranchCache.set(id, { branchName, projectId });
      }
    });
  },
  { deep: true, immediate: true },
);

function resolveBranchName(projectId?: string, worktreeId?: string) {
  if (!worktreeId) {
    return undefined;
  }
  const cached = worktreeBranchCache.get(worktreeId);
  if (cached && (!projectId || !cached.projectId || cached.projectId === projectId)) {
    return cached.branchName;
  }
  const match = projectStore.worktrees.find((worktree) => worktree.id === worktreeId);
  if (match?.branchName) {
    worktreeBranchCache.set(worktreeId, { branchName: match.branchName, projectId: match.projectId });
    return match.branchName;
  }
  return undefined;
}

function getLocationLabel(notification: NotificationItem) {
  return notification.branchName || notification.projectName || '';
}

function getCompletionHeader(notification: NotificationItem) {
  const projectLabel = notification.projectName || getLocationLabel(notification);
  const titleKey = notification.state === 'working' ? 'terminal.aiWorking' : 'terminal.aiCompleted';
  const baseTitle = t(titleKey);
  return projectLabel ? `${baseTitle} - ${projectLabel}` : baseTitle;
}

function getApprovalHeader(notification: NotificationItem) {
  const projectLabel = notification.projectName || getLocationLabel(notification);
  return projectLabel ? `${t('terminal.aiNeedsApproval')} - ${projectLabel}` : t('terminal.aiNeedsApproval');
}

function getNotificationHeader(notification: NotificationItem) {
  return notification.type === 'completion'
    ? getCompletionHeader(notification)
    : getApprovalHeader(notification);
}

function formatCompletionBody(notification: NotificationItem) {
  return notification.title;
}

function getNotificationDescription(notification: NotificationItem) {
  const location = getLocationLabel(notification);
  const body =
    notification.type === 'completion'
      ? formatCompletionBody(notification)
      : `${t('terminal.isWaitingForApproval')} - ${notification.title}`;
  return location ? `[${location}] ${body}` : body;
}

function getAssistantName(info?: AssistantInfo) {
  return info?.displayName || info?.name || 'AI';
}

function getProjectNameById(projectId?: string, fallback?: string) {
  if (!projectId) {
    return fallback;
  }
  const project = projectStore.projects.find((p) => p.id === projectId);
  return project?.name || fallback;
}

function mapCompletionRecord(record: CompletionRecordResponse): NotificationItem {
  const session = terminalStore.getSessionById(record.sessionId);
  const worktreeId = session?.worktreeId;
  const branchName = resolveBranchName(record.projectId, worktreeId);
  const assistantType = record.assistant?.type;

  return {
    id: record.id,
    recordId: record.id,
    type: 'completion',
    sessionId: record.sessionId,
    projectId: record.projectId,
    projectName: record.projectName || getProjectNameById(record.projectId),
    worktreeId,
    branchName,
    title: record.title || session?.title || 'AI Session',
    assistantName: getAssistantName(record.assistant),
    assistantType,
    assistantIcon: getAssistantIconByType(assistantType),
    assistantColor: getAssistantColorByType(assistantType),
    timestamp: record.completedAt ? new Date(record.completedAt) : new Date(),
    state: record.state === 'working' ? 'working' : 'completed',
  };
}

function mapApprovalRecord(record: ApprovalRecordResponse): NotificationItem {
  const session = terminalStore.getSessionById(record.sessionId);
  const worktreeId = session?.worktreeId;
  const branchName = resolveBranchName(record.projectId, worktreeId);
  const assistantType = record.assistant?.type;

  return {
    id: record.id,
    recordId: record.id,
    type: 'approval',
    sessionId: record.sessionId,
    projectId: record.projectId,
    projectName: record.projectName || getProjectNameById(record.projectId),
    worktreeId,
    branchName,
    title: record.title || session?.title || 'AI Session',
    assistantName: getAssistantName(record.assistant),
    assistantType,
    assistantIcon: getAssistantIconByType(assistantType),
    assistantColor: getAssistantColorByType(assistantType),
    timestamp: record.requestedAt ? new Date(record.requestedAt) : new Date(),
  };
}

function sortNotifications(list: NotificationItem[]) {
  return [...list].sort((a, b) => b.timestamp.getTime() - a.timestamp.getTime());
}

function setNotificationsForType(type: NotificationType, items: NotificationItem[]) {
  const others = notifications.value.filter((item) => item.type !== type);
  notifications.value = sortNotifications([...others, ...items]);
  if (type === 'completion') {
    autoMarkActiveCompletionNotifications();
  }
}

function getActiveSessionIds() {
  const activeSessions = new Set<string>();
  projectStore.projects.forEach((project) => {
    const activeSessionId = terminalStore.getActiveTabId(project.id);
    if (activeSessionId) {
      activeSessions.add(activeSessionId);
    }
  });
  return activeSessions;
}

function autoMarkActiveCompletionNotifications() {
  const activeSessions = getActiveSessionIds();
  if (!activeSessions.size) {
    return;
  }
  const idsToMark = notifications.value
    .filter((notification) => notification.type === 'completion' && activeSessions.has(notification.sessionId))
    .map((notification) => notification.id);
  if (idsToMark.length) {
    markNotificationsAsRead(idsToMark);
  }
}

function removeNotificationLocally(recordId: string) {
  notifications.value = notifications.value.filter((item) => item.recordId !== recordId);
}

function handleTerminalViewedEvent(event: any) {
  const sessionId = event?.sessionId;
  if (!sessionId) {
    return;
  }
  markSessionCompletionNotificationsAsRead(sessionId);
}

function getNotificationClass(notification: NotificationItem) {
  if (notification.type === 'completion') {
    return notification.state === 'working' ? 'notification-working' : 'notification-completion';
  }
  return 'notification-approval';
}

function handleAIWorking(event: any) {
  const { sessionId } = event || {};
  if (!sessionId) {
    return;
  }

  const updatedIds: string[] = [];
  let changed = false;

  notifications.value = notifications.value.map((notification) => {
    if (notification.type === 'completion' && notification.sessionId === sessionId) {
      if (notification.state !== 'working') {
        changed = true;
        updatedIds.push(notification.id);
        return {
          ...notification,
          state: 'working',
        };
      }
    }
    return notification;
  });

  if (updatedIds.length) {
    clearReadStateForNotifications(updatedIds);
  }

  if (changed) {
    console.log('[AI Notification] Marked completion as working', { sessionId });
  } else {
    // 如果当前列表里没有对应记录（可能是第一次就进入 working 状态），主动刷新
    void fetchCompletionRecords();
  }
}

async function fetchCompletionRecords(options?: { playSound?: boolean }) {
  if (isFetchingCompletions.value) {
    return;
  }
  isFetchingCompletions.value = true;
  const existingIds = new Set(
    notifications.value.filter((item) => item.type === 'completion').map((item) => item.recordId),
  );
  try {
    const response = await Apis.terminalSession.terminalCompletionRecordsList({ cacheFor: 0 }).send();
    const records = (response?.items ?? []) as CompletionRecordResponse[];
    const items = records.filter((record) => !record.dismissed).map(mapCompletionRecord);
    setNotificationsForType('completion', items);
    if (options?.playSound && items.some((item) => !existingIds.has(item.recordId))) {
      playCompletionSound();
    }
  } catch (error) {
    console.error('[AI Notification] Failed to load completion records', error);
  } finally {
    isFetchingCompletions.value = false;
  }
}

async function fetchApprovalRecords() {
  if (isFetchingApprovals.value) {
    return;
  }
  isFetchingApprovals.value = true;
  try {
    const response = await Apis.terminalSession.terminalApprovalRecordsList({ cacheFor: 0 }).send();
    const records = (response?.items ?? []) as ApprovalRecordResponse[];
    const items = records.filter((record) => !record.dismissed).map(mapApprovalRecord);

    // 获取所有审批通知的 sessionId 集合
    const approvalSessionIds = new Set(items.map(item => item.sessionId));

    // 找到需要被顶掉的完成通知
    const completionsToRemove = notifications.value.filter(
      item => item.type === 'completion' && approvalSessionIds.has(item.sessionId)
    );

    // 真正地 dismiss 这些完成通知（调用后端 API）
    for (const completion of completionsToRemove) {
      try {
        await Apis.terminalSession
          .terminalCompletionRecordDismiss({
            pathParams: { recordId: completion.recordId },
            cacheFor: 0,
          })
          .send();
      } catch (error) {
        console.error('[AI Notification] Failed to dismiss completion record', completion.recordId, error);
      }
    }

    // 从前端列表中移除这些完成通知
    notifications.value = notifications.value.filter(
      item => !(item.type === 'completion' && approvalSessionIds.has(item.sessionId))
    );

    setNotificationsForType('approval', items);
  } catch (error) {
    console.error('[AI Notification] Failed to load approval records', error);
  } finally {
    isFetchingApprovals.value = false;
  }
}

// 播放完成提示音
function playCompletionSound() {
  try {
    const audioContext = new (window.AudioContext || (window as any).webkitAudioContext)();
    const oscillator = audioContext.createOscillator();
    const gainNode = audioContext.createGain();

    oscillator.connect(gainNode);
    gainNode.connect(audioContext.destination);

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

// 处理完成事件
function handleAICompletion() {
  window.setTimeout(() => {
    void fetchCompletionRecords({ playSound: true });
  }, 150);
}

// 处理审批事件
function handleAIApproval() {
  window.setTimeout(() => {
    void fetchApprovalRecords();
  }, 150);
}

// 点击通知，切换到对应的终端
async function handleNotificationClick(notification: NotificationItem) {
  // 记录该通知已被点击
  markNotificationsAsRead([notification.id]);

  const targetProjectId = notification.projectId;
  if (!targetProjectId) {
    return;
  }

  const currentProjectId =
    typeof currentRoute.params.id === 'string' ? currentRoute.params.id : '';

  if (currentProjectId !== targetProjectId) {
    try {
      await router.push({ name: 'project', params: { id: targetProjectId } });
      await nextTick();
    } catch (error) {
      console.error('[AI Notification] Failed to switch project for notification', error);
    }
  }

  // Ensure the terminal panel is visible when jumping from notifications
  terminalStore.emitter.emit('terminal:ensure-expanded', {
    projectId: targetProjectId,
  });

  // 切换到对应的终端标签
  terminalStore.setActiveTab(targetProjectId, notification.sessionId);
}

// 关闭通知
async function dismissNotification(notification: NotificationItem) {
  try {
    if (notification.type === 'completion') {
      await Apis.terminalSession
        .terminalCompletionRecordDismiss({
          pathParams: { recordId: notification.recordId },
          cacheFor: 0,
        })
        .send();
    } else {
      await Apis.terminalSession
        .terminalApprovalRecordDismiss({
          pathParams: { recordId: notification.recordId },
          cacheFor: 0,
        })
        .send();
    }
    removeNotificationLocally(notification.recordId);
  } catch (error) {
    console.error('[AI Notification] Failed to dismiss record', error);
  }
}

// 跟踪每个 session 的 AI 助手状态（用于检测 AI 助手的移除）
const sessionHasAI = new Map<string, boolean>();

// 监听终端元数据更新，检测状态变化
function handleMetadataUpdate(payload: any) {
  const sessionId = payload.sessionId;
  const aiAssistant = payload.metadata?.aiAssistant;
  const aiState = aiAssistant?.state;

  const hadAI = sessionHasAI.get(sessionId) ?? false;
  const hasAI = !!aiAssistant;

  // 更新状态跟踪
  sessionHasAI.set(sessionId, hasAI);

  // 检测 AI 助手被移除（从有变为无）
  if (hadAI && !hasAI) {
    // AI 助手已关闭，刷新通知列表以移除相关通知
    void fetchCompletionRecords();
    void fetchApprovalRecords();
    return;
  }

  if (aiState && aiState !== 'waiting_approval') {
    void fetchApprovalRecords();
  }

  // 注意：不需要在 working 状态时获取完成记录
  // 因为后端在创建新的完成记录前会自动清除该 session 的旧记录
  // 完成通知只应该在 ai:completed 事件时才获取
}

// 监听 session 关闭，清除对应的所有通知
function handleSessionClose(sessionId: string) {
  // 清理状态跟踪
  sessionHasAI.delete(sessionId);

  void fetchCompletionRecords();
  void fetchApprovalRecords();
}

// 已订阅的 session IDs
const subscribedSessions = new Set<string>();

// 订阅单个 session 的事件
function subscribeToSession(sessionId: string) {
  if (subscribedSessions.has(sessionId)) {
    return; // 已经订阅过
  }

  terminalStore.emitter.on(sessionId, (payload: any) => {
    if (payload.type === 'metadata') {
      handleMetadataUpdate({ sessionId, metadata: payload.metadata });
    } else if (payload.type === 'exit' || payload.type === 'closed') {
      handleSessionClose(sessionId);
      // 取消订阅已关闭的 session
      subscribedSessions.delete(sessionId);
    }
  });

  subscribedSessions.add(sessionId);
}

// 取消订阅单个 session
function unsubscribeFromSession(sessionId: string) {
  terminalStore.emitter.off(sessionId);
  subscribedSessions.delete(sessionId);
  sessionHasAI.delete(sessionId);
}

// 订阅所有终端的元数据更新事件
function subscribeToAllSessions() {
  const allProjects = projectStore.projects;
  const currentSessionIds = new Set<string>();

  // 收集当前所有的 session IDs
  allProjects.forEach((project) => {
    const tabs = terminalStore.getTabs(project.id);
    tabs.forEach((tab) => {
      currentSessionIds.add(tab.id);
      subscribeToSession(tab.id);
    });
  });

  // 取消订阅已不存在的 sessions
  subscribedSessions.forEach((sessionId) => {
    if (!currentSessionIds.has(sessionId)) {
      unsubscribeFromSession(sessionId);
    }
  });
}

onMounted(() => {
  // 加载通知设置
  loadNotificationSettings();
  loadClickedNotifications();

  terminalStore.emitter.on('ai:completed', handleAICompletion);
  terminalStore.emitter.on('ai:approval-needed', handleAIApproval);
  terminalStore.emitter.on('ai:working', handleAIWorking);
  terminalStore.emitter.on('terminal:viewed', handleTerminalViewedEvent);

  void fetchCompletionRecords();
  void fetchApprovalRecords();

  // 订阅所有终端的状态变化
  subscribeToAllSessions();
});

onUnmounted(() => {
  terminalStore.emitter.off('ai:completed', handleAICompletion);
  terminalStore.emitter.off('ai:approval-needed', handleAIApproval);
  terminalStore.emitter.off('ai:working', handleAIWorking);
  terminalStore.emitter.off('terminal:viewed', handleTerminalViewedEvent);

  // 取消订阅所有终端
  subscribedSessions.forEach((sessionId) => {
    terminalStore.emitter.off(sessionId);
  });
  subscribedSessions.clear();
  sessionHasAI.clear();
});

// Watch session 列表变化，动态订阅/取消订阅
watch(
  () => {
    const allSessions: string[] = [];
    projectStore.projects.forEach((project) => {
      const tabs = terminalStore.getTabs(project.id);
      allSessions.push(...tabs.map((t) => t.id));
    });
    return allSessions.join(',');
  },
  () => {
    // Session 列表变化时重新订阅
    subscribeToAllSessions();
  }
);
</script>

<template>
  <div class="notification-bar-container">
    <!-- 通知开关按钮 -->
    <button
      class="notification-toggle-btn"
      @click="toggleNotifications"
      :title="notificationsEnabled ? t('terminal.disableNotifications') : t('terminal.enableNotifications')"
    >
      <svg
        v-if="notificationsEnabled"
        xmlns="http://www.w3.org/2000/svg"
        width="18"
        height="18"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"></path>
        <path d="M13.73 21a2 2 0 0 1-3.46 0"></path>
      </svg>
      <svg
        v-else
        xmlns="http://www.w3.org/2000/svg"
        width="18"
        height="18"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M6.3 5.3a1 1 0 0 0-1.4 1.4l1.5 1.5A6 6 0 0 0 6 10c0 7-3 9-3 9h14"></path>
        <path d="m21.7 18.7-1.6-1.6"></path>
        <path d="M2 2l20 20"></path>
        <path d="M8.7 3a6 6 0 0 1 10.3 5c0 1-.1 1.9-.4 2.7"></path>
      </svg>
    </button>

    <transition-group name="notification-slide" tag="div" class="notification-list">
      <div
        v-for="notification in visibleNotifications"
        :key="notification.id"
        :class="[
          'notification-item',
          getNotificationClass(notification),
          { 'notification-clicked': isNotificationClicked(notification.id) }
        ]"
        @click="handleNotificationClick(notification)"
      >
        <div class="notification-content">
          <div class="notification-header">
            <span
              class="notification-icon"
              :style="{ color: notification.assistantColor || defaultAssistantColor }"
              v-html="notification.assistantIcon || defaultAssistantIcon"
            ></span>
            <span class="notification-title">
              {{
                getNotificationHeader(notification)
              }}
            </span>
          </div>
          <div class="notification-body">
            <n-popover
              trigger="hover"
              :delay="1500"
              placement="bottom-end"
              :show-arrow="false"
              class="notification-popover"
            >
              <template #trigger>
                <div class="notification-description">
                  <span v-if="getLocationLabel(notification)" class="project-badge">
                    [{{ getLocationLabel(notification) }}]
                  </span>
                  <span class="notification-text">
                    <template v-if="notification.type === 'completion'">
                      {{ formatCompletionBody(notification) }}
                    </template>
                    <template v-else>
                      {{ t('terminal.isWaitingForApproval') }} - {{ notification.title }}
                    </template>
                  </span>
                </div>
              </template>
              <div class="notification-detail-text">
                {{ getNotificationDescription(notification) }}
              </div>
            </n-popover>
            <div class="notification-action-hint">
              {{ t('terminal.clickToJumpTerminal') }}
            </div>
          </div>
        </div>
        <button
          class="notification-close"
          @click.stop="dismissNotification(notification)"
          :title="t('common.close')"
        >
          ×
        </button>
      </div>
    </transition-group>
  </div>
</template>

<style scoped>
.notification-bar-container {
  position: fixed;
  top: 6px;
  right: 8px;
  z-index: 5;
  pointer-events: none;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 8px;
}

.notification-toggle-btn {
  width: 36px;
  height: 32px;
  border-radius: 6px;
  border: 1px solid var(--kanban-notification-button-border, rgba(0, 0, 0, 0.2));
  background: var(--app-surface-color, var(--body-color, #ffffff));
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  cursor: pointer;
  pointer-events: auto;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--kanban-notification-button-fg, var(--text-color, #000000));
  transition: all 0.2s ease;
  opacity: 0.85;
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}

.notification-toggle-btn:hover {
  opacity: 1;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.2);
  transform: scale(1.05);
  border-color: var(--n-color-primary, #3b82f6);
}

.notification-toggle-btn svg {
  display: block;
}

.notification-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  width: min(320px, calc(100vw - 32px));
  max-width: 360px;
}

.notification-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 10px 14px;
  background: var(--app-surface-color, var(--body-color, #ffffff));
  border-radius: 12px;
  box-shadow: 0 12px 28px rgba(15, 23, 42, 0.18);
  cursor: pointer;
  pointer-events: auto;
  transition: transform 0.2s ease, box-shadow 0.2s ease;
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-left: 4px solid transparent;
  min-width: 320px;
  width: 100%;
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
}

.notification-item:hover {
  transform: translateX(-4px);
  box-shadow: 0 16px 32px rgba(15, 23, 42, 0.22);
}

.notification-completion {
  --notification-completion-fill: var(--kanban-terminal-tab-completion-bg, rgba(16, 185, 129, 0.3));
  --notification-completion-accent: var(--kanban-terminal-tab-completion-border, rgba(16, 185, 129, 0.6));
  background: #d1fae5;
  border-color: rgba(16, 185, 129, 0.3);
  border-left-color: #10b981;
  box-shadow: 0 12px 28px rgba(16, 185, 129, 0.15);
}

/* 已点击过的完成通知样式 - 左侧提示条变黑灰色，背景变白色 */
.notification-completion.notification-clicked {
  border-left-color: #9ca3af !important;
  background: #ffffff !important;
  box-shadow: 0 12px 28px rgba(15, 23, 42, 0.12) !important;
}

/* 工作中 / 审批通知在已读后保持原样 */
.notification-approval {
  --notification-approval-fill: var(--kanban-terminal-tab-approval-bg, rgba(247, 144, 9, 0.25));
  --notification-approval-accent: var(--kanban-terminal-tab-approval-border, rgba(247, 144, 9, 0.55));
  background: #fed7aa;
  border-color: rgba(247, 144, 9, 0.3);
  border-left-color: #f79009;
  box-shadow: 0 12px 28px rgba(247, 144, 9, 0.15);
}

.notification-working {
  --notification-working-fill: var(--kanban-terminal-tab-working-bg, rgba(237, 233, 254, 1));
  --notification-working-accent: var(--kanban-terminal-tab-working-border, rgba(139, 92, 246, 1));
  background: var(--notification-working-fill);
  border-color: rgba(139, 92, 246, 0.3);
  border-left-color: var(--notification-working-accent);
  box-shadow: 0 12px 28px rgba(139, 92, 246, 0.15);
}

.notification-content {
  flex: 1;
  min-width: 0;
}

.notification-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
  font-weight: 600;
  font-size: 14px;
}

.notification-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--text-color, #000000);
}

.notification-icon :deep(svg) {
  display: block;
  width: 16px;
  height: 16px;
}

.notification-completion .notification-icon {
  color: var(--notification-completion-accent, rgba(16, 185, 129, 1));
}

.notification-approval .notification-icon {
  color: var(--notification-approval-accent, rgba(247, 144, 9, 1));
}

.notification-working .notification-icon {
  color: var(--notification-working-accent, rgba(139, 92, 246, 1));
}

.notification-title {
  color: var(--text-color, #000000);
}

.notification-body {
  font-size: 13px;
  color: var(--text-color-secondary, #666666);
  line-height: 1.3;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.notification-description {
  display: flex;
  align-items: baseline;
  flex-wrap: nowrap;
  width: 100%;
  min-width: 0;
  gap: 4px;
}

.notification-action-hint {
  font-size: 12px;
  color: var(--n-color-primary, #3b82f6);
  font-weight: 500;
}

.project-badge {
  font-weight: 500;
  color: var(--text-color, #000000);
  flex-shrink: 0;
}

.notification-text {
  display: inline-block;
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.notification-close {
  flex-shrink: 0;
  width: 20px;
  height: 20px;
  border: none;
  background: transparent;
  font-size: 20px;
  line-height: 1;
  cursor: pointer;
  color: var(--text-color-secondary, #666666);
  opacity: 0.6;
  transition: opacity 0.2s ease;
  padding: 0;
}

.notification-close:hover {
  opacity: 1;
}

.notification-detail-text {
  max-width: 420px;
  font-size: 13px;
  line-height: 1.4;
  color: var(--text-color, #000);
  word-break: break-word;
}

.notification-popover :deep(.n-popover__content) {
  padding: 10px 12px;
}

/* 动画 */
.notification-slide-enter-active {
  animation: slide-in 0.3s ease;
}

.notification-slide-leave-active {
  animation: slide-out 0.3s ease;
}

@keyframes slide-in {
  from {
    opacity: 0;
    transform: translateX(100%);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}

@keyframes slide-out {
  from {
    opacity: 1;
    transform: translateX(0);
  }
  to {
    opacity: 0;
    transform: translateX(100%);
  }
}
</style>
