<template>
  <div class="pty-test-page">
    <n-page-header>
      <template #title>
        <n-space align="center">
          <n-icon size="24">
            <TerminalOutline />
          </n-icon>
          <span>PTY 测试工具</span>
        </n-space>
      </template>
      <template #extra>
        <n-space>
          <n-button quaternary size="small" @click="goBack">
            返回项目
          </n-button>
          <n-button type="primary" size="small" :loading="starting" @click="startSession">
            启动测试终端
          </n-button>
          <n-button size="small" :disabled="!session" @click="stopSession">
            结束会话
          </n-button>
        </n-space>
      </template>
    </n-page-header>

    <n-alert type="warning" title="实验特性">
      该页面用于快速验证 xpty 的兼容性问题，不会影响现有终端功能。
    </n-alert>

    <n-card title="参数配置">
      <n-form :model="form" label-placement="left" label-width="96">
        <div class="config-grid">
          <n-form-item label="Shell 命令">
            <n-input v-model:value="form.shell" placeholder="留空使用默认配置" clearable />
          </n-form-item>
          <n-form-item label="工作目录">
            <n-input v-model:value="form.workingDir" placeholder="例如 D:\projects\demo" clearable />
          </n-form-item>
          <n-form-item label="行数">
            <n-input-number v-model:value="form.rows" :min="10" :max="80" />
          </n-form-item>
          <n-form-item label="列数">
            <n-input-number v-model:value="form.cols" :min="40" :max="200" />
          </n-form-item>
          <n-form-item label="xPTY 编码">
            <n-select v-model:value="form.ptyEncoding" :options="ptyEncodingOptions" />
          </n-form-item>
        </div>
      </n-form>
    </n-card>

    <n-card v-if="session" title="当前会话" size="small">
      <n-descriptions :column="1" label-placement="left">
        <n-descriptions-item label="会话 ID">
          <code>{{ session.id }}</code>
        </n-descriptions-item>
        <n-descriptions-item label="Shell">
          <code>{{ shellPreview }}</code>
        </n-descriptions-item>
        <n-descriptions-item label="工作目录">
          {{ session.workingDir || '继承服务目录' }}
        </n-descriptions-item>
        <n-descriptions-item label="创建时间">
          {{ formattedCreatedAt }}
        </n-descriptions-item>
        <n-descriptions-item label="WebSocket">
          <code>{{ session.wsPath }}</code>
        </n-descriptions-item>
        <n-descriptions-item label="xPTY 编码">
          {{ session.encoding || 'utf-8' }}
        </n-descriptions-item>
        <n-descriptions-item label="状态">
          <n-tag :type="statusTagType" size="small">{{ statusLabel }}</n-tag>
        </n-descriptions-item>
      </n-descriptions>
    </n-card>

    <div class="terminal-container">
      <div ref="terminalRef" class="terminal-shell"></div>
      <div v-if="overlayMessage" class="terminal-overlay">
        {{ overlayMessage }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useMessage } from 'naive-ui';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { WebglAddon } from '@xterm/addon-webgl';
import { SearchAddon } from '@xterm/addon-search';
import '@/styles/terminal.css';
import { http } from '@/api/http';
import { extractItem } from '@/api/response';
import { resolveWsUrl } from '@/utils/ws';
import { urlBase } from '@/api';
import { TerminalOutline } from '@vicons/ionicons5';

type SessionStatus = 'idle' | 'connecting' | 'ready' | 'closed' | 'error';

interface PtyTestSession {
  id: string;
  workingDir: string;
  shell: string[];
  rows: number;
  cols: number;
  createdAt: string;
  encoding?: string;
  wsPath: string;
  wsUrl?: string;
}

type ServerMessage = {
  type: 'ready' | 'data' | 'exit' | 'error';
  data?: string;
};

const router = useRouter();
const message = useMessage();
const ptyEncodingOptions = [
  { label: 'UTF-8', value: 'utf-8' },
  { label: 'GBK', value: 'gbk' },
  { label: 'GB18030', value: 'gb18030' },
] as const;
const form = reactive({
  shell: '',
  workingDir: '',
  rows: 24,
  cols: 80,
  ptyEncoding: 'utf-8',
});
const session = ref<PtyTestSession | null>(null);
const status = ref<SessionStatus>('idle');
const starting = ref(false);

const terminalRef = ref<HTMLDivElement | null>(null);
let terminal: Terminal | null = null;
let fitAddon: FitAddon | null = null;
let socket: WebSocket | null = null;

const shellPreview = computed(() => session.value?.shell.join(' ') || '默认');
const formattedCreatedAt = computed(() =>
  session.value ? new Date(session.value.createdAt).toLocaleString() : '--',
);

const overlayMessage = computed(() => {
  switch (status.value) {
    case 'connecting':
      return '正在连接 PTY…';
    case 'error':
      return '连接异常，请检查日志';
    case 'closed':
      return '会话已结束';
    default:
      return '';
  }
});

const statusLabel = computed(() => {
  switch (status.value) {
    case 'ready':
      return '已连接';
    case 'connecting':
      return '连接中';
    case 'error':
      return '异常';
    case 'closed':
      return '已关闭';
    default:
      return '未启动';
  }
});

const statusTagType = computed(() => {
  switch (status.value) {
    case 'ready':
      return 'success';
    case 'connecting':
      return 'warning';
    case 'error':
      return 'error';
    default:
      return 'default';
  }
});

function goBack() {
  router.push({ name: 'projects' });
}

