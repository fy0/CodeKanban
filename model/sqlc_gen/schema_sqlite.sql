-- 数据库建表语句
-- 生成时间: 2026-08-13 05:37:01
-- 数据库方言: sqlite
-- 总共 106 条语句


CREATE TABLE "users" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"nickname" text,"avatar" text,"brief" text,"username" text NOT NULL,"password" text NOT NULL,"salt" text NOT NULL,"disabled" numeric NOT NULL DEFAULT false,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX "idx_users_username" ON "users"("username");
CREATE INDEX "idx_users_deleted_at" ON "users"("deleted_at");


CREATE TABLE "user_access_tokens" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"user_id" text NOT NULL,"expired_at" datetime NOT NULL,PRIMARY KEY ("id"));
CREATE INDEX "idx_access_tokens_user_id" ON "user_access_tokens"("user_id");
CREATE INDEX "idx_user_access_tokens_deleted_at" ON "user_access_tokens"("deleted_at");


CREATE TABLE "projects" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"name" text NOT NULL,"path" text NOT NULL,"description" text,"default_branch" text,"worktree_base_path" text,"remote_url" text,"last_sync_at" datetime,"last_accessed_at" datetime,"hide_path" boolean NOT NULL DEFAULT false,"priority" integer,PRIMARY KEY ("id"));
CREATE INDEX "idx_projects_last_accessed_at" ON "projects"("last_accessed_at");
CREATE UNIQUE INDEX "idx_projects_path" ON "projects"("path");
CREATE INDEX "idx_projects_name" ON "projects"("name");
CREATE INDEX "idx_projects_deleted_at" ON "projects"("deleted_at");


CREATE TABLE "project_agent_trusts" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"project_id" text NOT NULL,"agent" text NOT NULL,"trusted_path" text NOT NULL,"trusted_at" datetime NOT NULL,"revoked_at" datetime,PRIMARY KEY ("id"));
CREATE INDEX "idx_project_agent_trusts_revoked_at" ON "project_agent_trusts"("revoked_at");
CREATE UNIQUE INDEX "idx_project_agent_trust" ON "project_agent_trusts"("project_id","agent");
CREATE INDEX "idx_project_agent_trusts_deleted_at" ON "project_agent_trusts"("deleted_at");


CREATE TABLE "worktrees" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"project_id" text NOT NULL,"branch_name" text NOT NULL,"path" text NOT NULL,"is_main" boolean DEFAULT false,"is_bare" boolean DEFAULT false,"head_commit" text,"head_commit_message" text,"head_commit_date" datetime,"status_ahead" integer DEFAULT 0,"status_behind" integer DEFAULT 0,"status_modified" integer DEFAULT 0,"status_staged" integer DEFAULT 0,"status_untracked" integer DEFAULT 0,"status_conflicts" integer DEFAULT 0,"status_updated_at" datetime,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX "idx_worktrees_path" ON "worktrees"("path") WHERE deleted_at IS NULL;
CREATE INDEX "idx_worktrees_branch_name" ON "worktrees"("branch_name");
CREATE INDEX "idx_worktrees_project_id" ON "worktrees"("project_id");
CREATE INDEX "idx_worktrees_deleted_at" ON "worktrees"("deleted_at");


CREATE TABLE "notepads" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"project_id" text,"name" text NOT NULL,"content" text,"order_index" real NOT NULL,PRIMARY KEY ("id"));
CREATE INDEX "idx_notepads_order_index" ON "notepads"("order_index");
CREATE INDEX "idx_notepads_project_id" ON "notepads"("project_id");
CREATE INDEX "idx_notepads_deleted_at" ON "notepads"("deleted_at");


CREATE TABLE "ai_sessions" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"session_id" text NOT NULL,"type" text NOT NULL,"project_path" text NOT NULL,"file_path" text NOT NULL,"model" text,"title" text,"session_started_at" datetime NOT NULL,"last_message_at" datetime,"message_count" integer DEFAULT 0,"assistant_message_count" integer DEFAULT 0,"file_mod_time" datetime NOT NULL,"file_size" integer NOT NULL,PRIMARY KEY ("id"));
CREATE INDEX "idx_ai_sessions_project_path" ON "ai_sessions"("project_path");
CREATE UNIQUE INDEX "idx_session_type" ON "ai_sessions"("session_id","type");
CREATE INDEX "idx_ai_sessions_deleted_at" ON "ai_sessions"("deleted_at");


