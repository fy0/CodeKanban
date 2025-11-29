export interface Project {
  id: string;
  name: string;
  path: string;
  description: string | null;
  defaultBranch: string | null;
  worktreeBasePath: string | null;
  remoteUrl: string | null;
  hidePath: boolean;
  priority: number | null;
  createdAt: string;
  updatedAt: string;
}

export interface Worktree {
  id: string;
  projectId: string;
  branchName: string;
  path: string;
  isMain: boolean;
  headCommit: string | null;
  headCommitDate: string | null;
  statusAhead: number | null;
  statusBehind: number | null;
  statusModified: number | null;
  statusStaged: number | null;
  statusUntracked: number | null;
  statusUpdatedAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface Task {
  id: string;
  projectId: string;
  worktreeId?: string | null;
  branchName: string; // 关联的分支名称，即使worktree被删除也能显示
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
  // Process information
  processPid?: number;
  processStatus?: 'idle' | 'busy' | 'unknown';
  processHasChildren?: boolean;
  runningCommand?: string;
  // AI Assistant information
  aiAssistant?: {
    type: string;
    name: string;
    displayName: string;
    detected: boolean;
    command?: string;
    state?: string;
    stateUpdatedAt?: string;
    stats?: {
      thinkingDuration: number;
      executingDuration: number;
      waitingApprovalDuration: number;
      waitingInputDuration: number;
      currentStateDuration: number;
    };
  };
  taskId?: string;
}

export interface BranchInfo {
  name: string;
  isCurrent: boolean;
  isRemote: boolean;
  headCommit: string;
  hasWorktree?: boolean;
}

export interface BranchListResult {
  local: BranchInfo[];
  remote: BranchInfo[];
}

export interface MergeResult {
  success: boolean;
  conflicts: string[];
  message: string;
}

export interface NotePad {
  id: string;
  projectId?: string | null;
  name: string;
  content: string;
  orderIndex: number;
  createdAt: string;
  updatedAt: string;
}

export interface AIAssistantStatusConfig {
  claudeCode: boolean;
  codex: boolean;
  qwenCode: boolean;
  gemini: boolean;
  cursor: boolean;
  copilot: boolean;
}

export interface DeveloperConfig {
  enableTerminalScrollback: boolean;
  renameSessionTitleEachCommand: boolean;
}
