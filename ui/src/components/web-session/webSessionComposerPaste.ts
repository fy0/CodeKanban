const BLOCK_TAGS = new Set([
  'address',
  'article',
  'aside',
  'blockquote',
  'div',
  'dl',
  'dt',
  'dd',
  'figcaption',
  'figure',
  'footer',
  'h1',
  'h2',
  'h3',
  'h4',
  'h5',
  'h6',
  'header',
  'hr',
  'li',
  'main',
  'nav',
  'ol',
  'p',
  'pre',
  'section',
  'table',
  'tr',
  'ul',
]);

const SKIPPED_TAGS = new Set([
  'head',
  'link',
  'meta',
  'noscript',
  'script',
  'style',
  'template',
  'title',
]);

const IMAGE_EXTENSION_BY_MIME: Record<string, string> = {
  'image/bmp': 'bmp',
  'image/gif': 'gif',
  'image/jpeg': 'jpg',
  'image/jpg': 'jpg',
  'image/png': 'png',
  'image/svg+xml': 'svg',
  'image/tiff': 'tiff',
  'image/webp': 'webp',
};

export type ComposerPasteSegment =
  | {
      type: 'text';
      value: string;
    }
  | {
      type: 'image';
      imageIndex: number;
    }
  | {
      type: 'remote-image';
      remoteImageIndex: number;
    }
  | {
      type: 'unavailable-image';
      unavailableImageIndex: number;
    };

export interface WebSessionComposerPastePlan {
  failureMarker: string;
  images: File[];
  remoteImages: string[];
  segments: ComposerPasteSegment[];
  unavailableImages: string[];
  unavailableImageCount: number;
}

export interface WebSessionComposerPasteInput {
  failureMarker: string;
  html: string;
  imageFiles?: File[];
  parseHtml?: (html: string) => Document;
  plainText: string;
}

function getTransferImageKey(file: File) {
  const normalizedName = file.name.trim().toLowerCase() || 'clipboard-image';
  const normalizedType = file.type.trim().toLowerCase();
  return [normalizedName, normalizedType, String(file.size)].join(':');
}

export function mergeClipboardImageFiles(...groups: File[][]) {
  const imageFiles: File[] = [];
  const seen = new Set<string>();

  for (const file of groups.flat()) {
    if (!file || !file.type.toLowerCase().startsWith('image/')) {
      continue;
    }
    const key = getTransferImageKey(file);
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    imageFiles.push(file);
  }

  return imageFiles;
}

export function getImageFilesFromTransfer(dataTransfer: DataTransfer | null) {
  if (!dataTransfer) {
    return [];
  }

  const imageFiles: File[] = [];
  const register = (file: File | null) => {
    if (!file || !file.type.toLowerCase().startsWith('image/')) {
      return;
    }
    imageFiles.push(file);
  };

  for (const item of Array.from(dataTransfer.items || [])) {
    if (item.kind === 'file' || item.type.toLowerCase().startsWith('image/')) {
      register(item.getAsFile());
    }
  }
  for (const file of Array.from(dataTransfer.files || [])) {
    register(file);
  }

  return mergeClipboardImageFiles(imageFiles);
}

type ClipboardItemReader = () => Promise<
  Array<{
    getType: (type: string) => Promise<Blob>;
    types: readonly string[];
  }>
>;

export async function readClipboardImageFiles(readItems?: ClipboardItemReader) {
  const read =
    readItems ??
    (typeof navigator !== 'undefined' && navigator.clipboard?.read
      ? () => navigator.clipboard.read()
      : null);
  if (!read || (typeof globalThis.isSecureContext === 'boolean' && !globalThis.isSecureContext)) {
    return [];
  }

  try {
    const imageFiles: File[] = [];
    for (const item of await read()) {
      for (const type of item.types) {
        const normalizedType = type.trim().toLowerCase();
        if (!normalizedType.startsWith('image/')) {
          continue;
        }
        const blob = await item.getType(type);
        if (!blob || blob.size <= 0) {
          continue;
        }
        const extension = IMAGE_EXTENSION_BY_MIME[normalizedType] || 'png';
        imageFiles.push(
          new File([blob], `pasted-image-${imageFiles.length + 1}.${extension}`, {
            type: normalizedType,
            lastModified: Date.now(),
          })
        );
      }
    }
    return mergeClipboardImageFiles(imageFiles);
  } catch {
    return [];
  }
}

