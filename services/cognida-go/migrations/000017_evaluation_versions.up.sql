CREATE TABLE evaluation_dataset_heads (
 tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL, dataset_id VARCHAR(100) NOT NULL,
 current_version INT NOT NULL DEFAULT 0,
 PRIMARY KEY(tenant_id,user_id,dataset_id)
);
CREATE TABLE evaluation_dataset_versions (
 id VARCHAR(36) NOT NULL PRIMARY KEY, tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
 dataset_id VARCHAR(100) NOT NULL, version INT NOT NULL,
 idempotency_key VARCHAR(100) NOT NULL, request_digest CHAR(64) NOT NULL,
 manifest_digest CHAR(64) NOT NULL, body LONGTEXT NOT NULL, created_at DATETIME(6) NOT NULL,
 UNIQUE KEY uq_dataset_version(tenant_id,user_id,dataset_id,version),
 UNIQUE KEY uq_dataset_version_key(tenant_id,user_id,idempotency_key),
 CONSTRAINT fk_dataset_version_head FOREIGN KEY(tenant_id,user_id,dataset_id) REFERENCES evaluation_dataset_heads(tenant_id,user_id,dataset_id)
);
