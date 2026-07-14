export function normalizeWebSessionRevision(value: unknown): string {
  const normalized = typeof value === 'string' ? value.trim() : '';
  if (!/^\d+$/.test(normalized)) {
    return '';
  }
  try {
    return BigInt(normalized) > 0n ? BigInt(normalized).toString() : '';
  } catch {
    return '';
  }
}

export function compareWebSessionRevisions(left: unknown, right: unknown): number | null {
  const normalizedLeft = normalizeWebSessionRevision(left);
  const normalizedRight = normalizeWebSessionRevision(right);
  if (!normalizedLeft || !normalizedRight) {
    return null;
  }
  const leftRevision = BigInt(normalizedLeft);
  const rightRevision = BigInt(normalizedRight);
  if (leftRevision === rightRevision) {
    return 0;
  }
  return leftRevision > rightRevision ? 1 : -1;
}
