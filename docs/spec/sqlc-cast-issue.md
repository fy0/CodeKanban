# SQLC 与 SQLite CAST 使用经验总结

## 问题背景

在使用 sqlc 生成 SQLite 查询代码时，遇到了 CAST 类型转换相关的问题。本文档总结了相关经验和最佳实践。

## 核心问题

### 1. CAST 语法必须使用标准 SQL 格式

**错误写法：**
```sql
CAST(@keyword, text)  -- PostgreSQL 风格，SQLite 不支持
cast(@keyword, text)  -- 同样不支持
```

**正确写法：**
```sql
CAST(@keyword AS TEXT)  -- 标准 SQL 语法
```

**错误信息：**
```
SQL logic error: near ",": syntax error (1)
```

### 2. CAST 影响 sqlc 生成的 Go 类型

sqlc 会根据 SQL 查询中**第一次出现**的参数类型来推断 Go 结构体中的字段类型。

**示例：**

```sql
-- 如果第一次出现就使用 CAST
WHERE (CAST(@keyword AS TEXT) = '' OR nickname LIKE @keyword)
```

生成的 Go 代码：
```go
type UserListParams struct {
    Keyword string `db:"keyword" json:"keyword"`  // ✅ 正确：string 类型
}
```

**如果不使用 CAST：**

```sql
-- 第一次出现没有 CAST
WHERE (@keyword = '' OR nickname LIKE @keyword)
```

生成的 Go 代码：
```go
type UserListParams struct {
    Keyword interface{} `db:"keyword" json:"keyword"`  // ❌ 错误：interface{} 类型
}
```

### 3. 查询条件中的空值处理

当需要支持可选的查询参数时（如关键词搜索），需要正确处理空字符串。

**推荐写法：**

```sql
WHERE (CAST(@keyword AS TEXT) = ''
       OR COALESCE(nickname, '') LIKE CAST(@keyword AS TEXT)
       OR COALESCE(username, '') LIKE CAST(@keyword AS TEXT))
```

**说明：**
- `CAST(@keyword AS TEXT) = ''`：当 keyword 为空字符串时，忽略搜索条件
- `COALESCE(nickname, '')`：处理可能为 NULL 的字段
- 在所有使用 `@keyword` 的地方都添加 `CAST`，确保类型一致

**不推荐的写法：**

```sql
-- ❌ 使用 IS NULL 判断字符串（逻辑不对）
WHERE (CAST(@keyword AS TEXT) IS NULL OR nickname LIKE @keyword)

-- ❌ 使用 LENGTH 判断（复杂且可能有问题）
WHERE (LENGTH(CAST(@keyword AS TEXT)) = 0 OR nickname LIKE @keyword)

-- ❌ 使用 CASE 语句（过于复杂）
WHERE (
    CASE
        WHEN CAST(@keyword AS TEXT) = '' THEN 1
        ELSE (nickname LIKE CAST(@keyword AS TEXT))
    END
)
```

### 4. 布尔类型参数处理

对于布尔类型的可选参数，使用 COALESCE 设置默认值：

```sql
WHERE (COALESCE(CAST(@include_disabled AS BOOLEAN), 0) = 1 OR disabled = 0)
```

**说明：**
- `CAST(@include_disabled AS BOOLEAN)`：确保类型为 bool
- `COALESCE(..., 0)`：未提供参数时默认为 false
- 这样 sqlc 会生成 `bool` 类型而不是 `interface{}`

## 完整示例

```sql
-- name: UserList :many
SELECT
  id, created_at, updated_at, nickname, username, disabled
FROM users
WHERE deleted_at IS NULL
  AND (CAST(@keyword AS TEXT) = ''
       OR COALESCE(nickname, '') LIKE CAST(@keyword AS TEXT)
       OR COALESCE(username, '') LIKE CAST(@keyword AS TEXT))
  AND (COALESCE(CAST(@include_disabled AS BOOLEAN), 0) = 1 OR disabled = 0)
ORDER BY created_at DESC
LIMIT @limit
OFFSET @offset;
```

生成的 Go 代码：
```go
type UserListParams struct {
    Keyword         string `db:"keyword" json:"keyword"`
    IncludeDisabled bool   `db:"include_disabled" json:"includeDisabled"`
    Offset          int64  `db:"offset" json:"offset"`
    Limit           int64  `db:"limit" json:"limit"`
}
```

## Go 代码中的参数处理

```go
func UserList(ctx context.Context, q *model.Queries, req *UserListRequest, page, size int) ([]*model.User, int64, error) {
    // 关键词搜索：空字符串表示不搜索，有值则添加 % 进行前缀匹配
    keyword := ""
    if trimmed := strings.TrimSpace(req.Keyword); trimmed != "" {
        keyword = trimmed + "%"  // 前缀匹配（性能更好）
    }

    params := &model.UserListParams{
        Keyword:         keyword,  // 传递空字符串或 "xxx%"
        IncludeDisabled: req.IncludeDisabled,  // 传递 bool 值
        Offset:          int64((page - 1) * size),
        Limit:           int64(size),
    }

    return q.UserList(ctx, params)
}
```

## 最佳实践总结

1. **始终使用标准 SQL CAST 语法**：`CAST(@param AS TYPE)`
2. **在第一次使用参数时就加 CAST**：确保 sqlc 生成正确的 Go 类型
3. **保持一致**：如果参数在多处使用，所有地方都要 CAST 到同一类型
4. **处理 NULL 值**：使用 `COALESCE(column, default_value)` 处理可能为 NULL 的列
5. **布尔参数使用 COALESCE 设置默认值**：`COALESCE(CAST(@param AS BOOLEAN), 0)`
6. **字符串空值使用简单比较**：`CAST(@param AS TEXT) = ''` 而不是 `IS NULL`
7. **前缀匹配性能更好**：使用 `keyword%` 而不是 `%keyword%`

## 调试技巧

### 1. 查看生成的 SQL 常量

```go
// 在 *_gen.go 文件中查看
const userList = `-- name: UserList :many
SELECT ...
WHERE (CAST(?1 AS TEXT) = '' OR ...)  // 确认 CAST 语法正确
`
```

### 2. 查看生成的参数类型

```go
type UserListParams struct {
    Keyword string `db:"keyword"`  // 确认是 string 而不是 interface{}
}
```

### 3. 添加临时调试日志

```go
println("DEBUG: keyword=", keyword, "includeDisabled=", includeDisabled)
```

## 相关问题排查

### 问题：查询始终返回所有结果，关键词搜索不生效

**可能原因：**
1. 查询参数没有正确传递到 SQL（检查 API 层的参数绑定）
2. SQL 条件逻辑错误（OR 的优先级问题）
3. 参数类型不匹配

**解决方法：**
1. 添加调试日志查看参数值
2. 检查 Huma 框架的参数绑定（query tag）
3. 确认 SQL 逻辑正确（使用括号明确优先级）

### 问题：sqlc 生成的类型是 interface{} 而不是具体类型

**原因：**
参数在 SQL 中第一次出现时没有 CAST。

**解决方法：**
在参数第一次出现的地方添加 `CAST(@param AS TYPE)`。

## 参考资料

- [SQLite CAST 文档](https://www.sqlite.org/lang_expr.html#castexpr)
- [sqlc 类型推断](https://docs.sqlc.dev/en/stable/howto/named_parameters.html)
- [Huma 框架参数绑定](https://huma.rocks/features/request-inputs/)
