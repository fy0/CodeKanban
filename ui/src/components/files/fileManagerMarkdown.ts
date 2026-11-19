import { fileManagerApi } from '@/api/fileManager';
import { renderMarkdown } from '@/utils/markdown';

function resolveRelativeFilePath(sourcePath: string, url: string) {
  const [pathname = ''] = url.trim().split(/[?#]/, 1);
  if (!pathname || /^(?:[/\\]|[a-z][a-z\d+.-]*:)/i.test(pathname)) {
    return undefined;
  }

  let relativePath = pathname;
  try {
    relativePath = decodeURIComponent(relativePath);
  } catch {
    relativePath = pathname;
  }
  if (/^[/\\]/.test(relativePath)) {
    return null;
  }

  const segments = sourcePath.split('/').slice(0, -1);
  for (const segment of relativePath.split(/[/\\]/)) {
    if (!segment || segment === '.') {
      continue;
    }
    if (segment === '..') {
      if (!segments.length) {
        return null;
      }
      segments.pop();
    } else {
      segments.push(segment);
    }
  }
  return segments.join('/') || null;
}

export function renderFileManagerMarkdown(
  markdown: string,
  sourcePath: string,
  projectId: string,
  scopeId: string
) {
  const template = document.createElement('template');
  template.innerHTML = renderMarkdown(markdown);
  for (const element of template.content.querySelectorAll('a[href], img[src]')) {
    const isImage = element.localName === 'img';
    const attribute = isImage ? 'src' : 'href';
    const path = resolveRelativeFilePath(sourcePath, element.getAttribute(attribute) ?? '');
    if (path === undefined) {
      continue;
    }
    if (path === null) {
      element.removeAttribute(attribute);
      continue;
    }
    element.setAttribute(
      attribute,
      fileManagerApi.buildContentUrl(projectId, scopeId, path, isImage ? 'inline' : 'attachment')
    );
    if (!isImage) {
      element.setAttribute('data-file-preview-path', path);
    }
  }
  return template.innerHTML;
}
