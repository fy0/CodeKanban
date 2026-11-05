// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount, type VueWrapper } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { reactive } from 'vue';
import EventEmitter from 'eventemitter3';
import type {
  ServerMessage,
  TerminalTabState,
  TerminalRemoteSnapshot,
  TerminalSerializedSnapshot,
} from '@/stores/terminal';
import TerminalViewport from '../TerminalViewport.vue';

const { store, terminals } = vi.hoisted(() => ({
  store: {
    prepareViewport: vi.fn(),
    replayBufferedMessages: vi.fn(() => ({ count: 0 })),
    saveSerializedSnapshot: vi.fn(),
    getSerializedSnapshot: vi.fn(),
    getLatestServerSnapshot: vi.fn(),
    clearSnapshotState: vi.fn(),
  },
  terminals: [] as Array<{
    content: string;
    cols: number;
    rows: number;
    disposed: boolean;
    reset: () => void;
    input: (data: string) => void;
  }>,
}));

vi.mock('@/stores/terminal', () => ({ useTerminalStore: () => store }));
vi.mock('@/stores/settings', async () => {
  const { defineStore } = await import('pinia');
  return {
    DEFAULT_TERMINAL_FONT_FAMILY: 'monospace',
    useSettingsStore: defineStore('viewport-test-settings', {
      state: () => ({
        activeTerminalTheme: {},
        terminalFont: { fontSize: 14 },
        terminalWebGLRenderer: 'disable',
      }),
    }),
  };
});
vi.mock('naive-ui', () => ({ useMessage: () => ({ warning: vi.fn(), error: vi.fn() }) }));
vi.mock('@/composables/useLocale', () => ({ useLocale: () => ({ t: (key: string) => key }) }));
vi.mock('@/composables/useMobileKeyboard', () => ({
  useMobileKeyboard: () => ({ setFocused: vi.fn(), sync: () => ({ isResizeFrozen: false }) }),
}));
vi.mock('@/utils/terminalImageUpload', () => ({
  formatTerminalPathInput: vi.fn(),
  uploadTerminalImage: vi.fn(),
}));
vi.mock('@xterm/xterm', () => ({
  Terminal: class {
    content = '';
    disposed = false;
    rows: number;
    cols: number;
    options: object;
    textarea = document.createElement('textarea');
    buffer = { active: { length: 24 } };
    input = (_data: string) => {};
    constructor(options: { rows: number; cols: number }) {
      this.options = options;
      this.rows = options.rows;
      this.cols = options.cols;
      terminals.push(this);
    }
    open() {}
    loadAddon(addon: { activate?: (terminal: unknown) => void }) {
      addon.activate?.(this);
    }
    onData(listener: (data: string) => void) {
      this.input = listener;
    }
    attachCustomKeyEventHandler() {}
    write(data: string, callback: () => void) {
      this.content += data;
      callback();
    }
    writeln(data: string, callback: () => void) {
      this.content += data + '\r\n';
      callback();
    }
    reset() {
      this.content = '';
    }
    resize(cols: number, rows: number) {
      this.cols = cols;
      this.rows = rows;
    }
    scrollToBottom() {}
    scrollToTop() {}
    clearTextureAtlas() {}
    refresh() {}
    focus() {}
    blur() {}
    dispose() {
      this.disposed = true;
    }
  },
}));
vi.mock('@xterm/addon-fit', () => ({
  FitAddon: class {
    fit() {}
    dispose() {}
  },
}));
vi.mock('@xterm/addon-serialize', () => ({
  SerializeAddon: class {
    terminal?: { content: string };
    activate(terminal: { content: string }) {
      this.terminal = terminal;
    }
    serialize() {
      return this.terminal?.content;
    }
    dispose() {}
  },
}));
vi.mock('@xterm/addon-webgl', () => ({ WebglAddon: class {} }));
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: class {} }));
vi.mock('@xterm/addon-search', () => ({ SearchAddon: class {} }));

let wrapper: VueWrapper;
let emitter: EventEmitter;
let tab: TerminalTabState;
const send = vi.fn();

async function settle() {
  for (let i = 0; i < 40; i++) await Promise.resolve();
}

function create(mode: 'live' | 'snapshot' = 'live', historyReplayEnabled = true) {
  tab = reactive({
    id: 'restore-test',
    title: 'Terminal',
    projectId: 'p1',
    rows: 24,
    cols: 80,
    clientStatus: 'ready',
    renderMode: mode,
    connectionRenderMode: mode,
    connectionRole: 'active',
  } as TerminalTabState);
  emitter = new EventEmitter();
  wrapper = mount(TerminalViewport, {
    props: { tab, emitter, send, shouldAutoFocus: false, historyReplayEnabled },
  });
  return terminals.at(-1)!;
}

function emit(payload: ServerMessage) {
  emitter.emit(tab.id, payload);
}
function data(text: string) {
  emit({ type: 'data', data: btoa(text) });
}
function snapshot(content: string): TerminalRemoteSnapshot {
  return {
    kind: 'full',
    content,
    rows: 32,
    cols: 120,
    sequence: 1,
    baseSequence: 0,
    altScreen: false,
    cursorVisible: true,
    modeFlags: 0,
    capturedAt: new Date().toISOString(),
  };
}

