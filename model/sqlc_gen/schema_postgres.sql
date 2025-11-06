-- 数据库建表语句
-- 生成时间: 2025-10-07 04:00:01
-- 数据库方言: postgres
-- 总共 3 条语句


CREATE TABLE "example_users" ("id" text,"created_at" timestamptz,"updated_at" timestamptz,"deleted_at" timestamptz,"name" varchar(128),"note" varchar(255),PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_example_users_deleted_at" ON "example_users" ("deleted_at");

