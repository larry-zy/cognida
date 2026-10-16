CREATE TABLE quality_feedback_cases (
 id VARCHAR(36) NOT NULL PRIMARY KEY,
 tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
 version INT NOT NULL, status VARCHAR(32) NOT NULL,
 idempotency_key VARCHAR(100) NOT NULL, request_digest CHAR(64) NOT NULL,
 baseline_knowledge_id VARCHAR(36) NOT NULL,
 active_version_id VARCHAR(36) NOT NULL DEFAULT '',
 body LONGTEXT NOT NULL,
 created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
 UNIQUE KEY uq_feedback_key(tenant_id,user_id,idempotency_key),
 UNIQUE KEY uq_feedback_baseline(tenant_id,baseline_knowledge_id),
 KEY ix_feedback_owner(tenant_id,user_id,id)
);
CREATE TABLE quality_feedback_events (
 id VARCHAR(36) NOT NULL PRIMARY KEY, case_id VARCHAR(36) NOT NULL,
 publication_id VARCHAR(36) NOT NULL DEFAULT '', actor_id BIGINT NOT NULL,
 action VARCHAR(64) NOT NULL, detail VARCHAR(2000) NOT NULL,
 created_at DATETIME(6) NOT NULL,
 KEY ix_feedback_event(case_id,created_at,id)
);
