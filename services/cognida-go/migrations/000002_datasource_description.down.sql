-- 回滚：移除外部数据源业务描述字段。
ALTER TABLE `data_sources`
  DROP COLUMN `description`;
