import type { WebSessionPiTreeNode } from '@/api/webSession';

export type WebSessionPiTreeRow = {
  id: string;
  node: WebSessionPiTreeNode;
  depth: number;
};

export function projectWebSessionPiTreeRows(nodes: WebSessionPiTreeNode[]): WebSessionPiTreeRow[] {
  const byId = new Map(nodes.map(node => [node.id, node]));
  const depths = new Map<string, number>();

  function depthOf(node: WebSessionPiTreeNode): number {
    const known = depths.get(node.id);
    if (known !== undefined) return known;

    const seen = new Set<string>([node.id]);
    let depth = 0;
    let parentId = node.parentId ?? '';
    while (parentId && depth < 64) {
      if (seen.has(parentId)) break;
      seen.add(parentId);
      const parent = byId.get(parentId);
      if (!parent) break;
      depth += 1;
      parentId = parent.parentId ?? '';
    }
    depths.set(node.id, depth);
    return depth;
  }

  return nodes.map(node => ({ id: node.id, node, depth: depthOf(node) }));
}

export function canForkWebSessionPiTreeNode(node: WebSessionPiTreeNode | undefined): boolean {
  return Boolean(node && node.type === 'message' && node.role === 'user');
}

export function canOpenWebSessionPiTree(input: {
  archived: boolean;
  agent?: string;
  supportsTree: boolean;
  nativeSessionId?: string;
  threadPath?: string;
}): boolean {
  return Boolean(
    !input.archived &&
      input.agent === 'pi' &&
      input.supportsTree &&
      input.nativeSessionId &&
      input.threadPath
  );
}

export function canMutateWebSessionPiTree(input: {
  canOpen: boolean;
  running: boolean;
  pendingCount: number;
}): boolean {
  return input.canOpen && !input.running && input.pendingCount === 0;
}
