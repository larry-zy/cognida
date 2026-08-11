-- 存量回填：把历史文档的启用状态从 'disabled' 修正为 'enabled'〔DA-1〕。
-- 背景：此前 NewKnowledge / 上传创建流程把 enable_status 误设为默认 'disabled'，导致所有历史
-- 文档虽仍被检索命中、但状态显示为停用。启用/停用闭环上线后，检索侧会按 MySQL 权威后过滤剔除
-- 停用文档；若不回填，这批本应可检索的历史文档将被误判为停用而整体从检索结果消失。
-- 仅回填未删除、当前为 disabled 的行；已被用户显式停用的语义在存量库中无法区分，按「历史默认皆为误停用」处理。
UPDATE `knowledge`
  SET `enable_status` = 'enabled'
  WHERE `enable_status` = 'disabled' AND `deleted_at` IS NULL;
