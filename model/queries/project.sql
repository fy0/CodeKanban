
-- name: ProjectGetByID :one
SELECT * FROM projects
WHERE id = @id
  AND deleted_at IS NULL
LIMIT 1;

-- name: ProjectCreate :one
INSERT INTO projects (
  id,
  created_at,
  updated_at,
  name,
  path,
  description,
  default_branch,
  worktree_base_path,
  remote_url,
  hide_path,
  last_sync_at,
  priority
) VALUES (
  @id,
  @created_at,
  @updated_at,
  @name,
  @path,
  @description,
  CAST(@default_branch AS TEXT),
  @worktree_base_path,
  @remote_url,
  @hide_path,
  @last_sync_at,
  @priority
) RETURNING *;

-- name: ProjectList :many
SELECT * FROM projects
WHERE deleted_at IS NULL
ORDER BY created_at DESC;

-- name: ProjectUpdate :one
UPDATE projects
SET
  updated_at = @updated_at,
  name = @name,
  description = @description,
  hide_path = @hide_path
WHERE id = @id
  AND deleted_at IS NULL
RETURNING *;

-- name: ProjectSoftDelete :execrows
UPDATE projects
SET
  deleted_at = @deleted_at,
  updated_at = @updated_at
WHERE id = @id
  AND deleted_at IS NULL;

-- name: ProjectUpdatePriority :one
UPDATE projects
SET
  updated_at = @updated_at,
  priority = @priority
WHERE id = @id
  AND deleted_at IS NULL
RETURNING *;
