import dayjs from 'dayjs';
import type { LocaleType } from '@/composables/useLocale';

type WebSessionLocale = LocaleType | string;
type FormatterKind = 'time' | 'compactDateTime' | 'fullDateTime';

const formatterCache = new Map<string, Intl.DateTimeFormat>();

const FORMATTER_OPTIONS: Record<FormatterKind, Intl.DateTimeFormatOptions> = {
  time: {
    timeStyle: 'medium',
  },
  compactDateTime: {
    dateStyle: 'short',
    timeStyle: 'medium',
  },
  fullDateTime: {
    dateStyle: 'medium',
    timeStyle: 'medium',
  },
};

function isValidTimestamp(timestamp: number) {
  return Number.isFinite(timestamp) && timestamp > 0;
}

function isSameLocalCalendarDay(left: Date, right: Date) {
  return (
    left.getFullYear() === right.getFullYear() &&
    left.getMonth() === right.getMonth() &&
    left.getDate() === right.getDate()
  );
}

function getFormatter(locale: WebSessionLocale, kind: FormatterKind) {
  const normalizedLocale = String(locale || '');
  const cacheKey = `${normalizedLocale}:${kind}`;
  const cached = formatterCache.get(cacheKey);
  if (cached) {
    return cached;
  }

  const options = FORMATTER_OPTIONS[kind];

  let formatter: Intl.DateTimeFormat;
  try {
    formatter = new Intl.DateTimeFormat(normalizedLocale || undefined, options);
  } catch {
    formatter = new Intl.DateTimeFormat(undefined, options);
  }

  formatterCache.set(cacheKey, formatter);
  return formatter;
}

export function formatWebSessionTimestamp(
  timestamp: number,
  locale: WebSessionLocale,
  now: Date = new Date()
) {
  if (!isValidTimestamp(timestamp)) {
    return '';
  }

  const date = new Date(timestamp);
  const formatter = isSameLocalCalendarDay(date, now)
    ? getFormatter(locale, 'time')
    : getFormatter(locale, 'compactDateTime');

  return formatter.format(date);
}

export function formatWebSessionDateTime(timestamp: number, locale: WebSessionLocale) {
  if (!isValidTimestamp(timestamp)) {
    return '';
  }

  return getFormatter(locale, 'fullDateTime').format(new Date(timestamp));
}

export function formatWebSessionSidebarTime(
  timestamp: number,
  now: Date = new Date(),
  showYesterdayAsTime = true
) {
  if (!isValidTimestamp(timestamp)) {
    return '';
  }

  const date = dayjs(timestamp);
  const reference = dayjs(now);
  if (
    date.isSame(reference, 'day') ||
    (showYesterdayAsTime && date.isSame(reference.subtract(1, 'day'), 'day'))
  ) {
    return date.format('HH:mm');
  }
  if (date.isSame(reference, 'year')) {
    return date.format('M/D');
  }
  return date.format('YY/M/D');
}
