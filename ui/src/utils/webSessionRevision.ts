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

export function normalizeWebSessionAttentionRevision(value: unknown): string {
  const normalized = typeof value === 'string' ? value.trim() : '';
  if (!/^\d+$/.test(normalized)) {
    return '';
  }
  try {
    const revision = BigInt(normalized);
    return revision >= 0n ? revision.toString() : '';
  } catch {
    return '';
  }
}

export function compareWebSessionAttentionRevisions(left: unknown, right: unknown): number | null {
  const normalizedLeft = normalizeWebSessionAttentionRevision(left);
  const normalizedRight = normalizeWebSessionAttentionRevision(right);
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