CREATE TABLE "terminal_restore_sessions" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"project_id" text NOT NULL,"worktree_id" text NOT NULL,"title" text NOT NULL,"order_index" real NOT NULL DEFAULT 0,"initial_working_dir" text NOT NULL,"last_cwd" text NOT NULL,"shell_family" text NOT NULL DEFAULT "","shell_supported" boolean NOT NULL DEFAULT false,"shell_state" text NOT NULL DEFAULT "idle","pending_command" text,"replay_eligible" boolean NOT NULL DEFAULT false,"command_started_at" datetime,PRIMARY KEY ("id"));
CREATE INDEX "idx_terminal_restore_sessions_order_index" ON "terminal_restore_sessions"("order_index");
CREATE INDEX "idx_terminal_restore_sessions_worktree_id" ON "terminal_restore_sessions"("worktree_id");
CREATE INDEX "idx_terminal_restore_sessions_project_id" ON "terminal_restore_sessions"("project_id");
CREATE INDEX "idx_terminal_restore_sessions_deleted_at" ON "terminal_restore_sessions"("deleted_at");


CREATE TABLE "web_sessions" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"project_id" text NOT NULL,"worktree_id" text,"order_index" real NOT NULL DEFAULT 0,"agent" text NOT NULL,"claude_runtime" text NOT NULL DEFAULT "claude","backend" text NOT NULL DEFAULT "legacy_exec","title" text NOT NULL,"title_auto" boolean NOT NULL DEFAULT false,"model" text,"reasoning_effort" text,"workflow_mode" text NOT NULL DEFAULT "default","permission_level" text NOT NULL DEFAULT "elevated","active_call_timeout_enabled" boolean,"auto_retry_enabled" boolean NOT NULL DEFAULT false,"auto_retry_policy_mode" text NOT NULL DEFAULT "default","auto_retry_scope" text NOT NULL DEFAULT "network_only","auto_retry_preset" text NOT NULL DEFAULT "gentle_stop","auto_retry_max_attempts" integer NOT NULL DEFAULT 0,"auto_retry_dispatch_pending_on_failure" boolean NOT NULL DEFAULT false,"permission_mode" text,"cwd" text NOT NULL,"native_session_id" text,"native_leaf_id" text,"source_revision" text,"cyber_policy_flagged" boolean NOT NULL DEFAULT false,"status" text NOT NULL,"assistant_state" text,"has_unread" boolean NOT NULL DEFAULT false,"archived_at" datetime,"activity_at" datetime,"status_updated_at" datetime,"assistant_state_updated_at" datetime,"source_kind" text NOT NULL DEFAULT "codex_app_server","sync_state" text NOT NULL DEFAULT "missing","last_sync_mode" text,"source_created_at" datetime,"source_updated_at" datetime,"last_synced_at" datetime,"thread_path" text,"thread_preview" text,"turn_count" integer NOT NULL DEFAULT 0,"item_count" integer NOT NULL DEFAULT 0,"last_message_at" datetime,"last_event_seq" integer NOT NULL DEFAULT 0,"snapshot_revision" integer NOT NULL DEFAULT 1,"work_duration_ms" integer NOT NULL DEFAULT 0,"work_current_run_id" text,"work_current_run_started_at" datetime,"work_current_run_paused_at" datetime,"work_current_run_paused_duration_ms" integer NOT NULL DEFAULT 0,"work_current_run_pause_depth" integer NOT NULL DEFAULT 0,"work_retry_wait_started_at" datetime,"work_retry_source_run_id" text,"work_timing_backfill_state" text NOT NULL DEFAULT "pending","work_timing_backfill_version" integer NOT NULL DEFAULT 0,"goal_objective" text,"goal_status" text,"goal_token_budget" integer,"goal_tokens_used" integer NOT NULL DEFAULT 0,"goal_time_used_seconds" integer NOT NULL DEFAULT 0,"goal_created_at" datetime,"goal_updated_at" datetime,"total_input_tokens" integer NOT NULL DEFAULT 0,"total_cached_input_tokens" integer NOT NULL DEFAULT 0,"total_output_tokens" integer NOT NULL DEFAULT 0,"total_cost" real NOT NULL DEFAULT 0,"last_completed_input_tokens" integer NOT NULL DEFAULT 0,"last_completed_cached_input_tokens" integer NOT NULL DEFAULT 0,"last_completed_output_tokens" integer NOT NULL DEFAULT 0,"latest_turn_input_tokens" integer NOT NULL DEFAULT 0,"latest_turn_cached_input_tokens" integer NOT NULL DEFAULT 0,"latest_turn_output_tokens" integer NOT NULL DEFAULT 0,"latest_turn_usage_updated_at" datetime,"latest_token_count_input_tokens" integer NOT NULL DEFAULT 0,"latest_token_count_cached_input_tokens" integer NOT NULL DEFAULT 0,"latest_token_count_output_tokens" integer NOT NULL DEFAULT 0,"latest_token_count_total_tokens" integer NOT NULL DEFAULT 0,"latest_token_count_updated_at" datetime,"session_context_window_tokens" integer NOT NULL DEFAULT 0,"session_context_window_observed_at" datetime,"context_baseline_input_tokens" integer NOT NULL DEFAULT 0,"context_baseline_cached_input_tokens" integer NOT NULL DEFAULT 0,"context_baseline_output_tokens" integer NOT NULL DEFAULT 0,"last_context_compaction_at" datetime,"auto_retry_attempt" integer NOT NULL DEFAULT 0,"auto_retry_next_at" datetime,"auto_retry_last_error_code" text,"last_error" text,"sync_error" text,PRIMARY KEY ("id"));
CREATE INDEX "idx_web_session_work_backfill" ON "web_sessions"("work_timing_backfill_state","work_timing_backfill_version");
CREATE INDEX "idx_web_sessions_work_current_run_id" ON "web_sessions"("work_current_run_id");
CREATE INDEX "idx_web_sessions_source_updated_at" ON "web_sessions"("source_updated_at");
CREATE INDEX "idx_web_sessions_sync_state" ON "web_sessions"("sync_state");
CREATE INDEX "idx_web_sessions_status_updated_at" ON "web_sessions"("status_updated_at");
CREATE INDEX "idx_web_sessions_activity_at" ON "web_sessions"("activity_at");
CREATE INDEX "idx_web_sessions_archived_at" ON "web_sessions"("archived_at");
CREATE INDEX "idx_web_sessions_assistant_state" ON "web_sessions"("assistant_state");
CREATE INDEX "idx_web_sessions_status" ON "web_sessions"("status");
CREATE INDEX "idx_web_sessions_agent" ON "web_sessions"("agent");
CREATE INDEX "idx_web_sessions_order_index" ON "web_sessions"("order_index");
CREATE INDEX "idx_web_sessions_worktree_id" ON "web_sessions"("worktree_id");
CREATE INDEX "idx_web_sessions_project_id" ON "web_sessions"("project_id");
CREATE INDEX "idx_web_sessions_deleted_at" ON "web_sessions"("deleted_at");


