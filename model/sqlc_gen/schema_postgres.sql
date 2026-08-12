-- 数据库建表语句
-- 生成时间: 2025-10-07 04:00:01
-- 数据库方言: postgres

-- 当前模板尚未提供完整的 postgres 专用建表语句。

CREATE TABLE "project_agent_trusts" (
    "id" text NOT NULL,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone,
    "deleted_at" timestamp with time zone,
    "project_id" text NOT NULL,
    "agent" text NOT NULL,
    "trusted_path" text NOT NULL,
    "trusted_at" timestamp with time zone NOT NULL,
    "revoked_at" timestamp with time zone,
    PRIMARY KEY ("id")
);
CREATE INDEX "idx_project_agent_trusts_revoked_at" ON "project_agent_trusts" ("revoked_at");
CREATE UNIQUE INDEX "idx_project_agent_trust" ON "project_agent_trusts" ("project_id", "agent");
CREATE INDEX "idx_project_agent_trusts_deleted_at" ON "project_agent_trusts" ("deleted_at");
