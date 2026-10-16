CREATE TABLE feature_jobs (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
 feature_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, feature_version INT NOT NULL,
 idempotency_key VARCHAR(100) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_digest CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 status VARCHAR(32) NOT NULL, version INT NOT NULL DEFAULT 1,
 body LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
 lease_token CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '', lease_until DATETIME(6) NULL,
 created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
 UNIQUE KEY uk_feature_job_key(tenant_id,user_id,idempotency_key),
 KEY idx_feature_job_pending(status,lease_until),
 CONSTRAINT chk_feature_job_body CHECK(JSON_VALID(body))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE feature_watermarks (
 tenant_id BIGINT NOT NULL,user_id BIGINT NOT NULL,feature_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,feature_version INT NOT NULL,
 event_time DATETIME(6) NULL, job_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
 PRIMARY KEY(tenant_id,user_id,feature_id,feature_version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
