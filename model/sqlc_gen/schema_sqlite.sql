-- 数据库建表语句
-- 生成时间: 2025-10-07 04:44:36
-- 数据库方言: sqlite
-- 总共 3 条语句


CREATE TABLE `example_users` (`id` text,`created_at` datetime,`updated_at` datetime,`deleted_at` datetime,`name` text,`note` text,PRIMARY KEY (`id`));
CREATE INDEX `idx_example_users_deleted_at` ON `example_users`(`deleted_at`);

