import { urlBase } from '@/api';
import { extractItem } from '@/api/response';

export type TerminalImageUploadSource = 'paste' | 'drop';

type UploadImageResponse = {
  path: string;
  fileName: string;
  size: number;
};

type UploadProgress = {
  loaded: number;
  total?: number;
  percent: number | null;
};

const STREAM_UPLOAD_PATH = '/api/v1/upload/clipboard-image-stream';

const MIME_EXTENSION_MAP: Record<string, string> = {
  'image/png': 'png',
  'image/jpeg': 'jpg',
  'image/jpg': 'jpg',
  'image/gif': 'gif',
  'image/webp': 'webp',
  'image/bmp': 'bmp',
  'image/svg+xml': 'svg',
  'image/tiff': 'tiff',
};

function padTimestamp(value: number) {
  return String(value).padStart(2, '0');
}

function resolveImageExtension(contentType?: string) {
  const normalized = contentType?.trim().toLowerCase();
  if (!normalized) {
    return 'png';
  }
  return MIME_EXTENSION_MAP[normalized] || 'png';
}

export function buildPastedImageFileName(contentType?: string, now = new Date()) {
  const extension = resolveImageExtension(contentType);
  const timestamp = [
    now.getFullYear(),
    padTimestamp(now.getMonth() + 1),
    padTimestamp(now.getDate()),
    '-',
    padTimestamp(now.getHours()),
    padTimestamp(now.getMinutes()),
    padTimestamp(now.getSeconds()),
  ].join('');

  return `pasted-image-${timestamp}.${extension}`;
}

export function formatTerminalPathInput(path: string) {
  if (!path) {
    return '';
  }

  const normalizedPath = /\s/.test(path) ? `"${path.replace(/"/g, '\\"')}"` : path;
  return `${normalizedPath} `;
}

export async function uploadTerminalImage(options: {
  blob: Blob | File;
  fileName?: string;
  source: TerminalImageUploadSource;
  onProgress?: (progress: UploadProgress) => void;
}) {
  const { blob, fileName, source, onProgress } = options;
  const resolvedFileName = fileName?.trim() || buildPastedImageFileName(blob.type);
  const formData = new FormData();
  formData.append('file', blob, resolvedFileName);
  formData.append('fileName', resolvedFileName);
  formData.append('source', source);

  return new Promise<UploadImageResponse>((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    const uploadUrl = urlBase
      ? new URL(STREAM_UPLOAD_PATH, urlBase).toString()
      : STREAM_UPLOAD_PATH;

    xhr.open('POST', uploadUrl, true);
    xhr.withCredentials = true;
    xhr.responseType = 'json';

    xhr.upload.onprogress = event => {
      if (!onProgress) {
        return;
      }

      const percent =
        event.lengthComputable && event.total > 0
          ? Math.max(0, Math.min(100, Math.round((event.loaded / event.total) * 100)))
          : null;

      onProgress({
        loaded: event.loaded,
        total: event.lengthComputable ? event.total : undefined,
        percent,
      });
    };

    xhr.onerror = () => {
      reject(new Error('network error while uploading image'));
    };

    xhr.onload = () => {
      const payload =
        xhr.response && typeof xhr.response === 'object'
          ? xhr.response
          : xhr.responseText
            ? JSON.parse(xhr.responseText)
            : undefined;

      if (xhr.status < 200 || xhr.status >= 300) {
        const detail =
          typeof payload === 'object' && payload !== null && 'detail' in payload
            ? String((payload as { detail?: string }).detail || '')
            : '';
        reject(new Error(detail || `upload failed with status ${xhr.status}`));
        return;
      }

      const item = extractItem<UploadImageResponse>(payload);
      if (!item?.path) {
        reject(new Error('upload succeeded but no file path was returned'));
        return;
      }

      resolve(item);
    };

    xhr.send(formData);
  });
}
