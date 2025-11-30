-- name: WorktreeCreate :one
INSERT INTO worktrees (
  id,
  created_at,
  updated_at,
  project_id,
  branch_name,
  path,
  is_main,
  is_bare,
  head_commit,
  head_commit_message,
  head_commit_date,
  status_ahead,
  status_behind,
  status_modified,
  status_staged,
  status_untracked,
  status_conflicts,
  status_updated_at
) VALUES (
  @id,
  @created_at,
  @updated_at,
  @project_id,
  @branch_name,
  @path,
  @is_main,
  @is_bare,
  @head_commit,
  @head_commit_message,
  @head_commit_date,
  @status_ahead,
  @status_behind,
  @status_modified,
  @status_staged,
  @status_untracked,
  @status_conflicts,
  @status_updated_at
) RETURNING *;

-- name: WorktreeGetByID :one
SELECT
  id,
  created_at,
  updated_at,
  deleted_at,
  project_id,
  branch_name,
  path,
  is_main,
  is_bare,
  head_commit,
  head_commit_message,
  head_commit_date,
  status_ahead,
  status_behind,
  status_modified,
  status_staged,
  status_untracked,
  status_conflicts,
  status_updated_at
FROM worktrees
WHERE id = @id
  AND deleted_at IS NULL
LIMIT 1;

-- name: WorktreeListByProject :many
SELECT * FROM worktrees
WHERE project_id = @project_id
  AND deleted_at IS NULL
ORDER BY is_main DESC, created_at ASC;

-- name: WorktreeSoftDelete :execrows
UPDATE worktrees
SET
  deleted_at = @deleted_at,
  updated_at = @updated_at
WHERE id = @id
  AND deleted_at IS NULL;

-- name: WorktreeUpdateStatus :one
UPDATE worktrees
SET
  updated_at = @updated_at,
  status_ahead = @status_ahead,
  status_behind = @status_behind,
  status_modified = @status_modified,
  status_staged = @status_staged,
  status_untracked = @status_untracked,
  status_conflicts = @status_conflicts,
  status_updated_at = @status_updated_at,
  head_commit = COALESCE(@head_commit, head_commit),
  head_commit_message = COALESCE(@head_commit_message, head_commit_message),
  head_commit_date = COALESCE(@head_commit_date, head_commit_date)
WHERE id = @id
  AND deleted_at IS NULL
RETURNING
  id,
  created_at,
  updated_at,
  deleted_at,
  project_id,
  branch_name,
  path,
  is_main,
  is_bare,
  head_commit,
  head_commit_message,
  head_commit_date,
  status_ahead,
  status_behind,
  status_modified,
  status_staged,
  status_untracked,
  status_conflicts,
  status_updated_at;

-- name: WorktreeUpdateMetadata :exec
UPDATE worktrees
SET
  updated_at = @updated_at,
  branch_name = @branch_name,
  head_commit = @head_commit,
  head_commit_message = NULL,
  is_main = @is_main,
  is_bare = @is_bare
WHERE id = @id
  AND deleted_at IS NULL;
