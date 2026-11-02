import { beforeEach, describe, expect, it } from 'vitest';

import {
  renderHighlightedCodeBlock,
  renderHighlightedPlainText,
  renderMarkdown,
  renderStreamingMarkdownBlocks,
  resetStreamingMarkdownBlocks,
} from '@/utils/markdown';

describe('renderMarkdown', () => {
  it.each(['```text\n```\n', '```\n', '~~~\n~~~\n'])(
    'omits truly empty code cards and their copy buttons: %j',
    source => {
      const html = renderMarkdown(source, { enableCodeBlockCopy: true });
      expect(html).not.toContain('<pre');
      expect(html).not.toContain('data-message-code-copy');
    }
  );

  it.each([' ', '\t', '\n\n'])('preserves whitespace-only code content: %j', content => {
    expect(renderMarkdown('```text\n' + content + '\n```\n')).toContain('<pre');
  });

  it('separates repaired chat rendering from standard rendering and raw copy text', () => {
    const body = ['Intro', '```text', 'Read source', '```', 'After workflow'];
    const source = ['```text', ...body, '```'].join('\n');
    const standard = renderMarkdown(source);
    const repaired = renderMarkdown(source, { repairMalformedOuterFence: true });

    expect(repaired).toBe(renderMarkdown(['````text', ...body, '````'].join('\n')));
    expect(repaired).toContain('Read source\n```\nAfter workflow');
    expect(repaired).not.toContain('<p>After workflow');
    expect(standard).toContain('<p>After workflow');
    expect(renderMarkdown(source)).toBe(standard);
    expect(renderMarkdown(source, { repairMalformedOuterFence: false })).toBe(standard);
    expect(renderHighlightedPlainText(source)).toBe(source);
  });

  it('highlights fenced code blocks by default', () => {
    const html = renderMarkdown('```go\nfmt.Println("hi")\n```');

    expect(html).toContain('class="hljs language-go"');
    expect(html).toContain('data-language="go"');
  });

  it('highlights haxe fenced code blocks', () => {
    const html = renderMarkdown('```haxe\nclass Main {}\n```');

    expect(html).toContain('class="hljs language-haxe"');
    expect(html).toContain('data-language="haxe"');
  });

  it('supports the hx alias for haxe fenced code blocks', () => {
    const html = renderMarkdown('```hx\nclass Main {}\n```');

    expect(html).toContain('class="hljs language-haxe"');
    expect(html).toContain('data-language="hx"');
  });

  it('can skip code highlighting for streaming renders', () => {
    const html = renderMarkdown('```html\n<div class="box">\n```', {
      disableCodeHighlight: true,
    });

    expect(html).toContain('class="hljs"');
    expect(html).toContain('data-language="html"');
    expect(html).not.toContain('language-xml');
    expect(html).not.toContain('<span class="hljs-');
    expect(html).toContain('&lt;div class=&quot;box&quot;&gt;');
  });

  it('adds a code-block copy button only when enabled', () => {
    const enabledHtml = renderMarkdown('```bash\necho "hi"\n```', {
      enableCodeBlockCopy: true,
      codeBlockCopyLabel: 'copy',
    });
    const disabledHtml = renderMarkdown('```bash\necho "hi"\n```');

    expect(enabledHtml).toContain('data-message-code-copy="true"');
    expect(enabledHtml).toContain('class="markdown-code-copy-button"');
    expect(enabledHtml).toContain('>copy</button>');
    expect(enabledHtml).toContain('data-code-copy="true"');
    expect(disabledHtml).not.toContain('data-message-code-copy="true"');
  });

  it('keeps link rendering intact when highlighting is disabled', () => {
    const html = renderMarkdown('[docs](https://example.com)', {
      disableCodeHighlight: true,
    });

    expect(html).toContain('href="https://example.com"');
    expect(html).toContain('data-message-link="true"');
    expect(html).toContain('target="_blank"');
  });

  it.each([
    [
      'D:/codes/2025/zayud/screenshots/progress-relogin-two.png',
      'D:/codes/2025/zayud/screenshots/progress-relogin-two.png',
    ],
    [
      '/D:/codes/2025/zayud/screenshots/progress-relogin-two.png',
      'D:/codes/2025/zayud/screenshots/progress-relogin-two.png',
    ],
    [
      String.raw`D:\codes\2025\zayud\screenshots\progress-relogin-two.png`,
      'D:/codes/2025/zayud/screenshots/progress-relogin-two.png',
    ],
    ['/d:/screenshots/整改%20台账%231.png', 'd:/screenshots/整改 台账#1.png'],
    ['file:///D:/screenshots/整改%20台账%231.png', 'D:/screenshots/整改 台账#1.png'],
    ['file://localhost/D:/screenshots/preview.png', 'D:/screenshots/preview.png'],
  ])('loads a local Markdown image through the preview endpoint: %s', (source, path) => {
    const markdown = `![重新登录后的整改台账](${source} "整改截图")`;
    const html = renderMarkdown(markdown);
    const src = /<img src="([^"]+)"/.exec(html)?.[1];
    expect(src).toBeDefined();
    const url = new URL(src!, 'http://localhost');

    expect(url.pathname).toBe('/api/v1/web-sessions/image-view');
    expect(url.searchParams.get('path')).toBe(path);
    expect(html).toContain('alt="重新登录后的整改台账"');
    expect(html).toContain('title="整改截图"');
    expect(
      renderStreamingMarkdownBlocks(`image:${source}`, markdown)
        .map(block => block.html)
        .join('')
    ).toBe(html);
  });

  it.each([
    'https://example.com/preview.png',
    '//example.com/preview.png',
    '/images/preview.png',
    '/api/v1/web-sessions/attachments/preview.png',
    './screenshots/preview.png',
    'data:image/png;base64,aGVsbG8=',
  ])('preserves web and relative image sources: %s', source => {
    expect(renderMarkdown(`![preview](${source})`)).toContain(`src="${source}"`);
  });

  it('keeps image syntax inside code as literal text', () => {
    const image = '![preview](/D:/screenshots/preview.png)';

    expect(renderMarkdown('`' + image + '`')).not.toContain('<img');
    expect(renderMarkdown('```markdown\n' + image + '\n```')).not.toContain('<img');
  });

  it('adds a copy button only for absolute http and https links when enabled', () => {
    const html = renderMarkdown(
      [
        '[http](http://10.128.128.111:6032/)',
        '[https](https://example.com/docs)',
        '[relative](/docs/getting-started)',
      ].join('\n\n'),
      {
        enableLinkCopy: true,
        linkCopyLabel: 'Copy link',
      }
    );

    expect(html).toContain('data-message-link-copy="true"');
    expect(html).toContain('data-message-link-copy-href="http://10.128.128.111:6032/"');
    expect(html).toContain('data-message-link-copy-href="https://example.com/docs"');
    expect(html).not.toContain('data-message-link-copy-href="/docs/getting-started"');
  });

  it('does not add copy buttons when link copy is disabled', () => {
    const html = renderMarkdown('[http](http://10.128.128.111:6032/)');

    expect(html).not.toContain('data-message-link-copy="true"');
  });

  it('highlights search text without modifying markdown tags or attributes', () => {
    const html = renderMarkdown('**切换** and [切换](https://example.com)', {
      textHighlightQuery: '切换',
    });

    expect(html).toContain('<strong><mark class="markdown-search-highlight">切换</mark></strong>');
    expect(html).toContain(
      '<a href="https://example.com" target="_blank" rel="noopener noreferrer" data-message-link="true"><mark class="markdown-search-highlight">切换</mark></a>'
    );
    expect(html).not.toContain('href="<mark');
  });

  it('highlights escaped plain text safely', () => {
    const html = renderHighlightedPlainText('<切换> & ready', '<切换>');

    expect(html).toContain('<mark class="markdown-search-highlight">&lt;切换&gt;</mark>');
    expect(html).toContain('&amp; ready');
  });

  it('preserves ordered, unordered, and nested list structure', () => {
    const html = renderMarkdown(
      [
        '1. first',
        '2. second',
        '   - child',
        '   - child two',
        '3. third',
        '   1. nested one',
        '   2. nested two',
      ].join('\n')
    );

    expect(html).toContain('<ol>');
    expect(html).toContain('<ul>');
    expect(html).toContain('<li>first</li>');
    expect(html).toContain('<li>child</li>');
    expect(html).toContain('<li>nested one</li>');
  });

  it('renders standalone highlighted diff blocks', () => {
    const html = renderHighlightedCodeBlock('@@ -1 +1 @@\n-old\n+new\n', 'diff');

    expect(html).toContain('class="hljs language-diff"');
    expect(html).toContain('hljs-addition');
    expect(html).toContain('hljs-deletion');
  });
});

