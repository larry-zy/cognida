CREATE TABLE query_jobs (
 id CHAR(36) NOT NULL PRIMARY KEY,
 tenant_id BIGINT NOT NULL,
 user_id BIGINT NOT NULL,
 operation_key VARCHAR(100) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash CHAR(64) CHARACTER SET ascii NOT NULL,
 status VARCHAR(32) NOT NULL,
 terminal BOOLEAN NOT NULL DEFAULT FALSE,
 version BIGINT NOT NULL DEFAULT 1,
 epoch BIGINT NOT NULL DEFAULT 0,
 lease_until DATETIME(6) NULL,
 body JSON NOT NULL,
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 UNIQUE KEY uq_query_job_key (tenant_id,user_id,operation_key),
 KEY idx_query_job_owner (tenant_id,user_id,id),
 KEY idx_query_job_worker (terminal,lease_until,updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