export function exposeOfficeImageFallbacks(html: string) {
  const downlevelFallbackPattern =
    /<!--\s*\[if\s+!vml\]\s*><!-->([\s\S]*?)<!--<!\s*\[endif\]\s*-->/gi;
  const hiddenFallbackPattern = /<!--\s*\[if\s+!vml\]\s*>([\s\S]*?)<!\s*\[endif\]\s*-->/gi;
  const hasNonVmlFallback = downlevelFallbackPattern.test(html) || hiddenFallbackPattern.test(html);

  downlevelFallbackPattern.lastIndex = 0;
  hiddenFallbackPattern.lastIndex = 0;
  let prepared = html.replace(downlevelFallbackPattern, '$1').replace(hiddenFallbackPattern, '$1');

  if (!hasNonVmlFallback) {
    prepared = prepared.replace(
      /<!--\s*\[if[^\]]*\bvml\b[^\]]*\]\s*>([\s\S]*?)<!\s*\[endif\]\s*-->/gi,
      '$1'
    );
  }

  return prepared;
}

function decodeBase64(value: string) {
  const binary = globalThis.atob(value.replace(/\s+/g, ''));
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return bytes;
}

function decodePercentEncodedBytes(value: string) {
  const bytes: number[] = [];
  const encoder = new TextEncoder();

  for (let index = 0; index < value.length; ) {
    if (value[index] === '%' && /^[0-9a-f]{2}$/i.test(value.slice(index + 1, index + 3))) {
      bytes.push(Number.parseInt(value.slice(index + 1, index + 3), 16));
      index += 3;
      continue;
    }

    const codePoint = value.codePointAt(index);
    if (codePoint == null) {
      break;
    }
    const character = String.fromCodePoint(codePoint);
    bytes.push(...encoder.encode(character));
    index += character.length;
  }

  return new Uint8Array(bytes);
}

function buildDataImageFile(source: string, imageIndex: number) {
  const commaIndex = source.indexOf(',');
  if (commaIndex <= 5) {
    return null;
  }

  const metadata = source.slice(5, commaIndex);
  const mimeType = metadata.split(';', 1)[0]?.trim().toLowerCase() || '';
  if (!mimeType.startsWith('image/')) {
    return null;
  }

  try {
    const payload = source.slice(commaIndex + 1);
    const bytes = /(?:^|;)base64(?:;|$)/i.test(metadata)
      ? decodeBase64(payload)
      : decodePercentEncodedBytes(payload);
    if (bytes.length === 0) {
      return null;
    }
    const extension = IMAGE_EXTENSION_BY_MIME[mimeType] || 'png';
    return new File([bytes], `pasted-image-${imageIndex + 1}.${extension}`, {
      type: mimeType,
      lastModified: Date.now(),
    });
  } catch {
    return null;
  }
}

function getNodeTagName(node: Node) {
  const element = node as Element;
  return String(element.localName || element.nodeName || '')
    .trim()
    .toLowerCase();
}

function isImageNode(node: Node) {
  const tagName = getNodeTagName(node);
  return tagName === 'img' || tagName === 'imagedata' || tagName === 'v:imagedata';
}

function getImageSource(node: Node) {
  const element = node as Element;
  return ['src', 'data-src', 'data-original']
    .map(attribute => String(element.getAttribute?.(attribute) || '').trim())
    .find(Boolean);
}

function appendText(segments: ComposerPasteSegment[], value: string) {
  if (!value) {
    return;
  }
  const previous = segments[segments.length - 1];
  if (previous?.type === 'text') {
    previous.value += value;
    return;
  }
  segments.push({ type: 'text', value });
}

function appendImage(segments: ComposerPasteSegment[], images: File[], file: File) {
  const imageIndex = images.push(file) - 1;
  segments.push({ type: 'image', imageIndex });
}

function appendRemoteImage(
  segments: ComposerPasteSegment[],
  remoteImages: string[],
  source: string
) {
  const remoteImageIndex = remoteImages.push(source) - 1;
  segments.push({ type: 'remote-image', remoteImageIndex });
}

function appendUnavailableImage(
  segments: ComposerPasteSegment[],
  unavailableImages: string[],
  source: string
) {
  const unavailableImageIndex = unavailableImages.push(source) - 1;
  segments.push({ type: 'unavailable-image', unavailableImageIndex });
}

function appendResolvedImage(
  node: Node,
  segments: ComposerPasteSegment[],
  images: File[],
  remoteImages: string[],
  unavailableImages: string[],
  pendingFiles: File[]
) {
  const clipboardFile = pendingFiles.shift();
  if (clipboardFile) {
    appendImage(segments, images, clipboardFile);
    return;
  }

  const source = getImageSource(node) || '';
  if (source.toLowerCase().startsWith('data:')) {
    const file = buildDataImageFile(source, images.length);
    if (file) {
      appendImage(segments, images, file);
      return;
    }
  } else {
    try {
      const url = new URL(source);
      if (url.protocol === 'http:' || url.protocol === 'https:') {
        appendRemoteImage(segments, remoteImages, url.toString());
        return;
      }
    } catch {
      // Word file URLs and relative references are intentionally not resolved.
    }
  }

  appendUnavailableImage(segments, unavailableImages, source);
}

