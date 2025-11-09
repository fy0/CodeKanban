export interface Project {
  id: string;
  name: string;
  path: string;
  description: string;
  defaultBranch: string;
  worktreeBasePath: string;
  remoteUrl: string;
  createdAt: string;
  updatedAt: string;
}

export interface Worktree {
  id: string;
  projectId: string;
  branchName: string;
  path: string;
  isMain: boolean;
  headCommit: string;
  statusAhead: number;
  statusBehind: number;
  statusModified: number;
  statusStaged: number;
  statusUntracked: number;
  statusUpdatedAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface Task {
  id: string;
  projectId: string;
  worktreeId?: string | null;
  title: string;
  description: string;
  status: 'todo' | 'in_progress' | 'done' | 'archived';
  priority: number;
  orderIndex: number;
  tags: string[];
  dueDate?: string | null;
  completedAt?: string | null;
  createdAt: string;
  updatedAt: string;
  worktree?: Worktree;
}

export interface TaskComment {
  id: string;
  taskId: string;
  content: string;
  createdAt: string;
  updatedAt: string;
}

export interface TerminalSession {
  id: string;
  projectId: string;
  worktreeId: string;
  workingDir: string;
  title: string;
  createdAt: string;
  lastActive: string;
  status: 'starting' | 'running' | 'closed' | 'error';
  wsPath: string;
  wsUrl: string;
  rows: number;
  cols: number;
}
