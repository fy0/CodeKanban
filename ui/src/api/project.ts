import type { Project, Worktree } from '@/types/models';
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
    const body = (await http.Get<ListProjectsResponse>('/projects').send()) ?? {};
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

  async create(data: { name: string; path: string; description?: string }): Promise<Project> {
    const body = (await http.Post<ItemResponse<Project>>('/projects', data).send()) ?? {};
    if (!body.item) {
      throw new Error('failed to create project');
    }
    return body.item;
  },

  async delete(id: string): Promise<void> {
    await http.Delete(`/projects/${id}`).send();
  },
};

export const worktreeApi = {
  async list(projectId: string): Promise<Worktree[]> {
    const body =
      (await http.Get<{ items?: Worktree[] }>(`/projects/${projectId}/worktrees`).send()) ?? {};
    return body.items ?? [];
  },

  async create(
    projectId: string,
    data: { branchName: string; baseBranch?: string; createBranch?: boolean },
  ): Promise<Worktree> {
    const payload = {
      branchName: data.branchName,
      baseBranch: data.baseBranch ?? '',
      createBranch: data.createBranch ?? true,
    };
    const body =
      (await http.Post<ItemResponse<Worktree>>(
        `/projects/${projectId}/worktrees`,
        payload,
      ).send()) ?? {};
    if (!body.item) {
      throw new Error('failed to create worktree');
    }
    return body.item;
  },

  async delete(id: string, force = false, deleteBranch = false): Promise<void> {
    await http.Delete(`/worktrees/${id}?force=${force}&deleteBranch=${deleteBranch}`).send();
  },

  async sync(projectId: string): Promise<void> {
    await http.Post(`/projects/${projectId}/sync-worktrees`, {}).send();
  },
};

export const systemApi = {
  async openExplorer(path: string): Promise<void> {
    await http.Post('/system/open-explorer', { path }).send();
  },
  async openTerminal(path: string): Promise<void> {
    await http.Post('/system/open-terminal', { path }).send();
  },
};
