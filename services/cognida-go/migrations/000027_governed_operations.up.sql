-- Internal, tenant/user-owned derived objects only. This migration does not
-- grant write access to any external datasource or original business table.
CREATE TABLE governed_operation_targets (
 id VARCHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 tenant_id BIGINT NOT NULL,user_id BIGINT NOT NULL,
 name VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 version BIGINT NOT NULL DEFAULT 0,active_version_id VARCHAR(36) NOT NULL DEFAULT '',
 created_at DATETIME(6) NOT NULL,updated_at DATETIME(6) NOT NULL,
 UNIQUE KEY uq_operation_target(tenant_id,user_id,name),KEY idx_operation_target_scope(tenant_id,user_id,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE governed_operation_jobs (
 id VARCHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 tenant_id BIGINT NOT NULL,user_id BIGINT NOT NULL,target_id VARCHAR(36) NOT NULL,
 operation_key CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 payload_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 kind VARCHAR(16) NOT NULL,status VARCHAR(32) NOT NULL,
 version BIGINT NOT NULL,epoch BIGINT NOT NULL DEFAULT 0,lease_until_us BIGINT NOT NULL DEFAULT 0,
 deadline_us BIGINT NOT NULL,body JSON NOT NULL,
 created_at DATETIME(6) NOT NULL,updated_at DATETIME(6) NOT NULL,
 UNIQUE KEY uq_operation_job_key(tenant_id,user_id,operation_key),
 KEY idx_operation_job_recovery(status,lease_until_us),KEY idx_operation_job_scope(tenant_id,user_id,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE governed_operation_versions (
 id VARCHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 tenant_id BIGINT NOT NULL,user_id BIGINT NOT NULL,target_id VARCHAR(36) NOT NULL,
 version BIGINT NOT NULL,operation_id VARCHAR(36) NOT NULL,
 physical_table VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 row_count BIGINT NOT NULL,content_digest CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 body JSON NOT NULL,created_at DATETIME(6) NOT NULL,
 UNIQUE KEY uq_operation_version(target_id,version),UNIQUE KEY uq_operation_commit(operation_id),
 KEY idx_operation_versions_scope(tenant_id,user_id,target_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
