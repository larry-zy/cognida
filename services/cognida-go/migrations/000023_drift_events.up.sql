CREATE TABLE drift_events (
 id VARCHAR(36) NOT NULL PRIMARY KEY,
 record_id VARCHAR(36) NOT NULL, tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
 action VARCHAR(64) NOT NULL, actor_id BIGINT NOT NULL, version INT NOT NULL,
 created_at DATETIME(6) NOT NULL,
 UNIQUE KEY uk_drift_event (record_id,version),
 KEY idx_drift_event_scope (tenant_id,user_id,record_id,version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
