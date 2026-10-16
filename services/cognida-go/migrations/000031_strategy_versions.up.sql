CREATE TABLE strategy_heads (
 tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL, slot VARCHAR(100) NOT NULL,
 kind VARCHAR(32) NOT NULL, revision BIGINT NOT NULL DEFAULT 0, next_version INT NOT NULL DEFAULT 1,
 baseline_id CHAR(36) NOT NULL DEFAULT '', active_id CHAR(36) NOT NULL DEFAULT '',
 PRIMARY KEY (tenant_id,user_id,slot)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE strategy_versions (
 id CHAR(36) NOT NULL, tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL, slot VARCHAR(100) NOT NULL,
 version_number INT NOT NULL, body LONGTEXT NOT NULL, disabled_at DATETIME(3) NULL, created_at DATETIME(3) NOT NULL,
 PRIMARY KEY(id), UNIQUE KEY uq_strategy_version(tenant_id,user_id,slot,version_number),
 KEY ix_strategy_owner(tenant_id,user_id,id),
 CONSTRAINT fk_strategy_version_head FOREIGN KEY(tenant_id,user_id,slot) REFERENCES strategy_heads(tenant_id,user_id,slot)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE strategy_events (
 id CHAR(36) NOT NULL, tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL, slot VARCHAR(100) NOT NULL,
 kind VARCHAR(32) NOT NULL, operation_key VARCHAR(120) NOT NULL, request_hash CHAR(64) NOT NULL,
 version_id CHAR(36) NOT NULL, payload LONGTEXT NOT NULL, created_at DATETIME(3) NOT NULL,
 PRIMARY KEY(id), UNIQUE KEY uq_strategy_event(tenant_id,user_id,slot,kind,operation_key),
 KEY ix_strategy_event_page(tenant_id,user_id,slot,kind,id),
 CONSTRAINT fk_strategy_event_head FOREIGN KEY(tenant_id,user_id,slot) REFERENCES strategy_heads(tenant_id,user_id,slot),
 CONSTRAINT fk_strategy_event_version FOREIGN KEY(version_id) REFERENCES strategy_versions(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
