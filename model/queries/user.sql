-- name: UserGetByUsername :one
SELECT * FROM users
WHERE username = @username
  AND deleted_at IS NULL
LIMIT 1;

-- name: UserGetById :one
SELECT * FROM users
WHERE id = @id
  AND deleted_at IS NULL
LIMIT 1;

-- name: UserCreate :one
INSERT INTO users (
  id,
  created_at,
  updated_at,
  nickname,
  avatar,
  brief,
  username,
  password,
  salt,
  disabled
) VALUES (
  @id,
  @created_at,
  @updated_at,
  @nickname,
  @avatar,
  @brief,
  @username,
  @password,
  @salt,
  @disabled
) RETURNING
  id,
  created_at,
  updated_at,
  deleted_at,
  nickname,
  avatar,
  brief,
  username,
  password,
  salt,
  disabled;

-- name: UserUpdatePassword :exec
UPDATE users
SET
  updated_at = @updated_at,
  password = @password,
  salt = @salt
WHERE id = @id
  AND deleted_at IS NULL;

-- name: UserUpdateInfo :one
UPDATE users
SET
  updated_at = @updated_at,
  nickname = COALESCE(@nickname, nickname),
  avatar = COALESCE(@avatar, avatar),
  brief = COALESCE(@brief, brief)
WHERE id = @id
  AND deleted_at IS NULL
RETURNING
  id,
  created_at,
  updated_at,
  deleted_at,
  nickname,
  avatar,
  brief,
  username,
  password,
  salt,
  disabled;

-- name: UserDisable :exec
UPDATE users
SET
  updated_at = @updated_at,
  disabled = @disabled
WHERE id = @id
  AND deleted_at IS NULL;

-- name: UserDelete :exec
UPDATE users
SET
  deleted_at = @deleted_at,
  updated_at = @updated_at
WHERE id = @id
  AND deleted_at IS NULL;

-- name: UserList :many
SELECT
  id,
  created_at,
  updated_at,
  deleted_at,
  nickname,
  avatar,
  brief,
  username,
  '' AS password,
  '' AS salt,
  disabled
FROM users
WHERE deleted_at IS NULL
  AND (CAST(@keyword AS TEXT) = '' OR COALESCE(nickname, '') LIKE CAST(@keyword AS TEXT) OR COALESCE(username, '') LIKE CAST(@keyword AS TEXT))
  AND (COALESCE(CAST(@include_disabled AS BOOLEAN), 0) = 1 OR disabled = 0)
ORDER BY created_at DESC
LIMIT @limit
OFFSET @offset;

-- name: UserListCount :one
SELECT COUNT(1) AS count
FROM users
WHERE deleted_at IS NULL
  AND (CAST(@keyword AS TEXT) = '' OR COALESCE(nickname, '') LIKE CAST(@keyword AS TEXT) OR COALESCE(username, '') LIKE CAST(@keyword AS TEXT))
  AND (COALESCE(CAST(@include_disabled AS BOOLEAN), 0) = 1 OR disabled = 0);

-- name: AccessTokenCreate :one
INSERT INTO user_access_tokens (
  id,
  created_at,
  updated_at,
  user_id,
  expired_at
) VALUES (
  @id,
  @created_at,
  @updated_at,
  @user_id,
  @expired_at
) RETURNING
  id,
  created_at,
  updated_at,
  user_id,
  expired_at;

-- name: AccessTokenGetById :one
SELECT
  id,
  created_at,
  updated_at,
  user_id,
  expired_at
FROM user_access_tokens
WHERE id = @id
LIMIT 1;

-- name: AccessTokenDeleteAllByUserId :exec
DELETE FROM user_access_tokens
WHERE user_id = @user_id;

-- name: AccessTokenRefresh :exec
UPDATE user_access_tokens
SET
  updated_at = @updated_at,
  expired_at = @expired_at
WHERE id = @id;
