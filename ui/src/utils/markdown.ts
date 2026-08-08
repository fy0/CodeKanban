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
import { Marked, type Tokens } from 'marked';
import { resolveCopyableAbsoluteHref } from '@/utils/messageLinkNavigation';

type HljsLanguageModule = Parameters<typeof hljs.registerLanguage>[1];
export interface RenderMarkdownOptions {
  disableCodeHighlight?: boolean;
  enableCodeBlockCopy?: boolean;
  codeBlockCopyLabel?: string;
  enableLinkCopy?: boolean;
  linkCopyLabel?: string;
  textHighlightQuery?: string;
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

  while ((tagMatch = tagPattern.exec(value))) {
    result += highlightRenderedTextSegment(value.slice(cursor, tagMatch.index), matcher);
    result += tagMatch[0];
    cursor = tagPattern.lastIndex;
  }

  result += highlightRenderedTextSegment(value.slice(cursor), matcher);
  return result;
}

function highlightRenderedTextSegment(value: string, matcher: RegExp) {
  matcher.lastIndex = 0;
  return value.replace(matcher, match => `<mark class="markdown-search-highlight">${match}</mark>`);
}

export function renderHighlightedPlainText(value: string, query?: string) {
  return highlightRenderedText(escapeHtml(value), query ?? '');
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

  renderer.use({
    renderer: {
      code(token) {
        return renderCodeBlock(token, options);
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

export function renderMarkdown(value: string, options: RenderMarkdownOptions = {}) {
  if (!value) {
    return '';
  }

  try {
    const rendered = getMarkdownRenderer(options).parse(value) as string;
    return highlightRenderedText(rendered, options.textHighlightQuery ?? '');
  } catch {
    const rendered = escapeHtml(value).replace(/\n/g, '<br>');
    return highlightRenderedText(rendered, options.textHighlightQuery ?? '');
  }
}
