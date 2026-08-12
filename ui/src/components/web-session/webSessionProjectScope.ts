import type { WebSessionAgent } from '@/types/models';

export type WebSessionProjectScopeProject = {
  id: string;
  name?: string | null;
  path?: string | null;
};

export type WebSessionProjectScopeWorktree = {
  id: string;
  projectId: string;
  path?: string | null;
};

export type WebSessionDraftProjectPresentation = {
  title: string;
  cwd: string;
  worktreeId: string | null;
};

export type WebSessionProjectInitializationToken = {
  projectId: string;
  version: number;
};

function normalizeText(value: unknown) {
  return typeof value === 'string' ? value.trim() : '';
}

export function resolveWebSessionScopedProject(
  projectId: string,
  currentProject: WebSessionProjectScopeProject | null | undefined,
  projects: readonly WebSessionProjectScopeProject[]
) {
  const normalizedProjectId = normalizeText(projectId);
  if (!normalizedProjectId) {
    return null;
  }
  if (normalizeText(currentProject?.id) === normalizedProjectId) {
    return currentProject ?? null;
  }
  return projects.find(project => normalizeText(project.id) === normalizedProjectId) ?? null;
}

export function resolveWebSessionDraftProjectPresentation(input: {
  projectId: string;
  agent: WebSessionAgent;
  worktreeId?: string | null;
  currentProject?: WebSessionProjectScopeProject | null;
  projects?: readonly WebSessionProjectScopeProject[];
  worktrees?: readonly WebSessionProjectScopeWorktree[];
  worktreesReady?: boolean;
}): WebSessionDraftProjectPresentation {
  const projectId = normalizeText(input.projectId);
  const project = resolveWebSessionScopedProject(
    projectId,
    input.currentProject,
    input.projects ?? []
  );
  const projectName = normalizeText(project?.name);
  const projectPath = normalizeText(project?.path);
  const baseAgent = input.agent === 'claude' ? 'Claude' : input.agent === 'pi' ? 'Pi' : 'Codex';
  const requestedWorktreeId = normalizeText(input.worktreeId);
  const worktree = requestedWorktreeId
    ? (input.worktrees ?? []).find(
        item =>
          normalizeText(item.id) === requestedWorktreeId &&
          normalizeText(item.projectId) === projectId
      )
    : null;

  if (worktree) {
    return {
      title: projectName ? `${baseAgent} · ${projectName}` : baseAgent,
      cwd: normalizeText(worktree.path),
      worktreeId: worktree.id,
    };
  }

  return {
    title: projectName ? `${baseAgent} · ${projectName}` : baseAgent,
    cwd: requestedWorktreeId && !input.worktreesReady ? '' : projectPath,
    worktreeId: requestedWorktreeId && !input.worktreesReady ? requestedWorktreeId : null,
  };
}

export function createWebSessionProjectInitializationGate() {
  let version = 0;

  return {
    begin(projectId: string): WebSessionProjectInitializationToken {
      version += 1;
      return {
        projectId: normalizeText(projectId),
        version,
      };
    },
    invalidate() {
      version += 1;
    },
    isCurrent(token: WebSessionProjectInitializationToken, currentProjectId: string) {
      return token.version === version && token.projectId === normalizeText(currentProjectId);
    },
  };
}