async function startSession() {
  if (starting.value) {
    return;
  }
  starting.value = true;
  try {
    await cleanupSession(true);
    status.value = 'idle';
    const payload = await http.Post<PtyTestSession>('/pty-test/sessions', {
      shell: form.shell.trim(),
      workingDir: form.workingDir.trim(),
      rows: form.rows,
      cols: form.cols,
      encoding: form.ptyEncoding,
    }).send();
    const created = extractItem<PtyTestSession>(payload);
    if (!created) {
      throw new Error('服务未返回会话信息');
    }
    created.encoding = created.encoding || form.ptyEncoding;
    session.value = created;
    status.value = 'connecting';
    ensureTerminal();
    connectSocket(created);
    message.success('PTY 测试会话已创建');
  } catch (error: any) {
    status.value = 'error';
    message.error(error?.message ?? '创建会话失败');
  } finally {
    starting.value = false;
  }
}

async function stopSession() {
  if (!session.value) {
    return;
  }
  try {
    if (socket && socket.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify({ type: 'close' }));
    } else {
      await http.Delete(`/pty-test/sessions/${session.value.id}`).send();
    }
    message.success('已请求关闭会话');
  } catch (error: any) {
    message.warning(error?.message ?? '关闭请求发送失败');
  } finally {
    await cleanupSession(false);
  }
}

function ensureTerminal() {
  if (terminal) {
    terminal.reset();
    terminal.clear();
    return;
  }
  terminal = new Terminal({
    convertEol: true,
    rows: form.rows,
    cols: form.cols,
    cursorBlink: true,
    fontFamily: 'var(--kanban-font-mono, "Fira Code", monospace)',
    fontSize: 13,
    theme: {
      background: 'var(--kanban-terminal-bg, #0f111a)',
      foreground: 'var(--kanban-terminal-fg, #f6f8ff)',
    },
  });

  fitAddon = new FitAddon();
  const webLinksAddon = new WebLinksAddon();
  const searchAddon = new SearchAddon();
  const webglAddon = new WebglAddon();
  terminal.loadAddon(fitAddon);
  terminal.loadAddon(webLinksAddon);
  terminal.loadAddon(searchAddon);
  try {
    terminal.loadAddon(webglAddon);
  } catch {
    // WebGL may fail on some GPUs, ignore.
  }

  terminal.onData(data => {
    if (socket && socket.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify({ type: 'input', data }));
    }
  });

  const container = terminalRef.value;
  if (container) {
    terminal.open(container);
    fitAddon?.fit();
  }
}

function handleResize() {
  if (!terminal || !fitAddon || !session.value) {
    return;
  }
  fitAddon.fit();
  session.value.cols = terminal.cols;
  session.value.rows = terminal.rows;
  if (socket && socket.readyState === WebSocket.OPEN) {
    socket.send(
      JSON.stringify({
        type: 'resize',
        cols: terminal.cols,
        rows: terminal.rows,
      }),
    );
  }
}

function connectSocket(view: PtyTestSession) {
  const wsURL = resolveWsUrl(view.wsUrl || view.wsPath, urlBase);
  const ws = new WebSocket(wsURL);
  socket = ws;

  ws.onopen = () => {
    status.value = 'ready';
    handleResize();
    terminal?.focus();
  };

  ws.onmessage = event => {
    let payload: ServerMessage | null = null;
    try {
      payload = JSON.parse(event.data as string) as ServerMessage;
    } catch {
      return;
    }
    if (!terminal || !payload) {
      return;
    }
    switch (payload.type) {
      case 'data':
        if (payload.data) {
          terminal.write(decodeChunk(payload.data));
        }
        break;
      case 'exit':
        terminal.writeln(`\r\n[server] ${payload.data || 'session ended'}`);
        status.value = 'closed';
        break;
      case 'error':
        if (payload.data) {
          terminal.writeln(`\r\n[error] ${payload.data}`);
          status.value = 'error';
        }
        break;
      default:
        break;
    }
  };

  ws.onclose = () => {
    socket = null;
    if (status.value !== 'error') {
      status.value = 'closed';
    }
  };

  ws.onerror = () => {
    status.value = 'error';
  };
}

function decodeChunk(chunk: string) {
  if (!chunk) {
    return '';
  }
  const bytes = base64ToUint8Array(chunk);
  try {
    return new TextDecoder('utf-8').decode(bytes);
  } catch {
    return window.atob(chunk);
  }
}

function base64ToUint8Array(value: string) {
  const binary = window.atob(value);
  const len = binary.length;
  const bytes = new Uint8Array(len);
  for (let i = 0; i < len; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

async function cleanupSession(sendDelete: boolean) {
  if (socket) {
    socket.onopen = null;
    socket.onmessage = null;
    socket.onclose = null;
    socket.onerror = null;
    socket.close();
    socket = null;
  }

  if (sendDelete && session.value) {
    try {
      await http.Delete(`/pty-test/sessions/${session.value.id}`).send();
    } catch {
      // Ignore cleanup errors.
    }
  }
  session.value = null;
}

onMounted(() => {
  ensureTerminal();
  window.addEventListener('resize', handleResize);
});

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize);
  void cleanupSession(true);
  terminal?.dispose();
  terminal = null;
  fitAddon?.dispose();
  fitAddon = null;
});
</script>

<style scoped>
.pty-test-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 24px;
}

.config-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 12px 40px;
}

.terminal-container {
  border: 1px solid var(--n-border-color);
  border-radius: 6px;
  overflow: hidden;
  position: relative;
  min-height: 420px;
  background-color: var(--kanban-terminal-bg, #0f111a);
}

.terminal-shell {
  height: 420px;
}

.terminal-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.35);
  color: var(--kanban-terminal-fg, #f6f8ff);
  font-size: 13px;
  pointer-events: none;
}

code {
  font-family: var(--kanban-font-mono, 'Fira Code', monospace);
}
</style>
