import dayjs from 'dayjs';
import { invalidateCache } from 'alova';
import Apis, { alovaInstance } from '@/api';
import { useReq } from '@/api/composable';
import type { Task, TaskComment } from '@/types/models';

export interface CreateTaskPayload {
  title: string;
  description: string;
  status?: Task['status'];
  priority?: number;
  tags?: string[];
  worktreeId?: string | null;
  dueDate?: string | null;
}

export interface UpdateTaskPayload {
  title?: string;
  description?: string;
  priority?: number;
  tags?: string[];
  dueDate?: string | null;
}

export interface MoveTaskPayload {
  status?: Task['status'];
  orderIndex?: number;
  worktreeId?: string | null;
}

const normalizeDueDate = (value?: string | null): string | null => {
  if (value === undefined || value === null || value === '') {
    return null;
  }
  const parsed = dayjs(value);
  if (!parsed.isValid()) {
    return null;
  }
  return parsed.endOf('day').toISOString();
};

// 单例模式：在模块顶层创建 useReq 实例，确保缓存共享
const listTasks = useReq((projectId: string) =>
  Apis.task.list({
    pathParams: { projectId },
    params: { page: 1, pageSize: 500 },
  })
);

const getTask = useReq((taskId: string) =>
  Apis.task.getById({
    pathParams: { id: taskId },
    cacheFor: 0,
  })
);

const createTask = useReq((projectId: string, payload: CreateTaskPayload) =>
  Apis.task.create({
    pathParams: { projectId },
    data: {
      title: payload.title,
      description: payload.description ?? '',
      status: payload.status ?? 'todo',
      priority: payload.priority ?? 0,
      tags: payload.tags ?? [],
      worktreeId: payload.worktreeId ?? null,
      dueDate: normalizeDueDate(payload.dueDate),
    },
  })
);

const updateTask = useReq((taskId: string, payload: UpdateTaskPayload) =>
  Apis.task.update({
    pathParams: { id: taskId },
    data: {
      title: payload.title ?? undefined,
      description: payload.description ?? undefined,
      priority: payload.priority ?? undefined,
      tags: payload.tags ?? undefined,
      dueDate: normalizeDueDate(payload.dueDate) ?? undefined,
    },
  })
);

const deleteTask = useReq((taskId: string) =>
  Apis.task.delete({
    pathParams: { id: taskId },
  })
);

const moveTask = useReq((taskId: string, payload: MoveTaskPayload) =>
  Apis.task.move({
    pathParams: { id: taskId },
    data: {
      status: payload.status,
      orderIndex: payload.orderIndex,
      worktreeId: payload.worktreeId ?? undefined,
    },
  })
);

const bindWorktree = useReq((taskId: string, worktreeId: string | null) =>
  Apis.task.bindWorktree({
    pathParams: { id: taskId },
    data: { worktreeId: worktreeId ?? undefined },
  })
);

const listComments = useReq((taskId: string) =>
  Apis.taskComment.list({
    pathParams: { id: taskId },
  })
);

const createComment = useReq((taskId: string, content: string) =>
  Apis.taskComment.create({
    pathParams: { id: taskId },
    data: { content },
  })
);

const deleteCommentReq = useReq((commentId: string) =>
  Apis.taskComment.delete({
    pathParams: { id: commentId },
  })
);

type TaskCacheInvalidationOptions = {
  taskId?: string;
  projectId?: string;
};

/** 使任务相关缓存失效 */
function invalidateTaskCache() {
  // 使用 snapshots.match 匹配包含 /tasks 的请求，然后使其缓存失效
  // TODO: 感觉没啥用，永远查不出来
  const methods = alovaInstance.snapshots.match({
    filter: method => method.url.includes('/tasks'),
  });
  if (methods.length > 0) {
    invalidateCache(methods);
  }
}

// 导出单例实例
export const taskActions = {
  listTasks,
  getTask,
  createTask,
  updateTask,
  deleteTask,
  moveTask,
  bindWorktree,
  listComments,
  createComment,
  deleteCommentReq,
  invalidateTaskCache,
};

/** @deprecated 请直接使用 taskActions 单例 */
export const useTaskActions = () => taskActions;