CREATE TABLE "web_session_scheduled_inputs" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"web_session_id" text NOT NULL,"action" text NOT NULL DEFAULT "message","target_id" text,"payload_json" text NOT NULL DEFAULT "{}","mode" text NOT NULL DEFAULT "send","text" text,"attachment_ids_json" text NOT NULL DEFAULT "[]","schedule_kind" text NOT NULL DEFAULT "at_time","scheduled_for" datetime NOT NULL,"idle_since" datetime,"blocking_reasons_json" text NOT NULL DEFAULT "[]","condition_error" text NOT NULL DEFAULT "","status" text NOT NULL DEFAULT "scheduled","last_error" text NOT NULL DEFAULT "","sent_at" datetime,"canceled_at" datetime,PRIMARY KEY ("id"));
CREATE INDEX "idx_web_session_scheduled_inputs_status" ON "web_session_scheduled_inputs"("status");
CREATE INDEX "idx_web_session_scheduled_inputs_scheduled_for" ON "web_session_scheduled_inputs"("scheduled_for");
CREATE INDEX "idx_web_session_scheduled_inputs_schedule_kind" ON "web_session_scheduled_inputs"("schedule_kind");
CREATE INDEX "idx_web_session_scheduled_inputs_mode" ON "web_session_scheduled_inputs"("mode");
CREATE INDEX "idx_web_session_scheduled_inputs_target_id" ON "web_session_scheduled_inputs"("target_id");
CREATE INDEX "idx_web_session_scheduled_inputs_action" ON "web_session_scheduled_inputs"("action");
CREATE INDEX "idx_web_session_scheduled_inputs_web_session_id" ON "web_session_scheduled_inputs"("web_session_id");
CREATE INDEX "idx_web_session_scheduled_inputs_deleted_at" ON "web_session_scheduled_inputs"("deleted_at");


CREATE TABLE "web_session_turns" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"web_session_id" text NOT NULL,"source_thread_id" text,"source_turn_id" text,"order_index" integer NOT NULL,"status" text NOT NULL DEFAULT "completed","error_json" text,"source_created" boolean NOT NULL DEFAULT false,PRIMARY KEY ("id"));
CREATE INDEX "idx_web_session_turns_source_turn_id" ON "web_session_turns"("source_turn_id");
CREATE INDEX "idx_web_session_turns_source_thread_id" ON "web_session_turns"("source_thread_id");
CREATE INDEX "idx_web_session_turn_source" ON "web_session_turns"("web_session_id","source_thread_id");
CREATE INDEX "idx_web_session_turn_order" ON "web_session_turns"("web_session_id","order_index");
CREATE INDEX "idx_web_session_turns_deleted_at" ON "web_session_turns"("deleted_at");


