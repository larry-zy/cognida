-- Existing values acquire a conservative migration-time availability. Their
-- historical event timestamps alone cannot prove prior system availability.
ALTER TABLE feature_values
 ADD COLUMN available_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 ADD COLUMN value_exact LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL,
 MODIFY entity_id VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
 ADD CONSTRAINT chk_feature_value_exact CHECK(value_exact IS NULL OR JSON_VALID(value_exact));
UPDATE feature_values SET value_exact=CAST(value AS CHAR CHARACTER SET utf8mb4);
CREATE TABLE feature_materializations (
 id VARCHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 tenant_id BIGINT NOT NULL,
 user_id BIGINT NOT NULL,
 idempotency_key VARCHAR(100) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_digest CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 payload JSON NOT NULL,
 available_at DATETIME(6) NOT NULL,
 PRIMARY KEY(id),
 UNIQUE KEY uk_feature_materialization_key(tenant_id,user_id,idempotency_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
