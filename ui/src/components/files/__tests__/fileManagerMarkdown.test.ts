// @vitest-environment happy-dom

import { mount } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';
import { renderFileManagerMarkdown } from '@/components/files/fileManagerMarkdown';
import FilePreviewSurface from '@/components/files/FilePreviewSurface.vue';

vi.mock('@/api', () => ({
  ApiError: class ApiError extends Error {},
  urlBase: '',
}));

vi.mock('@/api/http', () => ({ http: {} }));

const sourcePath = 'docs/guides/a.md';

function renderPreview(markdown: string, path = sourcePath) {
  return renderFileManagerMarkdown(markdown, path, 'project-1', 'worktree:feature');
}

function parsePreview(markdown: string, path = sourcePath) {
  const container = document.createElement('div');
  container.innerHTML = renderPreview(markdown, path);
  return container;
}

describe('file manager Markdown preview', () => {
  it.each([
    ['b.md', 'docs/guides/b.md'],
    ['./b.md', 'docs/guides/b.md'],
    ['../../b.md', 'b.md'],
    ['../notes/%E5%9B%BE%20%23%25.png', 'docs/notes/图 #%.png'],
    ['b.md?raw=1#details', 'docs/guides/b.md'],
  ])('resolves link %s to %s in the same scope', (href, expectedPath) => {
    const link = parsePreview(`[file](${href})`).querySelector('a')!;
    const url = new URL(link.getAttribute('href')!, 'http://localhost');
    expect(link.dataset.filePreviewPath).toBe(expectedPath);
    expect(url.pathname).toBe('/api/v1/projects/project-1/files/content');
    expect(url.searchParams.get('scopeId')).toBe('worktree:feature');
    expect(url.searchParams.get('path')).toBe(expectedPath);
    expect(url.searchParams.get('disposition')).toBe('attachment');
  });

  it('loads an embedded image from the directory of each Markdown file', () => {
    const markdown = '![preview](./images/example.png)';
    for (const directory of ['docs', 'examples']) {
      const image = parsePreview(markdown, `${directory}/a.md`).querySelector('img')!;
      const url = new URL(image.getAttribute('src')!, 'http://localhost');
      expect(url.searchParams.get('path')).toBe(`${directory}/images/example.png`);
      expect(url.searchParams.get('scopeId')).toBe('worktree:feature');
      expect(url.searchParams.get('disposition')).toBe('inline');
      expect(image.alt).toBe('preview');
    }
  });

  it.each(['https://example.com/b.md', '//example.com/b.md', '/api/file', '#details'])(
    'leaves browser links unchanged: %s',
    href => {
      const link = parsePreview(`[link](${href})`).querySelector('a')!;
      expect(link.getAttribute('href')).toBe(href);
      expect(link.dataset.filePreviewPath).toBeUndefined();
    }
  );

  it('does not navigate to relative references outside the scope', () => {
    const preview = parsePreview('[file](../../../b.md)\n\n![image](%2e%2e/image.png)', 'a.md');
    expect(preview.querySelector('a')?.hasAttribute('href')).toBe(false);
    expect(preview.querySelector('img')?.hasAttribute('src')).toBe(false);
  });

  it('opens a clicked file link in the preview instead of navigating the browser', () => {
    const wrapper = mount(FilePreviewSurface, {
      props: {
        previewResult: {
          entry: {
            name: 'a.md',
            path: sourcePath,
            previewKind: 'markdown',
            kind: 'file',
            size: 0,
            modifiedAt: '',
            hidden: false,
          },
          previewKind: 'markdown',
          truncated: false,
          inlineUrl: '',
          downloadUrl: '',
        },
        previewMode: 'file',
        renderedMarkdown: renderPreview('[**image**](../images/example.png)'),
        previewLoading: false,
        previewError: '',
        previewFallbackText: '',
        previewMeta: '',
        emptyLabel: '',
        binaryPreviewHint: '',
        previewTruncatedLabel: '',
        downloadLabel: '',
        showDiffToggle: false,
        fileLabel: '',
        diffLabel: '',
        diffResult: null,
        diffLoading: false,
        diffError: '',
        renderedDiff: '',
        diffUnavailableText: '',
      },
      global: { stubs: { NButton: true } },
    });

    const event = new MouseEvent('click', { bubbles: true, cancelable: true });
    wrapper.get('a strong').element.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(true);
    expect(wrapper.emitted('open-linked-file')).toEqual([['docs/images/example.png']]);
    wrapper.unmount();
  });
});
