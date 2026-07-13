export type FileManagerScopeKind = 'project' | 'worktree';
export type FileManagerEntryKind = 'file' | 'directory' | 'symlink';
export type FileManagerGitStatusKind =
  | 'modified'
  | 'added'
  | 'deleted'
  | 'renamed'
  | 'untracked'
  | 'conflicted'
  | 'dirty';
export type FileManagerPreviewKind =
  | 'image'
  | 'text'
  | 'markdown'
  | 'pdf'
  | 'audio'
  | 'video'
  | 'binary';

export interface FileManagerGitStatus {
  kind: FileManagerGitStatusKind;
  previousPath?: string;
}

export interface FileManagerScope {
  id: string;
  kind: FileManagerScopeKind;
  label: string;
  rootPath: string;
  worktreeId?: string;
}

export interface FileManagerBreadcrumb {
  name: string;
  path: string;
}

export interface FileManagerEntry {
  name: string;
  path: string;
  kind: FileManagerEntryKind;
  size: number;
  modifiedAt: string;
  mime?: string;
  extension?: string;
  previewKind: FileManagerPreviewKind;
  hidden: boolean;
  gitStatus?: FileManagerGitStatus;
}

export interface FileManagerListResult {
  scope: FileManagerScope;
  currentPath: string;
  parentPath?: string;
  breadcrumbs: FileManagerBreadcrumb[];
  entries: FileManagerEntry[];
}

export interface FileManagerSearchResult {
  scope: FileManagerScope;
  currentPath: string;
  entries: FileManagerEntry[];
  truncated: boolean;
}

export interface FileManagerPreviewResult {
  entry: FileManagerEntry;
  previewKind: FileManagerPreviewKind;
  textContent?: string;
  truncated: boolean;
  inlineUrl: string;
  downloadUrl: string;
}

export interface FileManagerChangeEntry {
  name: string;
  path: string;
  previewKind: FileManagerPreviewKind;
  hidden: boolean;
  exists: boolean;
  status: FileManagerGitStatus;
  additions: number;
  deletions: number;
  statsAvailable: boolean;
}

export interface FileManagerChangesResult {
  scope: FileManagerScope;
  entries: FileManagerChangeEntry[];
  changeToken: string;
  truncated: boolean;
  statsComplete: boolean;
  statsTimedOut: boolean;
  untrackedIncluded: boolean;
  warningReason?: string;
}

export interface FileManagerChangesSummaryResult {
  scope: FileManagerScope;
  count: number;
  changeToken: string;
  additions: number | null;
  deletions: number | null;
  statsComplete: boolean;
  statsTimedOut: boolean;
}

export interface FileManagerDiffResult {
  path: string;
  status?: FileManagerGitStatus;
  available: boolean;
  reason?: string;
  previousPath?: string;
  diffText?: string;
  comparedTo: 'HEAD';
}

export interface FileManagerArchiveJob {
  archiveId: string;
  fileName: string;
  size: number;
  createdAt: string;
  expiresAt: string;
  downloadUrl: string;
}

export interface FileManagerUploadSession {
  uploadId: string;
  projectId: string;
  scopeId: string;
  directoryPath: string;
  targetPath: string;
  fileName: string;
  size: number;
  offset: number;
  chunkSize: number;
  createdAt: string;
  updatedAt: string;
  expiresAt: string;
}

export interface FileManagerBulkFailure {
  path: string;
  name: string;
  message: string;
}

export interface FileManagerBulkResult {
  succeeded: Array<{
    path: string;
    name: string;
  }>;
  failed: FileManagerBulkFailure[];
}

export type FileTransferDirection = 'upload' | 'download';
export type FileTransferStatus =
  | 'queued'
  | 'running'
  | 'paused'
  | 'completed'
  | 'failed'
  | 'canceled';

export interface FileTransferTask {
  id: string;
  projectId: string;
  scopeId: string;
  directoryPath: string;
  direction: FileTransferDirection;
  name: string;
  status: FileTransferStatus;
  loaded: number;
  total?: number;
  progress: number | null;
  speed: number;
  error?: string;
  retryAttempt?: number;
  retryMaxAttempts?: number;
  createdAt: number;
  updatedAt: number;
}