describe('renderMarkdown rendering cache', () => {
  it('reuses the rendered html for an unchanged body', () => {
    const body = '# Heading\n\nsome **body** text';

    // Identity matters: the timeline re-renders on every stream delta, and a
    // fresh parse per render is what made long sessions janky.
    expect(renderMarkdown(body)).toBe(renderMarkdown(body));
  });

  it('re-renders when the body changes', () => {
    const first = renderMarkdown('first body');
    const second = renderMarkdown('second body');

    expect(second).not.toBe(first);
    expect(first).toContain('first body');
    expect(second).toContain('second body');
  });

  it('keeps results apart per highlight query', () => {
    const body = 'searchable body';
    const plain = renderMarkdown(body);
    const highlighted = renderMarkdown(body, { textHighlightQuery: 'searchable' });

    expect(highlighted).not.toBe(plain);
    expect(highlighted).toContain('markdown-search-highlight');
    expect(renderMarkdown(body)).toBe(plain);
  });

  it('drops Magic Context markers before parsing', () => {
    const html = renderMarkdown('§12§ thinking about §13§ the parser');

    expect(html).not.toContain('§');
    expect(html).toContain('thinking about');
  });
});

describe('renderHighlightedPlainText markers', () => {
  it('drops Magic Context markers from raw text', () => {
    expect(renderHighlightedPlainText('§7§ raw §8§ text')).toBe('raw text');
  });
});

