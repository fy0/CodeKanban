import hljs from 'highlight.js/lib/core';
import bash from 'highlight.js/lib/languages/bash';
import c from 'highlight.js/lib/languages/c';
import cpp from 'highlight.js/lib/languages/cpp';
import css from 'highlight.js/lib/languages/css';
import diff from 'highlight.js/lib/languages/diff';
import dockerfile from 'highlight.js/lib/languages/dockerfile';
import go from 'highlight.js/lib/languages/go';
import haxe from 'highlight.js/lib/languages/haxe';
import ini from 'highlight.js/lib/languages/ini';
import java from 'highlight.js/lib/languages/java';
import javascript from 'highlight.js/lib/languages/javascript';
import json from 'highlight.js/lib/languages/json';
import makefile from 'highlight.js/lib/languages/makefile';
import markdown from 'highlight.js/lib/languages/markdown';
import python from 'highlight.js/lib/languages/python';
import rust from 'highlight.js/lib/languages/rust';
import scss from 'highlight.js/lib/languages/scss';
import sql from 'highlight.js/lib/languages/sql';
import typescript from 'highlight.js/lib/languages/typescript';
import xml from 'highlight.js/lib/languages/xml';
import yaml from 'highlight.js/lib/languages/yaml';
import { Marked, Renderer, type Tokens } from 'marked';
import { stripMagicContextTags } from '@/utils/magicContextTags';
import { getStreamingMarkdownSource, repairMalformedOuterFence } from '@/utils/markdownFences';
import { createMarkdownMathExtension } from '@/utils/markdownMath';
import { resolveCopyableAbsoluteHref } from '@/utils/messageLinkNavigation';
import { resolveMarkdownImageSrc } from '@/utils/webSessionImages';

type HljsLanguageModule = Parameters<typeof hljs.registerLanguage>[1];
export interface RenderMarkdownOptions {
  disableCodeHighlight?: boolean;
  enableCodeBlockCopy?: boolean;
  codeBlockCopyLabel?: string;
  enableLinkCopy?: boolean;
  linkCopyLabel?: string;
  textHighlightQuery?: string;
  /** Opt in only for completed chat display, never raw text or file previews. */
  repairMalformedOuterFence?: boolean;
}

const registeredLanguages = new Set<string>();

function registerLanguage(name: string, language: HljsLanguageModule) {
  hljs.registerLanguage(name, language);
  registeredLanguages.add(name);
}

registerLanguage('bash', bash);
registerLanguage('c', c);
registerLanguage('cpp', cpp);
registerLanguage('css', css);
registerLanguage('diff', diff);
registerLanguage('dockerfile', dockerfile);
registerLanguage('go', go);
registerLanguage('haxe', haxe);
registerLanguage('ini', ini);
registerLanguage('java', java);
registerLanguage('javascript', javascript);
registerLanguage('json', json);
registerLanguage('makefile', makefile);
registerLanguage('markdown', markdown);
registerLanguage('python', python);
registerLanguage('rust', rust);
registerLanguage('scss', scss);
registerLanguage('sql', sql);
registerLanguage('typescript', typescript);
registerLanguage('xml', xml);
registerLanguage('yaml', yaml);

const languageAliases: Record<string, string> = {
  cc: 'cpp',
  cjs: 'javascript',
  conf: 'ini',
  cxx: 'cpp',
  docker: 'dockerfile',
  env: 'ini',
  h: 'c',
  hx: 'haxe',
  hpp: 'cpp',
  htm: 'xml',
  html: 'xml',
  js: 'javascript',
  jsx: 'javascript',
  md: 'markdown',
  mjs: 'javascript',
  mts: 'typescript',
  properties: 'ini',
  py: 'python',
  rs: 'rust',
  sass: 'scss',
  sh: 'bash',
  shell: 'bash',
  svg: 'xml',
  ts: 'typescript',
  tsx: 'typescript',
  toml: 'ini',
  vue: 'xml',
  xhtml: 'xml',
  xml: 'xml',
  yml: 'yaml',
  zsh: 'bash',
};

