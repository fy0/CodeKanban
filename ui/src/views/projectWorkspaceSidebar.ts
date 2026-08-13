export const PROJECT_SIDEBAR_DEFAULT_WIDTH = 240;
export const PROJECT_SIDEBAR_EXPANDED_MIN_WIDTH = 200;
export const PROJECT_SIDEBAR_MAX_WIDTH = 400;
export const PROJECT_SIDEBAR_COMPACT_WIDTH = 68;
// Delay compact mode so narrow widths can keep using wrapped text before switching to icon-only.
export const PROJECT_SIDEBAR_COMPACT_SNAP_THRESHOLD = 120;
export const WORKTREE_SIDEBAR_MIN_WIDTH = 240;
export const WORKTREE_SIDEBAR_DEFAULT_WIDTH = 280;
export const WORKTREE_SIDEBAR_MAX_WIDTH = 320;
export const MIN_MAIN_WORKSPACE_WIDTH = 320;

export interface ProjectSidebarBoundsOptions {
  windowWidth: number;
  worktreeCollapsed: boolean;
  worktreeWidth: number;
}

export function clampProjectSidebarWidth(min: number, value: number, max: number) {
  return Math.max(min, Math.min(max, value));
}

export function normalizeWorktreeSidebarWidth(value: unknown) {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return WORKTREE_SIDEBAR_DEFAULT_WIDTH;
  }
  return clampProjectSidebarWidth(
    WORKTREE_SIDEBAR_MIN_WIDTH,
    Math.round(value),
    WORKTREE_SIDEBAR_MAX_WIDTH
  );
}

export function resolveProjectSidebarMaxWidth({
  windowWidth,
  worktreeCollapsed,
  worktreeWidth,
}: ProjectSidebarBoundsOptions) {
  const reservedWorktreeWidth = worktreeCollapsed
    ? 0
    : normalizeWorktreeSidebarWidth(worktreeWidth);
  const maxByViewport = windowWidth - reservedWorktreeWidth - MIN_MAIN_WORKSPACE_WIDTH;

  return Math.min(
    PROJECT_SIDEBAR_MAX_WIDTH,
    Math.max(PROJECT_SIDEBAR_EXPANDED_MIN_WIDTH, Math.round(maxByViewport))
  );
}

export function resolveProjectSidebarDragWidth(storedWidth: number, maxWidth: number) {
  return clampProjectSidebarWidth(
    PROJECT_SIDEBAR_COMPACT_WIDTH,
    Math.round(storedWidth),
    Math.round(maxWidth)
  );
}

export function isProjectSidebarCompact(width: number) {
  return Math.round(width) < PROJECT_SIDEBAR_COMPACT_SNAP_THRESHOLD;
}
