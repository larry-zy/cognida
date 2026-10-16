CREATE TABLE knowledge_label_applications (
 id VARCHAR(36) NOT NULL PRIMARY KEY,
 tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
 knowledge_id VARCHAR(36) NOT NULL, result_id VARCHAR(36) NOT NULL,
 review_version INT NOT NULL, source_ref_id VARCHAR(100) NOT NULL,
 content_digest CHAR(64) NOT NULL, scheme_digest CHAR(64) NOT NULL,
 idempotency_key VARCHAR(100) NOT NULL, request_digest CHAR(64) NOT NULL,
 labels_json JSON NOT NULL, status VARCHAR(20) NOT NULL,
 created_at DATETIME(6) NOT NULL,
 UNIQUE KEY uq_knowledge_label_key(tenant_id,user_id,idempotency_key),
 UNIQUE KEY uq_knowledge_label_revision(tenant_id,knowledge_id,result_id,review_version)
);
CREATE TABLE knowledge_applied_labels (
 tenant_id BIGINT NOT NULL, knowledge_id VARCHAR(36) NOT NULL,
 label VARCHAR(128) COLLATE utf8mb4_bin NOT NULL,
 application_id VARCHAR(36) NOT NULL, content_digest CHAR(64) NOT NULL,
 PRIMARY KEY(tenant_id,knowledge_id,label),
 KEY idx_knowledge_label_filter(tenant_id,label,knowledge_id),
 CONSTRAINT fk_knowledge_label_receipt FOREIGN KEY(application_id) REFERENCES knowledge_label_applications(id)
);
