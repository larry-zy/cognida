CREATE TABLE huggingface_imports (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 tenant_id BIGINT NOT NULL,
 user_id BIGINT NOT NULL,
 operation_key CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 status VARCHAR(24) NOT NULL,
 deadline_us BIGINT NOT NULL,
 body JSON NOT NULL,
 PRIMARY KEY (id),
 UNIQUE KEY uq_hf_import_key (tenant_id,user_id,operation_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
