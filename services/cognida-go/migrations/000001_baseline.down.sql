-- 000001_baseline (down) — 回滚基线：删除全部业务表。
-- 无外键约束，删除顺序无关；FOREIGN_KEY_CHECKS 归零仅作防御。

SET FOREIGN_KEY_CHECKS = 0;
DROP TABLE IF EXISTS `agent_experiences`;
DROP TABLE IF EXISTS `agent_operation_audit`;
DROP TABLE IF EXISTS `agent_semantic_coverage_logs`;
DROP TABLE IF EXISTS `agent_semantic_dimensions`;
DROP TABLE IF EXISTS `agent_semantic_logical_tables`;
DROP TABLE IF EXISTS `agent_semantic_measures`;
DROP TABLE IF EXISTS `agent_semantic_metrics`;
DROP TABLE IF EXISTS `agent_semantic_models`;
DROP TABLE IF EXISTS `agent_semantic_relations`;
DROP TABLE IF EXISTS `agents`;
DROP TABLE IF EXISTS `audit_logs`;
DROP TABLE IF EXISTS `chunks`;
DROP TABLE IF EXISTS `column_profiles`;
DROP TABLE IF EXISTS `data_sources`;
DROP TABLE IF EXISTS `evaluation_dataset_records`;
DROP TABLE IF EXISTS `evaluation_datasets`;
DROP TABLE IF EXISTS `evaluation_qa_results`;
DROP TABLE IF EXISTS `evaluation_tasks`;
DROP TABLE IF EXISTS `knowledge`;
DROP TABLE IF EXISTS `knowledge_base_settings`;
DROP TABLE IF EXISTS `knowledge_bases`;
DROP TABLE IF EXISTS `knowledge_tags`;
DROP TABLE IF EXISTS `llm_models`;
DROP TABLE IF EXISTS `messages`;
DROP TABLE IF EXISTS `quality_check_records`;
DROP TABLE IF EXISTS `refresh_tokens`;
DROP TABLE IF EXISTS `retrieval_settings`;
DROP TABLE IF EXISTS `sessions`;
DROP TABLE IF EXISTS `tenant_users`;
DROP TABLE IF EXISTS `tenants`;
DROP TABLE IF EXISTS `trace_spans`;
DROP TABLE IF EXISTS `users`;
SET FOREIGN_KEY_CHECKS = 1;
