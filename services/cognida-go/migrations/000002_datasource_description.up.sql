-- 为外部数据源新增业务描述字段：供 Data Agent 按业务语义自动选库（list_datasources 工具回显）。
-- 可空、不影响既有行；描述由用户在数据源管理界面维护。
ALTER TABLE `data_sources`
  ADD COLUMN `description` varchar(512) COLLATE utf8mb4_unicode_ci DEFAULT NULL AFTER `name`;
