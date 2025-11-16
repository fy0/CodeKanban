-- name: TaskCountByWorktree :one
SELECT COUNT(1) AS count
FROM tasks
WHERE worktree_id = @worktree_id
  AND deleted_at IS NULL
  AND status IN ('todo', 'in_progress');