CREATE TABLE "web_session_items" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"web_session_id" text NOT NULL,"web_turn_id" text,"source_thread_id" text,"source_turn_id" text,"source_item_id" text,"order_index" integer NOT NULL,"run_id" text,"run_duration_ms" integer,"run_outcome" text,"item_kind" text NOT NULL,"item_type" text NOT NULL,"role" text,"status" text,"level" text,"text" text,"done" boolean NOT NULL DEFAULT false,"timestamp" datetime,"observed_at" datetime,"attachments_json" text,"tool_json" text,"detail_json" text,"payload_json" text,PRIMARY KEY ("id"));
CREATE INDEX "idx_web_session_items_observed_at" ON "web_session_items"("observed_at");
CREATE INDEX "idx_web_session_items_timestamp" ON "web_session_items"("timestamp");
CREATE INDEX "idx_web_session_items_item_type" ON "web_session_items"("item_type");
CREATE INDEX "idx_web_session_items_item_kind" ON "web_session_items"("item_kind");
CREATE INDEX "idx_web_session_items_source_item_id" ON "web_session_items"("source_item_id");
CREATE INDEX "idx_web_session_items_source_turn_id" ON "web_session_items"("source_turn_id");
CREATE INDEX "idx_web_session_items_source_thread_id" ON "web_session_items"("source_thread_id");
CREATE INDEX "idx_web_session_items_web_turn_id" ON "web_session_items"("web_turn_id");
CREATE INDEX "idx_web_session_items_web_session_id" ON "web_session_items"("web_session_id");
CREATE INDEX "idx_web_session_item_run" ON "web_session_items"("web_session_id","run_id");
CREATE INDEX "idx_web_session_item_source" ON "web_session_items"("web_session_id","source_thread_id","source_item_id");
CREATE INDEX "idx_web_session_item_order" ON "web_session_items"("web_session_id","order_index");
CREATE INDEX "idx_web_session_items_deleted_at" ON "web_session_items"("deleted_at");


CREATE TABLE "web_session_run_timings" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"web_session_id" text NOT NULL,"run_id" text NOT NULL,"started_at" datetime NOT NULL,"ended_at" datetime,"paused_duration_ms" integer NOT NULL DEFAULT 0,"duration_ms" integer NOT NULL DEFAULT 0,"outcome" text,"terminal_event_seq" integer NOT NULL DEFAULT 0,"backfilled" boolean NOT NULL DEFAULT false,"anchor_item_id" text,"anchor_source_thread_id" text,"anchor_source_turn_id" text,"anchor_source_item_id" text,PRIMARY KEY ("id"));
CREATE INDEX "idx_web_session_run_timings_anchor_item_id" ON "web_session_run_timings"("anchor_item_id");
CREATE INDEX "idx_web_session_run_timings_outcome" ON "web_session_run_timings"("outcome");
CREATE INDEX "idx_web_session_run_timings_web_session_id" ON "web_session_run_timings"("web_session_id");
CREATE UNIQUE INDEX "idx_web_session_run_timing" ON "web_session_run_timings"("web_session_id","run_id");
CREATE INDEX "idx_web_session_run_timings_deleted_at" ON "web_session_run_timings"("deleted_at");


CREATE TABLE "web_session_sub_agents" ("id" text NOT NULL,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,"web_session_id" text NOT NULL,"thread_id" text NOT NULL,"parent_thread_id" text,"agent_path" text,"nickname" text,"role" text,"status" text NOT NULL,"summary" text,"current_turn_id" text,"latest_item_id" text,"latest_order_index" integer NOT NULL DEFAULT 0,"last_event_seq" integer NOT NULL DEFAULT 0,"started_at" datetime,"last_activity_at" datetime,"ended_at" datetime,PRIMARY KEY ("id"));
CREATE INDEX "idx_web_session_sub_agents_last_activity_at" ON "web_session_sub_agents"("last_activity_at");
CREATE INDEX "idx_web_session_sub_agents_current_turn_id" ON "web_session_sub_agents"("current_turn_id");
CREATE INDEX "idx_web_session_sub_agents_status" ON "web_session_sub_agents"("status");
CREATE INDEX "idx_web_session_sub_agents_parent_thread_id" ON "web_session_sub_agents"("parent_thread_id");
CREATE INDEX "idx_web_session_sub_agents_thread_id" ON "web_session_sub_agents"("thread_id");
CREATE INDEX "idx_web_session_sub_agents_web_session_id" ON "web_session_sub_agents"("web_session_id");
CREATE UNIQUE INDEX "idx_web_session_sub_agent_thread" ON "web_session_sub_agents"("web_session_id","thread_id");
CREATE INDEX "idx_web_session_sub_agents_deleted_at" ON "web_session_sub_agents"("deleted_at");
