import type { BranchListResult, MergeResult } from '@/types/models';
import { http } from './http';

export const branchApi = {
  async list(projectId: string): Promise<BranchListResult> {
    const body = (await http.Get<{ body?: { item?: BranchListResult } }>(`/projects/${projectId}/branches`).send()) ?? {};
    const item = body.body?.item;
    return item ?? { local: [], remote: [] };
  },

  async create(
    projectId: string,
    payload: { name: string; base?: string; createWorktree?: boolean },
  ): Promise<void> {
    await http
      .Post(`/projects/${projectId}/branches`, {
        name: payload.name,
        base: payload.base ?? '',
        createWorktree: payload.createWorktree ?? false,
      })
      .send();
  },

  async delete(projectId: string, branchName: string, force = false): Promise<void> {
    await http.Delete(`/projects/${projectId}/branches/${encodeURIComponent(branchName)}?force=${force}`).send();
  },

  async merge(
    worktreeId: string,
    payload: {
      targetBranch: string;
      sourceBranch: string;
      strategy?: 'merge' | 'rebase' | 'squash';
      commit?: boolean;
      commitMessage?: string;
    },
  ): Promise<MergeResult> {
    const body =
      (await http
        .Post<{ body?: { item?: MergeResult } }>(`/worktrees/${worktreeId}/merge`, {
          targetBranch: payload.targetBranch,
          sourceBranch: payload.sourceBranch,
          strategy: payload.strategy ?? 'merge',
          commit: payload.commit ?? false,
          commitMessage: payload.commitMessage ?? '',
        })
        .send()) ?? {};
    return (
      body.body?.item ?? {
        success: false,
        message: 'unknown result',
        conflicts: [],
      }
    );
  },
};
