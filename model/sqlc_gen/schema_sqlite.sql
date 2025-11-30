-- 数据库建表语句
-- 生成时间: 2025-11-30 04:41:04
-- 数据库方言: sqlite
-- 总共 36 条语句


CREATE TABLE "users" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"nickname" text,"avatar" text,"brief" text,"username" text NOT NULL,"password" text NOT NULL,"salt" text NOT NULL,"disabled" numeric NOT NULL DEFAULT false,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX "idx_users_username" ON "users"("username");
CREATE INDEX "idx_users_deleted_at" ON "users"("deleted_at");


CREATE TABLE "user_access_tokens" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"user_id" text NOT NULL,"expired_at" datetime NOT NULL,PRIMARY KEY ("id"));
CREATE INDEX "idx_access_tokens_user_id" ON "user_access_tokens"("user_id");
CREATE INDEX "idx_user_access_tokens_deleted_at" ON "user_access_tokens"("deleted_at");


CREATE TABLE "projects" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"name" text NOT NULL,"path" text NOT NULL,"description" text,"default_branch" text,"worktree_base_path" text,"remote_url" text,"last_sync_at" datetime,"hide_path" boolean NOT NULL DEFAULT false,"priority" integer,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX "idx_projects_path" ON "projects"("path");
CREATE INDEX "idx_projects_name" ON "projects"("name");
CREATE INDEX "idx_projects_deleted_at" ON "projects"("deleted_at");


CREATE TABLE "worktrees" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"project_id" text NOT NULL,"branch_name" text NOT NULL,"path" text NOT NULL,"is_main" boolean DEFAULT false,"is_bare" boolean DEFAULT false,"head_commit" text,"head_commit_message" text,"head_commit_date" datetime,"status_ahead" integer DEFAULT 0,"status_behind" integer DEFAULT 0,"status_modified" integer DEFAULT 0,"status_staged" integer DEFAULT 0,"status_untracked" integer DEFAULT 0,"status_conflicts" integer DEFAULT 0,"status_updated_at" datetime,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX "idx_worktrees_path" ON "worktrees"("path") WHERE deleted_at IS NULL;
CREATE INDEX "idx_worktrees_branch_name" ON "worktrees"("branch_name");
CREATE INDEX "idx_worktrees_project_id" ON "worktrees"("project_id");
CREATE INDEX "idx_worktrees_deleted_at" ON "worktrees"("deleted_at");


CREATE TABLE "tasks" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"project_id" text NOT NULL,"worktree_id" text,"branch_name" text,"title" text NOT NULL,"description" text,"status" text NOT NULL,"priority" integer DEFAULT 0,"order_index" real NOT NULL,"tags" text,"due_date" datetime,"completed_at" datetime,PRIMARY KEY ("id"));
CREATE INDEX "idx_tasks_order_index" ON "tasks"("order_index");
CREATE INDEX "idx_tasks_priority" ON "tasks"("priority");
CREATE INDEX "idx_tasks_status" ON "tasks"("status");
CREATE INDEX "idx_tasks_worktree_id" ON "tasks"("worktree_id");
CREATE INDEX "idx_tasks_project_id" ON "tasks"("project_id");
CREATE INDEX "idx_tasks_deleted_at" ON "tasks"("deleted_at");


CREATE TABLE "task_comments" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"task_id" text NOT NULL,"content" text NOT NULL,PRIMARY KEY ("id"));
CREATE INDEX "idx_task_comments_task_id" ON "task_comments"("task_id");
CREATE INDEX "idx_task_comments_deleted_at" ON "task_comments"("deleted_at");


CREATE TABLE "notepads" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"project_id" text,"name" text NOT NULL,"content" text,"order_index" real NOT NULL,PRIMARY KEY ("id"));
CREATE INDEX "idx_notepads_order_index" ON "notepads"("order_index");
CREATE INDEX "idx_notepads_project_id" ON "notepads"("project_id");
CREATE INDEX "idx_notepads_deleted_at" ON "notepads"("deleted_at");