beforeEach(() => {
  setActivePinia(createPinia());
  vi.useFakeTimers();
  vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockReturnValue(800);
  vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(600);
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
  terminals.length = 0;
  vi.clearAllMocks();
  store.getSerializedSnapshot.mockReturnValue(null);
  store.getLatestServerSnapshot.mockReturnValue(null);
});

afterEach(() => {
  wrapper?.unmount();
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe('terminal renderer restore', () => {
  it('preserves the existing live screen on reconnect when server history replay is disabled', async () => {
    const terminal = create('live', false);
    emit({ type: 'ready' });
    data('existing screen');
    emit({ type: 'replay-complete' });
    await settle();
    data(' queued output');
    emit({ type: 'ready' });
    data(' new output');
    emit({ type: 'replay-complete' });
    await settle();
    expect(terminal.content).toBe('existing screen queued output new output');
    expect(store.prepareViewport).not.toHaveBeenCalled();
  });

  it('resets cached live output before replay and discards pending writes from the old connection', async () => {
    const terminal = create();
    emit({ type: 'ready' });
    data('history\r\n');
    emit({ type: 'replay-complete' });
    await settle();
    expect(terminal.content).toBe('history\r\n');
    data('old queued tail');
    emit({ type: 'ready' });
    data('history\r\nnew output');
    emit({ type: 'replay-complete' });
    await settle();
    expect(terminal.content).toBe('history\r\nnew output');
    terminal.input('ls\r');
    expect(send).toHaveBeenCalledWith(tab.id, { type: 'input', data: 'ls\r' });
  });

  it('initializes live remounts from server replay without merging serialized snapshots', async () => {
    store.getSerializedSnapshot.mockReturnValue({
      content: 'obsolete local screen',
      updatedAt: Date.now(),
      cols: 80,
      rows: 24,
    });
    const first = create();
    emit({ type: 'ready' });
    data('first screen');
    emit({ type: 'replay-complete' });
    await settle();
    wrapper.unmount();
    expect(first.disposed).toBe(true);
    expect(emitter.listenerCount(tab.id)).toBe(0);
    const next = create();
    emit({ type: 'ready' });
    data('first screen\r\nbackground output');
    emit({ type: 'replay-complete' });
    await settle();
    expect(next.content).toBe('first screen\r\nbackground output');
    expect(store.prepareViewport).toHaveBeenCalledTimes(2);
    expect(store.getSerializedSnapshot).not.toHaveBeenCalled();
  });

  it('restores the latest server snapshot rather than a recently serialized stale screen', async () => {
    store.getSerializedSnapshot.mockReturnValue({
      content: 'stale screen',
      updatedAt: Date.now() + 1000,
      cols: 80,
      rows: 24,
    });
    store.getLatestServerSnapshot.mockReturnValue(snapshot('latest background output'));
    const terminal = create('snapshot');
    emit({ type: 'replay-complete' });
    await settle();
    expect(terminal.content).toBe('\x1b[?1049llatest background output');
    expect([terminal.cols, terminal.rows]).toEqual([120, 32]);
    data('ignored PTY duplicate');
    await settle();
    expect(terminal.content).not.toContain('ignored');
    expect(send).toHaveBeenCalledWith(
      tab.id,
      expect.objectContaining({ type: 'snapshot-request' })
    );
  });

  it('retains a local snapshot as a fallback when no server frame is available', async () => {
    store.getLatestServerSnapshot.mockReturnValue(snapshot('saved output'));
    const first = create('snapshot');
    emit({ type: 'replay-complete' });
    await settle();
    wrapper.unmount();
    const saved = store.saveSerializedSnapshot.mock.calls.at(-1)?.[1] as TerminalSerializedSnapshot;
    expect(saved.content).toContain('saved output');
    expect(first.disposed).toBe(true);
    store.getSerializedSnapshot.mockReturnValue(saved);
    store.getLatestServerSnapshot.mockReturnValue(null);
    const next = create('snapshot');
    emit({ type: 'replay-complete' });
    await settle();
    expect(next.content).toBe(saved.content);
  });

  it('does not merge snapshot frames into a live fallback', async () => {
    store.getLatestServerSnapshot.mockReturnValue(snapshot('snapshot output'));
    const terminal = create('snapshot');
    emit({ type: 'replay-complete' });
    await settle();
    tab.connectionRenderMode = 'live';
    data('live replay');
    emit({ type: 'snapshot', snapshot: snapshot('late mirror frame') });
    await settle();
    expect(terminal.content).toBe('live replay');
  });

  it('does not replace a saved screen with an empty snapshot during rapid mount/unmount', () => {
    store.getSerializedSnapshot.mockReturnValue({
      content: 'saved screen',
      updatedAt: Date.now(),
      cols: 80,
      rows: 24,
    });
    create('snapshot');
    wrapper.unmount();
    expect(store.saveSerializedSnapshot).not.toHaveBeenCalled();
  });
});
