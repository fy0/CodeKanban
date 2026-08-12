import type {
  CodexSkillSummary,
  GitCapabilityResult,
  Project,
  ProjectAgentTrustStatus,
  Worktree,
} from '@/types/models';
import { http } from './http';

type ListProjectsResponse = {
  items?: Project[];
  total?: number;
};

type ItemResponse<T> = {
  item?: T;
};

export const projectApi = {
  async list(): Promise<{ items: Project[]; total: number }> {
    const body =
      (await http.Get<ListProjectsResponse>('/projects', { cacheFor: 0 }).send(true)) ?? {};
    const items = body.items ?? [];
    const total = typeof body.total === 'number' ? body.total : items.length;
    return { items, total };
  },

  async get(id: string): Promise<Project> {
    const body = (await http.Get<ItemResponse<Project>>(`/projects/${id}`).send()) ?? {};
    if (!body.item) {
      throw new Error('project not found');
    }
    return body.item;
  },

  async create(data: {
    name: string;
    path: string;
    description?: string;
    hidePath: boolean;
  }): Promise<Project> {
    const body = (await http.Post<ItemResponse<Project>>('/projects/create', data).send()) ?? {};
    if (!body.item) {
      throw new Error('failed to create project');
    }
    return body.item;
  },

  async update(
    id: string,
    data: { name: string; description?: string; hidePath: boolean }
  ): Promise<Project> {
    const body =
      (await http.Post<ItemResponse<Project>>(`/projects/${id}/update`, data).send()) ?? {};
    if (!body.item) {
      throw new Error('failed to update project');
    }
    return body.item;
  },

  async delete(id: string): Promise<void> {
    await http.Post(`/projects/${id}/delete`, {}).send();
  },

  async updatePriority(id: string, priority: number | null): Promise<Project> {
    const body =
      (await http.Post<ItemResponse<Project>>(`/projects/${id}/priority`, { priority }).send()) ??
      {};
    if (!body.item) {
      throw new Error('failed to update project priority');
    }
    return body.item;
  },

  async markAccess(id: string): Promise<Project> {
    const body =
      (await http.Post<ItemResponse<Project>>(`/projects/${id}/access`, {}).send()) ?? {};
    if (!body.item) {
      throw new Error('failed to record project access');
    }
    return body.item;
  },

  async clearAccess(id: string): Promise<Project> {
    const body =
      (await http.Post<ItemResponse<Project>>(`/projects/${id}/access/clear`, {}).send()) ?? {};
    if (!body.item) {
      throw new Error('failed to clear project access');
    }
    return body.item;
  },

  async getPiTrust(id: string): Promise<ProjectAgentTrustStatus> {
    const body =
      (await http
        .Get<ItemResponse<ProjectAgentTrustStatus>>(`/projects/${id}/agent-trust/pi`, {
          cacheFor: 0,
        })
        .send(true)) ?? {};
    if (!body.item) {
      throw new Error('failed to load Pi project access');
    }
    return body.item;
  },

  async trustForPi(id: string): Promise<ProjectAgentTrustStatus> {
    const body =
      (await http
        .Post<ItemResponse<ProjectAgentTrustStatus>>(`/projects/${id}/agent-trust/pi`, {})
        .send()) ?? {};
    if (!body.item) {
      throw new Error('failed to authorize Pi project access');
    }
    return body.item;
  },

  async revokePiTrust(id: string): Promise<ProjectAgentTrustStatus> {
    const body =
      (await http
        .Delete<ItemResponse<ProjectAgentTrustStatus>>(`/projects/${id}/agent-trust/pi`)
        .send()) ?? {};
    if (!body.item) {
      throw new Error('failed to revoke Pi project access');
    }
    return body.item;
  },

  async gitCapabilities(id: string): Promise<GitCapabilityResult> {
    const body =
      (await http
        .Get<ItemResponse<GitCapabilityResult>>(`/projects/${id}/git-capabilities`, {
          cacheFor: 0,
        })
        .send(true)) ?? {};
    if (!body.item) {
      throw new Error('failed to load Git capabilities');
    }
    return body.item;
  },
};

export const worktreeApi = {
  async list(projectId: string): Promise<Worktree[]> {
    const body =
      (await http.Get<{ items?: Worktree[] }>(`/projects/${projectId}/worktrees`).send(true)) ?? {};
    return body.items ?? [];
  },

  async refreshCommitInfo(projectId: string): Promise<Worktree[]> {
    const body =
      (await http
        .Post<{ items?: Worktree[] }>(`/projects/${projectId}/worktrees/refresh-commits`, {})
        .send()) ?? {};
    return body.items ?? [];
  },

  async create(
    projectId: string,
    data: {
      branchName: string;
      baseBranch?: string;
      createBranch?: boolean;
      location?: 'project' | 'global';
      globalBaseDirOverride?: string;
    }
  ): Promise<Worktree> {
    const payload = {
      branchName: data.branchName,
      baseBranch: data.baseBranch ?? '',
      createBranch: data.createBranch ?? true,
      location: data.location,
      globalBaseDirOverride: data.globalBaseDirOverride,
    };
    const body =
      (await http
        .Post<ItemResponse<Worktree>>(`/projects/${projectId}/worktrees/create`, payload)
        .send()) ?? {};
    if (!body.item) {
      throw new Error('failed to create worktree');
    }
    return body.item;
  },

  async delete(id: string, force = false, deleteBranch = false): Promise<void> {
    await http.Post(`/worktrees/${id}?force=${force}&deleteBranch=${deleteBranch}`, {}).send();
  },

  async sync(projectId: string): Promise<void> {
    await http.Post(`/projects/${projectId}/sync-worktrees`, {}).send();
  },
};

export const systemApi = {
  async listCodexSkills(): Promise<CodexSkillSummary[]> {
    const body =
      (await http.Get<{ items?: CodexSkillSummary[] }>('/system/codex-skills').send()) ?? {};
    return body.items ?? [];
  },
  async openExplorer(path: string): Promise<void> {
    await http.Post('/system/open-explorer', { path }).send();
  },
  async openTerminal(path: string): Promise<void> {
    await http.Post('/system/open-terminal', { path }).send();
  },
  async openEditor(options: {
    path: string;
    editor: string;
    customCommand?: string;
  }): Promise<void> {
    await http.Post('/system/open-editor', options).send();
  },
};