function escapeHtml(value: string) {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function highlightRenderedText(value: string, query: string) {
  const normalizedQuery = query.trim();
  if (!normalizedQuery) {
    return value;
  }

  const queryVariants = [...new Set([normalizedQuery, escapeHtml(normalizedQuery)])]
    .sort((left, right) => right.length - left.length)
    .map(escapeRegExp)
    .join('|');
  if (!queryVariants) {
    return value;
  }

  const matcher = new RegExp(queryVariants, 'gi');
  const tagPattern = /<!--[\s\S]*?-->|<[^>]*>/g;
  let result = '';
  let cursor = 0;
  let tagMatch: RegExpExecArray | null;
  let mathTag = '';
  let mathDepth = 0;

  while ((tagMatch = tagPattern.exec(value))) {
    const segment = value.slice(cursor, tagMatch.index);
    result += mathDepth ? segment : highlightRenderedTextSegment(segment, matcher);
    const tag = tagMatch[0];
    // Search marks would corrupt KaTeX's layout and MathML accessibility tree.
    if (!mathDepth && /\bdata-markdown-math="true"/.test(tag)) {
      mathTag = /^<(div|span)\b/.exec(tag)?.[1] ?? '';
    }
    if (mathTag) {
      if (tag.startsWith('</' + mathTag + '>')) {
        mathDepth -= 1;
        if (!mathDepth) mathTag = '';
      } else if (tag.startsWith('<' + mathTag + '>') || tag.startsWith('<' + mathTag + ' ')) {
        mathDepth += 1;
      }
    }
    result += tag;
    cursor = tagPattern.lastIndex;
  }

  result += highlightRenderedTextSegment(value.slice(cursor), matcher);
  return result;
}

function highlightRenderedTextSegment(value: string, matcher: RegExp) {
  matcher.lastIndex = 0;
  return value.replace(matcher, match => `<mark class="markdown-search-highlight">${match}</mark>`);
}

/**
 * Rendered HTML is cached because the whole timeline re-renders on every stream
 * delta: without it, every visible block re-parses its markdown on each update.
 * The key is the original string so an unchanged block hits the cache by
 * identity instead of rebuilding a composite key out of the whole body.
 */
function createRenderedHtmlCache(limit: number) {
  const buckets = new Map<string, Map<string, string>>();

  return {
    read(variant: string, value: string) {
      const bucket = buckets.get(variant);
      const cached = bucket?.get(value);
      if (cached === undefined || !bucket) {
        return undefined;
      }
      // Keep the most recently used entries when trimming.
      bucket.delete(value);
      bucket.set(value, cached);
      return cached;
    },
    write(variant: string, value: string, html: string) {
      let bucket = buckets.get(variant);
      if (!bucket) {
        bucket = new Map<string, string>();
        buckets.set(variant, bucket);
      }
      bucket.set(value, html);
      while (bucket.size > limit) {
        const oldest = bucket.keys().next().value;
        if (oldest === undefined) {
          break;
        }
        bucket.delete(oldest);
      }
      return html;
    },
  };
}

const plainTextHtmlCache = createRenderedHtmlCache(60);
const markdownHtmlCache = createRenderedHtmlCache(60);

function highlightVariantKey(query?: string) {
  return query ?? '';
}

export function renderHighlightedPlainText(value: string, query?: string) {
  if (!value) {
    return '';
  }
  const variant = highlightVariantKey(query);
  const cached = plainTextHtmlCache.read(variant, value);
  if (cached !== undefined) {
    return cached;
  }
  return plainTextHtmlCache.write(
    variant,
    value,
    highlightRenderedText(escapeHtml(stripMagicContextTags(value)), query ?? '')
  );
}

function pickLanguageName(value?: string) {
  if (!value) {
    return '';
  }
  return value.trim().toLowerCase().split(/\s+/, 1)[0] ?? '';
}

function normalizeLanguage(value?: string) {
  const rawLanguage = pickLanguageName(value);
  if (!rawLanguage) {
    return '';
  }

  const normalized = languageAliases[rawLanguage] ?? rawLanguage;
  return registeredLanguages.has(normalized) ? normalized : '';
}

function renderCodeCopyButton(options: RenderMarkdownOptions = {}) {
  if (!options.enableCodeBlockCopy) {
    return '';
  }

  const label = escapeHtml(options.codeBlockCopyLabel || 'copy');
  return `<button type="button" class="markdown-code-copy-button" data-message-code-copy="true" title="${label}" aria-label="${label}">${label}</button>`;
}

function renderCodeBlock({ text, lang }: Tokens.Code, options: RenderMarkdownOptions = {}) {
  if (text.length === 0) {
    return '';
  }
  const normalizedLanguage = normalizeLanguage(lang);
  const languageLabel = pickLanguageName(lang);
  const shouldHighlight = !options.disableCodeHighlight && Boolean(normalizedLanguage);
  const highlightedCode = shouldHighlight
    ? hljs.highlight(text, {
        language: normalizedLanguage,
        ignoreIllegals: true,
      }).value
    : escapeHtml(text);

  const dataLanguage = languageLabel ? ` data-language="${escapeHtml(languageLabel)}"` : '';
  const dataCodeCopy = options.enableCodeBlockCopy ? ' data-code-copy="true"' : '';
  const languageClass = shouldHighlight ? ` language-${normalizedLanguage}` : '';
  const codeCopyButton = renderCodeCopyButton(options);

  return `<pre class="markdown-code-block"${dataLanguage}${dataCodeCopy}>${codeCopyButton}<code class="hljs${languageClass}">${highlightedCode}</code></pre>`;
}

function renderLinkCopyButton(href: string, options: RenderMarkdownOptions) {
  if (!options.enableLinkCopy) {
    return '';
  }

  const copyHref = resolveCopyableAbsoluteHref(href);
  if (!copyHref) {
    return '';
  }

  const label = escapeHtml(options.linkCopyLabel || 'Copy link');
  return `<button type="button" class="message-link-copy-button" data-message-link-copy="true" data-message-link-copy-href="${escapeHtml(copyHref)}" title="${label}" aria-label="${label}">${label}</button>`;
}

export function renderHighlightedCodeBlock(
  value: string,
  lang?: string,
  options: RenderMarkdownOptions = {}
) {
  if (!value) {
    return '';
  }
  return renderCodeBlock(
    {
      type: 'code',
      raw: value,
      text: value,
      lang,
    },
    options
  );
}

function createMarkdownRenderer(options: RenderMarkdownOptions = {}) {
  const renderer = new Marked({
    async: false,
    breaks: true,
    gfm: true,
  });
  renderer.use(createMarkdownMathExtension());

  renderer.use({
    renderer: {
      code(token) {
        return renderCodeBlock(token, options);
      },
      image(token) {
        return Renderer.prototype.image.call(this, {
          ...token,
          href: resolveMarkdownImageSrc(token.href),
        });
      },
      link(token) {
        const hrefValue = token.href ?? '';
        const href = hrefValue ? ` href="${escapeHtml(hrefValue)}"` : '';
        const title = token.title ? ` title="${escapeHtml(token.title)}"` : '';
        const text = this.parser.parseInline(token.tokens);
        const linkHtml = `<a${href}${title} target="_blank" rel="noopener noreferrer" data-message-link="true">${text}</a>`;
        const copyButtonHtml = renderLinkCopyButton(hrefValue, options);
        if (!copyButtonHtml) {
          return linkHtml;
        }
        return `<span class="message-link-inline">${linkHtml}${copyButtonHtml}</span>`;
      },
    },
  });

  return renderer;
}

const markdownRendererCache = new Map<string, Marked>();

function getMarkdownRenderer(options: RenderMarkdownOptions = {}) {
  const key = JSON.stringify({
    disableCodeHighlight: !!options.disableCodeHighlight,
    enableCodeBlockCopy: !!options.enableCodeBlockCopy,
    codeBlockCopyLabel: options.codeBlockCopyLabel || '',
    enableLinkCopy: !!options.enableLinkCopy,
    linkCopyLabel: options.linkCopyLabel || '',
  });

  const existing = markdownRendererCache.get(key);
  if (existing) {
    return existing;
  }

  const renderer = createMarkdownRenderer(options);
  markdownRendererCache.set(key, renderer);
  return renderer;
}

function markdownOptionsVariantKey(options: RenderMarkdownOptions = {}) {
  return JSON.stringify({
    disableCodeHighlight: !!options.disableCodeHighlight,
    enableCodeBlockCopy: !!options.enableCodeBlockCopy,
    codeBlockCopyLabel: options.codeBlockCopyLabel || '',
    enableLinkCopy: !!options.enableLinkCopy,
    linkCopyLabel: options.linkCopyLabel || '',
    textHighlightQuery: options.textHighlightQuery || '',
    repairMalformedOuterFence: !!options.repairMalformedOuterFence,
  });
}

export function renderMarkdown(value: string, options: RenderMarkdownOptions = {}) {
  if (!value) {
    return '';
  }

  const variant = markdownOptionsVariantKey(options);
  const cached = markdownHtmlCache.read(variant, value);
  if (cached !== undefined) {
    return cached;
  }

  // Magic Context markers only ever addressed the model; strip them before the
  // text reaches the reader.
  const rawSource = stripMagicContextTags(value);
  const source = options.repairMalformedOuterFence
    ? repairMalformedOuterFence(rawSource)
    : rawSource;
  let html: string;
  try {
    const rendered = getMarkdownRenderer(options).parse(source) as string;
    html = highlightRenderedText(rendered, options.textHighlightQuery ?? '');
  } catch {
    const rendered = escapeHtml(source).replace(/\n/g, '<br>');
    html = highlightRenderedText(rendered, options.textHighlightQuery ?? '');
  }
  return markdownHtmlCache.write(variant, value, html);
}

export interface StreamingMarkdownBlock {
  key: string;
  html: string;
}

type MarkdownTokenList = Parameters<Marked['parser']>[0];
type MarkdownToken = MarkdownTokenList[number];

interface StreamingMarkdownState {
  variant: string;
  raws: string[];
  blocks: StreamingMarkdownBlock[];
}

const streamingMarkdownStateByKey = new Map<string, StreamingMarkdownState>();
const STREAMING_MARKDOWN_STATE_LIMIT = 24;

function readStreamingState(stateKey: string) {
  const state = streamingMarkdownStateByKey.get(stateKey);
  if (state) {
    streamingMarkdownStateByKey.delete(stateKey);
    streamingMarkdownStateByKey.set(stateKey, state);
  }
  return state;
}

function writeStreamingState(stateKey: string, state: StreamingMarkdownState) {
  streamingMarkdownStateByKey.set(stateKey, state);
  while (streamingMarkdownStateByKey.size > STREAMING_MARKDOWN_STATE_LIMIT) {
    const oldest = streamingMarkdownStateByKey.keys().next().value;
    if (oldest === undefined || oldest === stateKey) {
      break;
    }
    streamingMarkdownStateByKey.delete(oldest);
  }
}

function renderStreamingMarkdownBlock(token: MarkdownToken, options: RenderMarkdownOptions) {
  const query = options.textHighlightQuery ?? '';
  try {
    const html = getMarkdownRenderer(options).parser([token]) as string;
    return highlightRenderedText(html, query);
  } catch {
    return '';
  }
}

/**
 * Render markdown block by block so a body that is still streaming only pays for
 * the block that actually changed.
 *
 * Whole-body parsing per delta is quadratic over a message and re-runs syntax
 * highlighting over text that already scrolled past. Splitting on top-level
 * tokens keeps every settled block's HTML string identical, which lets the
 * renderer skip patching those nodes entirely; only the growing tail block is
 * re-parsed.
 */
export function renderStreamingMarkdownBlocks(
  stateKey: string,
  value: string,
  options: RenderMarkdownOptions = {}
): StreamingMarkdownBlock[] {
  if (!value) {
    streamingMarkdownStateByKey.delete(stateKey);
    return [];
  }

  const variant = markdownOptionsVariantKey(options);
  // Repair requires a completed message. Streaming only gates the last possible
  // marker line and otherwise retains the standard parser semantics.
  const source = getStreamingMarkdownSource(stripMagicContextTags(value));
  const tokens = (getMarkdownRenderer(options).lexer(source) as MarkdownToken[]).filter(
    token => token.type !== 'space'
  );
  const previous = readStreamingState(stateKey);
  const reusable = previous && previous.variant === variant ? previous : undefined;

  const blocks: StreamingMarkdownBlock[] = [];
  const raws: string[] = [];
  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    const raw = token.raw ?? '';
    raws.push(raw);
    const cached = reusable && reusable.raws[index] === raw ? reusable.blocks[index] : undefined;
    if (cached) {
      // Reusing the object reuses its html string, so the child skips its patch.
      blocks.push(cached);
      continue;
    }
    blocks.push({ key: `${index}`, html: renderStreamingMarkdownBlock(token, options) });
  }

  writeStreamingState(stateKey, { variant, raws, blocks });
  return blocks;
}

/** Test seam: streaming block state must not leak between cases. */
export function resetStreamingMarkdownBlocks() {
  streamingMarkdownStateByKey.clear();
}
