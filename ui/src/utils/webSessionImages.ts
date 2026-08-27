import { urlBase } from '@/api';

export function buildImagePlaceholder(index: number) {
  return `[Image #${index}]`;
}

export function buildImagePlaceholderLine(count: number) {
  if (count <= 0) {
    return '';
  }

  return Array.from({ length: count }, (_, index) => buildImagePlaceholder(index + 1)).join(' ');
}

const IMAGE_PLACEHOLDER_PATTERN = /\[Image #\d+\]/gi;

const IMAGE_EXTENSION_BY_MIME: Record<string, string> = {
  'image/png': '.png',
  'image/jpeg': '.jpg',
  'image/jpg': '.jpg',
  'image/gif': '.gif',
  'image/webp': '.webp',
  'image/bmp': '.bmp',
  'image/svg+xml': '.svg',
  'image/tiff': '.tiff',
};

export function insertImagePlaceholdersAtCursor(
  text: string,
  selectionStart: number,
  selectionEnd: number,
  placeholders: string[]
) {
  const insertion = placeholders.join(' ');
  if (!insertion) {
    return {
      text,
      cursor: selectionStart,
    };
  }

  const normalizedText = String(text ?? '');
  const safeStart = Math.max(0, Math.min(selectionStart, normalizedText.length));
  const safeEnd = Math.max(safeStart, Math.min(selectionEnd, normalizedText.length));
  const prefix = normalizedText.slice(0, safeStart);
  const suffix = normalizedText.slice(safeEnd);
  const needsLeadingSpace =
    prefix.length > 0 && !/[\s([{'"`]/.test(prefix[prefix.length - 1] || '');
  const needsTrailingSpace = suffix.length > 0 && !/[\s)\]}.,!?;:'"`]/.test(suffix[0] || '');
  const insertedText = `${needsLeadingSpace ? ' ' : ''}${insertion}${needsTrailingSpace ? ' ' : ''}`;

  return {
    text: `${prefix}${insertedText}${suffix}`,
    cursor: prefix.length + insertedText.length,
  };
}

export function isGenericImageAttachmentName(name: string) {
  const normalized = String(name || '')
    .trim()
    .split(/[\\/]/)
    .pop()
    ?.toLowerCase();
  if (!normalized) {
    return true;
  }

  const extensionIndex = normalized.lastIndexOf('.');
  const stem = extensionIndex > 0 ? normalized.slice(0, extensionIndex) : normalized;
  if (stem === 'image' || stem === 'blob' || stem === 'clipboard-image') {
    return true;
  }
  return stem.startsWith('pasted-image-');
}

export function buildUploadImageFileName(name: string, index: number, mimeType?: string) {
  if (!isGenericImageAttachmentName(name)) {
    return String(name || '').trim();
  }

  const normalizedMime = String(mimeType || '')
    .trim()
    .toLowerCase();
  const extension = IMAGE_EXTENSION_BY_MIME[normalizedMime] || '.png';
  return `image ${index}${extension}`;
}

export function resolveImageAttachmentDisplayName(name: string, index: number) {
  const trimmed = String(name || '')
    .trim()
    .split(/[\\/]/)
    .pop();
  if (trimmed && /^image \d+\.[a-z0-9]+$/i.test(trimmed)) {
    return trimmed.replace(/\.[a-z0-9]+$/i, '');
  }
  if (!trimmed || isGenericImageAttachmentName(trimmed)) {
    return `image ${index}`;
  }
  return trimmed;
}

export function stripImagePlaceholdersFromText(text: string, attachmentCount: number) {
  const normalized = String(text ?? '');
  if (!normalized || attachmentCount <= 0 || !/\[Image #\d+\]/i.test(normalized)) {
    return normalized;
  }

  return normalized
    .replace(IMAGE_PLACEHOLDER_PATTERN, ' ')
    .replace(/[ \t]+/g, ' ')
    .replace(/ *\n */g, '\n')
    .replace(/\n{2,}/g, '\n')
    .trim();
}

export type ImageViewToolOutput = {
  id?: string;
  path: string;
  type: 'imageView';
  cwd?: string;
};

function asRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

export function parseImageViewToolOutput(value: unknown): ImageViewToolOutput | null {
  let parsed = value;

  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (!trimmed.startsWith('{')) {
      return null;
    }

    try {
      parsed = JSON.parse(trimmed);
    } catch {
      return null;
    }
  }

  const record = asRecord(parsed);
  if (!record) {
    return null;
  }

  if (String(record.type ?? '').trim() !== 'imageView') {
    return null;
  }

  const path = String(record.path ?? '').trim();
  if (!path) {
    return null;
  }

  const id = String(record.id ?? '').trim();
  const cwd = String(record.cwd ?? record.workdir ?? '').trim();

  return {
    id: id || undefined,
    path,
    type: 'imageView',
    cwd: cwd || undefined,
  };
}

export function resolveImageViewDisplayName(path: string) {
  const trimmed = String(path || '')
    .trim()
    .split(/[\\/]/)
    .pop();
  return trimmed || String(path || '').trim();
}

export function buildImageViewPreviewUrl(
  path: string,
  options?: {
    cwd?: string;
    baseUrl?: string;
  }
) {
  const normalizedPath = String(path || '').trim();
  if (!normalizedPath) {
    return '';
  }

  const params = new URLSearchParams();
  params.set('path', normalizedPath);

  const cwd = String(options?.cwd || '').trim();
  if (cwd) {
    params.set('cwd', cwd);
  }

  const requestPath = `/api/v1/web-sessions/image-view?${params.toString()}`;
  const baseUrl = typeof options?.baseUrl === 'string' ? options.baseUrl : urlBase;
  return baseUrl ? new URL(requestPath, baseUrl).toString() : requestPath;
}
