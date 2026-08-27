import type { LocaleType } from '@/composables/useLocale';

export interface DailyTipDefinition {
  id: string;
  title: string;
  description: string;
}

export interface DailyTipState {
  lastShownDate: string | null;
}

export interface DailyTipVisibilityContext {
  routeName: unknown;
  projectId: string | null | undefined;
  enabled: boolean;
  lastShownDate: string | null;
  todayDateKey: string;
  tipCount: number;
}

type StorageReader = Pick<Storage, 'getItem'>;
type StorageWriter = Pick<Storage, 'setItem'>;

export const DAILY_TIP_STATE_STORAGE_KEY = 'daily_tip_state';

export const DEFAULT_DAILY_TIP_STATE: DailyTipState = {
  lastShownDate: null,
};

const DAILY_TIPS_ZH_CN: readonly DailyTipDefinition[] = [
  {
    id: 'shortcuts',
    title: '善用快捷键切换工作区',
    description:
      '会在最近访问的两个工作区标签之间切换；如果你只访问过一个标签，则默认在终端和会话之间切换。',
  },
  {
    id: 'editor',
    title: '提前配置默认编辑器',
    description:
      '在设置里选好你常用的编辑器后，“在编辑器中打开”会直接复用这套配置，跨工作区跳转会更顺手。',
  },
  {
    id: 'tab-badge',
    title: '看懂页面标签上的三个数字',
    description: '页面标签上的 3 个数字依次表示：运行中 / 待批准 / 未读。',
  },
] as const;

const DAILY_TIPS_EN_US: readonly DailyTipDefinition[] = [
  {
    id: 'shortcuts',
    title: 'Use shortcuts to switch context faster',
    description:
      'Switches between the two most recently visited workspace tabs. If you have only visited one tab, it falls back to switching between Terminal and AI Sessions.',
  },
  {
    id: 'editor',
    title: 'Set your default editor in advance',
    description:
      'Once your preferred editor is configured in Settings, “Open in Editor” becomes a reliable one-click jump between workspaces.',
  },
  {
    id: 'tab-badge',
    title: 'Read the three numbers on page tabs',
    description: 'The three numbers on a page tab mean: running / pending approval / unread.',
  },
] as const;

const DAILY_TIPS_BY_LOCALE: Record<LocaleType, readonly DailyTipDefinition[]> = {
  'zh-CN': DAILY_TIPS_ZH_CN,
  'en-US': DAILY_TIPS_EN_US,
};

export function getDailyTips(locale: string): readonly DailyTipDefinition[] {
  if (locale === 'en-US') {
    return DAILY_TIPS_BY_LOCALE['en-US'];
  }
  return DAILY_TIPS_BY_LOCALE['zh-CN'];
}

export function formatLocalDateKey(date = new Date()): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export function selectDailyTipIndex(dateKey: string, tipCount: number): number {
  if (!Number.isInteger(tipCount) || tipCount <= 0) {
    return -1;
  }
  const epochDay = resolveEpochDay(dateKey);
  return ((epochDay % tipCount) + tipCount) % tipCount;
}

export function selectRandomDailyTipIndex(randomValue: number, tipCount: number): number {
  if (!Number.isInteger(tipCount) || tipCount <= 0) {
    return -1;
  }
  const normalized = Number.isFinite(randomValue) ? randomValue : 0;
  const clamped = Math.min(Math.max(normalized, 0), 0.999999999999);
  return Math.floor(clamped * tipCount);
}

export function selectAnotherRandomDailyTipIndex(
  currentIndex: number,
  randomValue: number,
  tipCount: number
): number {
  if (!Number.isInteger(tipCount) || tipCount <= 0) {
    return -1;
  }
  if (tipCount === 1) {
    return 0;
  }

  const nextIndex = selectRandomDailyTipIndex(randomValue, tipCount);
  if (nextIndex < 0) {
    return -1;
  }
  if (nextIndex !== currentIndex) {
    return nextIndex;
  }
  return (nextIndex + 1) % tipCount;
}

export function sanitizeDailyTipState(value: unknown): DailyTipState {
  if (!value || typeof value !== 'object') {
    return { ...DEFAULT_DAILY_TIP_STATE };
  }
  const source = value as Partial<DailyTipState>;
  return {
    lastShownDate:
      typeof source.lastShownDate === 'string' && source.lastShownDate.trim().length > 0
        ? source.lastShownDate
        : null,
  };
}

export function loadDailyTipState(
  storage: StorageReader | null = resolveLocalStorage()
): DailyTipState {
  if (!storage) {
    return { ...DEFAULT_DAILY_TIP_STATE };
  }
  try {
    const raw = storage.getItem(DAILY_TIP_STATE_STORAGE_KEY);
    if (!raw) {
      return { ...DEFAULT_DAILY_TIP_STATE };
    }
    return sanitizeDailyTipState(JSON.parse(raw));
  } catch (error) {
    console.warn('Failed to load daily tip state.', error);
    return { ...DEFAULT_DAILY_TIP_STATE };
  }
}

export function saveDailyTipState(
  state: DailyTipState,
  storage: StorageWriter | null = resolveLocalStorage()
): DailyTipState {
  const nextState = sanitizeDailyTipState(state);
  if (!storage) {
    return nextState;
  }
  try {
    storage.setItem(DAILY_TIP_STATE_STORAGE_KEY, JSON.stringify(nextState));
  } catch (error) {
    console.warn('Failed to save daily tip state.', error);
  }
  return nextState;
}

export function shouldShowDailyTip(context: DailyTipVisibilityContext): boolean {
  if (context.routeName !== 'project') {
    return false;
  }
  if (!context.enabled) {
    return false;
  }
  if (typeof context.projectId !== 'string' || context.projectId.trim().length === 0) {
    return false;
  }
  if (!Number.isInteger(context.tipCount) || context.tipCount <= 0) {
    return false;
  }
  return context.lastShownDate !== context.todayDateKey;
}

function resolveEpochDay(dateKey: string): number {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(dateKey);
  if (match) {
    const year = Number(match[1]);
    const month = Number(match[2]);
    const day = Number(match[3]);
    if (
      Number.isInteger(year) &&
      Number.isInteger(month) &&
      Number.isInteger(day) &&
      month >= 1 &&
      month <= 12 &&
      day >= 1 &&
      day <= 31
    ) {
      return Math.floor(Date.UTC(year, month - 1, day) / 86400000);
    }
  }

  let hash = 0;
  for (let index = 0; index < dateKey.length; index += 1) {
    hash = (hash * 31 + dateKey.charCodeAt(index)) | 0;
  }
  return Math.abs(hash);
}

function resolveLocalStorage(): Storage | null {
  if (typeof window === 'undefined' || !window.localStorage) {
    return null;
  }
  return window.localStorage;
}
