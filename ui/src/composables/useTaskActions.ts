import dayjs from 'dayjs';
import Apis from '@/api';
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

export const useTaskActions = () => {
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

  const listTasks = useReq((projectId: string) =>
    Apis.task.list({
      pathParams: { projectId },
      params: { page: 1, pageSize: 500 },
    }),
  );

  const getTask = useReq((taskId: string) =>
    Apis.task.getById({
      pathParams: { id: taskId },
    }),
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
    }),
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
    }),
  );

  const deleteTask = useReq((taskId: string) =>
    Apis.task.delete({
      pathParams: { id: taskId },
    }),
  );

  const moveTask = useReq((taskId: string, payload: MoveTaskPayload) =>
    Apis.task.move({
      pathParams: { id: taskId },
      data: {
        status: payload.status,
        orderIndex: payload.orderIndex,
        worktreeId: payload.worktreeId ?? undefined,
      },
    }),
  );

  const bindWorktree = useReq((taskId: string, worktreeId: string | null) =>
    Apis.task.bindWorktree({
      pathParams: { id: taskId },
      data: { worktreeId: worktreeId ?? undefined },
    }),
  );

  const listComments = useReq((taskId: string) =>
    Apis.taskComment.list({
      pathParams: { id: taskId },
    }),
  );

  const createComment = useReq((taskId: string, content: string) =>
    Apis.taskComment.create({
      pathParams: { id: taskId },
      data: { content },
    }),
  );

  const deleteCommentReq = useReq((commentId: string) =>
    Apis.taskComment.delete({
      pathParams: { id: commentId },
    }),
  );

  return {
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
  };
};
