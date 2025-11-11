import type { NotePad } from '@/types/models';
import { http } from './http';

type ItemResponse<T> = {
  item?: T;
};

type ItemsResponse<T> = {
  items?: T[];
};

export const notepadApi = {
  async list(projectId?: string | null): Promise<NotePad[]> {
    // 如果提供了项目ID且不为空，则查询项目笔记；否则查询全局笔记
    const url = projectId ? `/notepads?projectId=${projectId}` : '/notepads';
    const body = (await http.Get<ItemsResponse<NotePad>>(url).send()) ?? {};
    return body.items ?? [];
  },

  async get(id: string): Promise<NotePad> {
    const body = (await http.Get<ItemResponse<NotePad>>(`/notepads/${id}`).send()) ?? {};
    if (!body.item) {
      throw new Error('notepad not found');
    }
    return body.item;
  },

  async create(data: { projectId?: string; name?: string; content?: string }): Promise<NotePad> {
    const body = (await http.Post<ItemResponse<NotePad>>('/notepads', data).send()) ?? {};
    if (!body.item) {
      throw new Error('failed to create notepad');
    }
    return body.item;
  },

  async update(id: string, data: { name?: string; content?: string }): Promise<NotePad> {
    const body = (await http.Patch<ItemResponse<NotePad>>(`/notepads/${id}`, data).send()) ?? {};
    if (!body.item) {
      throw new Error('failed to update notepad');
    }
    return body.item;
  },

  async delete(id: string): Promise<void> {
    await http.Delete(`/notepads/${id}`).send();
  },

  async move(id: string, orderIndex: number): Promise<NotePad> {
    const body =
      (await http.Post<ItemResponse<NotePad>>(`/notepads/${id}/move`, { orderIndex }).send()) ?? {};
    if (!body.item) {
      throw new Error('failed to move notepad');
    }
    return body.item;
  },
};