function walkHtmlNode(
  node: Node,
  segments: ComposerPasteSegment[],
  images: File[],
  remoteImages: string[],
  unavailableImages: string[],
  pendingFiles: File[]
) {
  if (node.nodeType === 3) {
    appendText(segments, node.textContent || '');
    return;
  }
  if (node.nodeType !== 1) {
    return;
  }

  const tagName = getNodeTagName(node);
  if (SKIPPED_TAGS.has(tagName)) {
    return;
  }
  if (isImageNode(node)) {
    appendResolvedImage(node, segments, images, remoteImages, unavailableImages, pendingFiles);
    return;
  }
  if (tagName === 'br') {
    appendText(segments, '\n');
    return;
  }

  const isBlock = BLOCK_TAGS.has(tagName);
  if (isBlock) {
    appendText(segments, '\n');
  }
  for (const child of Array.from(node.childNodes)) {
    walkHtmlNode(child, segments, images, remoteImages, unavailableImages, pendingFiles);
  }
  if (tagName === 'td' || tagName === 'th') {
    appendText(segments, '\t');
  }
  if (isBlock) {
    appendText(segments, '\n');
  }
}

function normalizeRenderedPaste(value: string) {
  return value
    .replace(/\u00a0/g, ' ')
    .replace(/\r\n?/g, '\n')
    .replace(/[\t\f\v ]+/g, ' ')
    .replace(/ *\n */g, '\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim();
}

export function buildWebSessionComposerPastePlan(
  input: WebSessionComposerPasteInput
): WebSessionComposerPastePlan | null {
  const failureMarker = String(input.failureMarker || '').trim() || '[Image upload failed]';
  const pendingFiles = Array.from(input.imageFiles || []).filter(file =>
    file.type.toLowerCase().startsWith('image/')
  );
  const segments: ComposerPasteSegment[] = [];
  const images: File[] = [];
  const remoteImages: string[] = [];
  const unavailableImages: string[] = [];
  let foundHtmlImage = false;

  const html = String(input.html || '').trim();
  if (html && (input.parseHtml || typeof DOMParser !== 'undefined')) {
    try {
      const preparedHtml = exposeOfficeImageFallbacks(html);
      const document = input.parseHtml
        ? input.parseHtml(preparedHtml)
        : new DOMParser().parseFromString(preparedHtml, 'text/html');
      foundHtmlImage = Array.from(document.body.querySelectorAll('*')).some(isImageNode);
      if (foundHtmlImage) {
        walkHtmlNode(
          document.body,
          segments,
          images,
          remoteImages,
          unavailableImages,
          pendingFiles
        );
      }
    } catch {
      foundHtmlImage = false;
      segments.length = 0;
      images.length = 0;
      remoteImages.length = 0;
      unavailableImages.length = 0;
    }
  }

  if (!foundHtmlImage && pendingFiles.length === 0) {
    return null;
  }

  if (!foundHtmlImage) {
    appendText(segments, String(input.plainText || ''));
  }

  if (pendingFiles.length > 0) {
    appendText(segments, segments.length > 0 ? '\n' : '');
    pendingFiles.forEach((file, index) => {
      appendImage(segments, images, file);
      if (index < pendingFiles.length - 1) {
        appendText(segments, ' ');
      }
    });
  }

  return {
    failureMarker,
    images,
    remoteImages,
    segments,
    unavailableImages,
    unavailableImageCount: unavailableImages.length,
  };
}

export function renderWebSessionComposerPastePlan(
  plan: WebSessionComposerPastePlan,
  imageReplacements: string[] = [],
  remoteImageReplacements: string[] = [],
  unavailableImageReplacements: string[] = []
) {
  const rendered = plan.segments
    .map(segment => {
      if (segment.type === 'text') {
        return segment.value;
      }
      const replacement =
        segment.type === 'image'
          ? imageReplacements[segment.imageIndex] || plan.failureMarker
          : segment.type === 'remote-image'
            ? remoteImageReplacements[segment.remoteImageIndex] ||
              plan.remoteImages[segment.remoteImageIndex] ||
              plan.failureMarker
            : unavailableImageReplacements[segment.unavailableImageIndex] || plan.failureMarker;
      return ` ${replacement} `;
    })
    .join('');

  return normalizeRenderedPaste(rendered);
}
