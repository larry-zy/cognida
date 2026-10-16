CREATE TABLE api_source_credentials (
 id VARCHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 tenant_id BIGINT NOT NULL,user_id BIGINT NOT NULL,name VARCHAR(128) NOT NULL,
 origin VARCHAR(512) NOT NULL,kind VARCHAR(16) NOT NULL,ciphertext TEXT NOT NULL,
 enabled BOOLEAN NOT NULL DEFAULT TRUE,created_at DATETIME(6) NOT NULL,
 KEY idx_api_credentials_scope(tenant_id,user_id,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE api_sources (
 id VARCHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 tenant_id BIGINT NOT NULL,user_id BIGINT NOT NULL,name VARCHAR(128) NOT NULL,
 version INT NOT NULL,enabled BOOLEAN NOT NULL DEFAULT TRUE,config JSON NOT NULL,
 watermark VARCHAR(64) NOT NULL DEFAULT '',active_run VARCHAR(36) NOT NULL DEFAULT '',
 created_at DATETIME(6) NOT NULL,updated_at DATETIME(6) NOT NULL,
 KEY idx_api_sources_scope(tenant_id,user_id,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE api_source_versions (
 source_id VARCHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,version INT NOT NULL,
 tenant_id BIGINT NOT NULL,user_id BIGINT NOT NULL,body JSON NOT NULL,
 PRIMARY KEY(source_id,version),KEY idx_api_versions_scope(tenant_id,user_id,source_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE api_ingestion_runs (
 id VARCHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 tenant_id BIGINT NOT NULL,user_id BIGINT NOT NULL,source_id VARCHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 operation_key CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 status VARCHAR(24) NOT NULL,version BIGINT NOT NULL,epoch BIGINT NOT NULL,
 lease_until_us BIGINT NOT NULL DEFAULT 0,body JSON NOT NULL,
 created_at DATETIME(6) NOT NULL,updated_at DATETIME(6) NOT NULL,
 UNIQUE KEY uq_api_run_operation(tenant_id,user_id,operation_key),
 KEY idx_api_run_recovery(status,lease_until_us),KEY idx_api_run_scope(tenant_id,user_id,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
