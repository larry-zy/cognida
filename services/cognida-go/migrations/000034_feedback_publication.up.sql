CREATE TABLE quality_feedback_publications (
 id VARCHAR(36) NOT NULL PRIMARY KEY, case_id VARCHAR(36) NOT NULL,
 tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
 version INT NOT NULL, status VARCHAR(32) NOT NULL,
 knowledge_id VARCHAR(36) NOT NULL,
 idempotency_key VARCHAR(100) NOT NULL, request_digest CHAR(64) NOT NULL,
 body LONGTEXT NOT NULL,
 created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
 UNIQUE KEY uq_feedback_publication_key(case_id,idempotency_key),
 UNIQUE KEY uq_feedback_publication_knowledge(knowledge_id),
 KEY ix_feedback_publication_case(tenant_id,user_id,case_id,id)
);
