CREATE TABLE governance_records (
 id VARCHAR(36) PRIMARY KEY, tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
 kind VARCHAR(40) NOT NULL, parent_id VARCHAR(64) NOT NULL DEFAULT '', status VARCHAR(32) NOT NULL,
 version INT NOT NULL DEFAULT 1, idempotency_key VARCHAR(128) NOT NULL,
 payload JSON NOT NULL, due_at DATETIME(6) NULL,
 created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
 UNIQUE KEY uk_governance_idempotency(tenant_id,kind,idempotency_key),
 KEY idx_governance_page(tenant_id,kind,id), KEY idx_governance_parent(tenant_id,kind,parent_id,id),
 KEY idx_governance_due(kind,status,due_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE feature_values (
 tenant_id BIGINT NOT NULL, feature_id VARCHAR(36) NOT NULL, version INT NOT NULL,
 entity_id VARCHAR(128) NOT NULL, observed_at DATETIME(6) NOT NULL, value JSON NOT NULL,
 source_ref_id VARCHAR(64) NOT NULL,
 PRIMARY KEY(tenant_id,feature_id,version,entity_id,observed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
