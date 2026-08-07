import type { GitCapabilityResult, GitEngine, GitOperationCapabilities } from '@/types/models';

export type GitOperation = keyof GitOperationCapabilities;

export function projectSupportsGit(capabilities: GitCapabilityResult | null | undefined): boolean {
  return Boolean(capabilities?.repository && capabilities.mode !== 'unavailable');
}

export function gitOperationEngine(
  capabilities: GitCapabilityResult | null | undefined,
  operation: GitOperation,
  worktreeId?: string | null
): GitEngine {
  if (!capabilities?.repository) {
    return 'unavailable';
  }
  if (worktreeId) {
    return (
      capabilities.worktrees.find(item => item.id === worktreeId)?.engines?.[operation] ??
      'unavailable'
    );
  }
  return capabilities.engines?.[operation] ?? 'unavailable';
}

export function gitOperationAvailable(
  capabilities: GitCapabilityResult | null | undefined,
  operation: GitOperation,
  worktreeId?: string | null
): boolean {
  if (!capabilities?.repository) {
    return false;
  }
  if (worktreeId) {
    const worktree = capabilities.worktrees.find(item => item.id === worktreeId);
    return Boolean(worktree?.operations[operation]);
  }
  return Boolean(capabilities.operations[operation]);
}

export function gitCapabilityReason(
  capabilities: GitCapabilityResult | null | undefined,
  operation: GitOperation,
  worktreeId?: string | null
): string | null {
  if (!capabilities) {
    return 'git_capabilities_unavailable';
  }
  if (gitOperationAvailable(capabilities, operation, worktreeId)) {
    return null;
  }
  const worktree = worktreeId
    ? capabilities.worktrees.find(item => item.id === worktreeId)
    : undefined;
  return worktree?.reasons[0]?.code ?? capabilities.reasons[0]?.code ?? 'git_operation_unsupported';
}
