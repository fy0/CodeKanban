import type { FileManagerScope } from '@/types/fileManager';

export interface MessageFileTarget {
  scopeId: string;
  path: string;
}

export interface ResolveMessageFileTargetOptions {
  workingDirectory?: string;
  preferredScopeId?: string;
}

export interface ResolveMessageLocalFilePathOptions {
  workingDirectory?: string;
}

const WINDOWS_ABSOLUTE_PATH_PATTERN = /^[a-z]:\//i;
const WINDOWS_URL_PATH_PATTERN = /^\/[a-z]:\//i;
const URL_SCHEME_PATTERN = /^[a-z][a-z\d+.-]*:/i;

function stripQueryAndHash(value: string) {
  const queryIndex = value.indexOf('?');
  const hashIndex = value.indexOf('#');
  const indexes = [queryIndex, hashIndex].filter(index => index >= 0);
  const endIndex = indexes.length > 0 ? Math.min(...indexes) : value.length;
  return value.slice(0, endIndex);
}

function decodePath(value: string) {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function normalizeSegments(segments: string[], allowRootParent = false) {
  const normalized: string[] = [];
  for (const segment of segments) {
    if (!segment || segment === '.') {
      continue;
    }
    if (segment === '..') {
      if (normalized.length === 0) {
        if (!allowRootParent) {
          return null;
        }
        continue;
      }
      normalized.pop();
      continue;
    }
    normalized.push(segment);
  }
  return normalized;
}

function normalizeAbsolutePath(value: string): string | null {
  let path = decodePath(stripQueryAndHash(value.trim()))
    .replace(/\\/g, '/')
    .replace(/:\d+(?::\d+)?$/, '');
  if (WINDOWS_URL_PATH_PATTERN.test(path)) {
    path = path.slice(1);
  }

  const isWindowsPath = WINDOWS_ABSOLUTE_PATH_PATTERN.test(path);
  const isPosixPath = path.startsWith('/');
  if (!isWindowsPath && !isPosixPath) {
    return null;
  }

  const prefix = isWindowsPath ? path.slice(0, 2) : '/';
  const remainder = isWindowsPath ? path.slice(2) : path.slice(1);
  const segments = normalizeSegments(remainder.split('/'), true);
  if (!segments) {
    return null;
  }
  if (isWindowsPath) {
    return segments.length > 0 ? `${prefix}/${segments.join('/')}` : `${prefix}/`;
  }
  return segments.length > 0 ? `/${segments.join('/')}` : '/';
}

function normalizeRelativePath(value: string): string | null {
  const path = decodePath(stripQueryAndHash(value.trim())).replace(/\\/g, '/');
  if (
    path.startsWith('/') ||
    path.startsWith('//') ||
    WINDOWS_ABSOLUTE_PATH_PATTERN.test(path) ||
    URL_SCHEME_PATTERN.test(path)
  ) {
    return null;
  }
  const segments = normalizeSegments(path.split('/'));
  return segments?.join('/') ?? null;
}

function resolveBaseUrl(baseHref: string) {
  try {
    return new URL(baseHref);
  } catch {
    return null;
  }
}

function resolveAbsoluteHrefPath(rawHref: string, baseHref: string): string | null {
  const href = rawHref.trim();
  if (!href || href.startsWith('#') || href.startsWith('?') || href.startsWith('//')) {
    return null;
  }

  const directPath = normalizeAbsolutePath(href);
  if (directPath) {
    return directPath;
  }
  if (!href.startsWith('/') && !URL_SCHEME_PATTERN.test(href)) {
    return null;
  }

  try {
    const resolved = new URL(href, baseHref);
    if (resolved.protocol === 'file:') {
      if (resolved.hostname && resolved.hostname !== 'localhost') {
        return null;
      }
      return normalizeAbsolutePath(resolved.pathname);
    }
    if (resolved.protocol !== 'http:' && resolved.protocol !== 'https:') {
      return null;
    }
    const base = resolveBaseUrl(baseHref);
    if (!base || resolved.origin !== base.origin) {
      return null;
    }
    return normalizeAbsolutePath(resolved.pathname);
  } catch {
    return null;
  }
}

function joinAbsoluteAndRelative(root: string, relativePath: string) {
  const normalizedRoot = normalizeAbsolutePath(root);
  if (!normalizedRoot) {
    return null;
  }
  return normalizeAbsolutePath(`${normalizedRoot}/${relativePath}`);
}

function pathComparisonValue(path: string) {
  return WINDOWS_ABSOLUTE_PATH_PATTERN.test(path) ? path.toLowerCase() : path;
}

function relativePathWithinRoot(candidate: string, root: string): string | null {
  const normalizedCandidate = normalizeAbsolutePath(candidate);
  const normalizedRoot = normalizeAbsolutePath(root);
  if (!normalizedCandidate || !normalizedRoot) {
    return null;
  }

  const candidateValue = pathComparisonValue(normalizedCandidate);
  const rootValue = pathComparisonValue(normalizedRoot);
  if (candidateValue === rootValue) {
    return '';
  }

  const rootPrefix = rootValue === '/' ? '/' : `${rootValue.replace(/\/$/, '')}/`;
  if (!candidateValue.startsWith(rootPrefix)) {
    return null;
  }

  let relativePath = normalizedCandidate.slice(rootPrefix.length);
  if (WINDOWS_ABSOLUTE_PATH_PATTERN.test(normalizedCandidate)) {
    relativePath = relativePath.replace(/:\d+(?::\d+)?$/, '');
  }
  return relativePath;
}

function orderedScopes(scopes: FileManagerScope[], preferredScopeId = '') {
  return [...scopes].sort((left, right) => {
    const rootLengthDelta =
      (normalizeAbsolutePath(right.rootPath)?.length ?? 0) -
      (normalizeAbsolutePath(left.rootPath)?.length ?? 0);
    if (rootLengthDelta !== 0) {
      return rootLengthDelta;
    }
    if (left.id === preferredScopeId) {
      return -1;
    }
    if (right.id === preferredScopeId) {
      return 1;
    }
    return 0;
  });
}

function targetForAbsolutePath(
  candidate: string,
  scopes: FileManagerScope[],
  preferredScopeId = ''
): MessageFileTarget | null {
  for (const scope of orderedScopes(scopes, preferredScopeId)) {
    const relativePath = relativePathWithinRoot(candidate, scope.rootPath);
    if (relativePath !== null) {
      return {
        scopeId: scope.id,
        path: relativePath,
      };
    }
  }
  return null;
}

export function isPotentialMessageFileHref(rawHref: string, baseHref: string) {
  const href = rawHref.trim();
  if (!href || href.startsWith('#') || href.startsWith('?')) {
    return false;
  }
  if (resolveAbsoluteHrefPath(href, baseHref)) {
    return true;
  }
  return normalizeRelativePath(href) !== null;
}

export function resolveMessageLocalFilePath(
  rawHref: string,
  baseHref: string,
  options: ResolveMessageLocalFilePathOptions = {}
): string | null {
  const absolutePath = resolveAbsoluteHrefPath(rawHref, baseHref);
  if (absolutePath) {
    return absolutePath;
  }

  const relativePath = normalizeRelativePath(rawHref);
  if (relativePath === null || !options.workingDirectory) {
    return null;
  }
  return joinAbsoluteAndRelative(options.workingDirectory, relativePath);
}

export function resolveMessageLocalFileName(path: string) {
  const normalizedPath = path.trim().replace(/\\/g, '/').replace(/\/+$/, '');
  return normalizedPath.split('/').pop() || normalizedPath;
}

export function resolveMessageFileTarget(
  rawHref: string,
  baseHref: string,
  scopes: FileManagerScope[],
  options: ResolveMessageFileTargetOptions = {}
): MessageFileTarget | null {
  if (scopes.length === 0) {
    return null;
  }

  const absolutePath = resolveAbsoluteHrefPath(rawHref, baseHref);
  if (absolutePath) {
    return targetForAbsolutePath(absolutePath, scopes, options.preferredScopeId);
  }

  const relativePath = normalizeRelativePath(rawHref);
  if (relativePath === null) {
    return null;
  }

  if (options.workingDirectory) {
    const workingDirectoryTarget = joinAbsoluteAndRelative(options.workingDirectory, relativePath);
    if (workingDirectoryTarget) {
      const target = targetForAbsolutePath(
        workingDirectoryTarget,
        scopes,
        options.preferredScopeId
      );
      if (target) {
        return target;
      }
    }
  }

  const fallbackScope = scopes.find(scope => scope.id === options.preferredScopeId) ?? scopes[0];
  return fallbackScope
    ? {
        scopeId: fallbackScope.id,
        path: relativePath,
      }
    : null;
}
