export function resolveWsUrl(pathOrUrl: string, base?: string) {
  if (!pathOrUrl) {
    throw new Error('无效的 WebSocket 地址');
  }
  if (pathOrUrl.startsWith('ws://') || pathOrUrl.startsWith('wss://')) {
    return pathOrUrl;
  }

  const normalizedBase = (() => {
    if (!base || base.trim() === '') {
      return `${window.location.protocol}//${window.location.host}`;
    }
    if (base.startsWith('//')) {
      return `${window.location.protocol}${base}`;
    }
    return base;
  })();

  const httpBase = normalizedBase.startsWith('http')
    ? normalizedBase
    : `http:${normalizedBase}`;
  const resolved = new URL(pathOrUrl, httpBase);
  resolved.protocol = resolved.protocol === 'https:' ? 'wss:' : 'ws:';
  return resolved.toString();
}