describe('renderStreamingMarkdownBlocks', () => {
  beforeEach(() => {
    resetStreamingMarkdownBlocks();
  });

  it('waits for a closing fence line without leaking partial markers into code', () => {
    const prefix = '# Title\n\n```text\nline one\n';
    const first = renderStreamingMarkdownBlocks('fence', prefix);
    for (const tail of ['`', '``', '```', '````']) {
      const next = renderStreamingMarkdownBlocks('fence', prefix + tail);
      expect(next[0]).toBe(first[0]);
      expect(next[1]).toBe(first[1]);
      expect(next[1].html).not.toContain('`');
    }
    const completed = prefix + '````\n\nAfter workflow';
    expect(
      renderStreamingMarkdownBlocks('fence', completed)
        .map(block => block.html)
        .join('')
    ).toBe(renderMarkdown(completed));
  });

  it('defers outer-fence repair until the final non-streaming render', () => {
    const source = '```text\nIntro\n```text\nRead source\n```\nAfter\n```\n';
    const options = { repairMalformedOuterFence: true };
    const streamed = renderStreamingMarkdownBlocks('unrepaired', source, options)
      .map(block => block.html)
      .join('');
    expect(streamed).toBe(renderMarkdown(source));
    expect(renderMarkdown(source, options)).not.toBe(streamed);
  });

  it('splits a body into its top-level blocks', () => {
    const blocks = renderStreamingMarkdownBlocks(
      'k',
      '# Title\n\nfirst para\n\n```js\nlet a = 1\n```'
    );

    expect(blocks.map(block => block.key)).toEqual(['0', '1', '2']);
    expect(blocks[0].html).toContain('<h1');
    expect(blocks[1].html).toContain('first para');
    expect(blocks[2].html).toContain('hljs');
  });

  it('reuses settled blocks so only the growing tail is re-rendered', () => {
    const first = renderStreamingMarkdownBlocks('k', '# Title\n\nstreaming');
    const second = renderStreamingMarkdownBlocks('k', '# Title\n\nstreaming more text');

    // Object identity is what lets the renderer skip patching: same object
    // means the identical html string, so no DOM work for settled blocks.
    expect(second[0]).toBe(first[0]);
    expect(second[1]).not.toBe(first[1]);
    expect(second[1].html).toContain('more text');
  });

  it('re-renders a block whose source changed', () => {
    const first = renderStreamingMarkdownBlocks('k', 'alpha');
    const second = renderStreamingMarkdownBlocks('k', 'beta');

    expect(second[0]).not.toBe(first[0]);
    expect(second[0].html).toContain('beta');
  });

  it('keeps block state per stream key', () => {
    const a = renderStreamingMarkdownBlocks('stream-a', 'shared text');
    const b = renderStreamingMarkdownBlocks('stream-b', 'shared text');

    expect(b[0].html).toBe(a[0].html);
    expect(b[0]).not.toBe(a[0]);
  });

  it('drops state for an empty body and recovers afterwards', () => {
    renderStreamingMarkdownBlocks('k', 'text');
    expect(renderStreamingMarkdownBlocks('k', '')).toEqual([]);
    expect(renderStreamingMarkdownBlocks('k', 'text')[0].html).toContain('text');
  });

  it('drops Magic Context markers before splitting', () => {
    const blocks = renderStreamingMarkdownBlocks('k', 'drop §12§, §13§ now');

    expect(blocks).toHaveLength(1);
    expect(blocks[0].html).not.toContain('§');
    expect(blocks[0].html).toContain('drop now');
  });
});
