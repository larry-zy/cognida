CREATE TABLE feature_job_values (
 tenant_id BIGINT NOT NULL,user_id BIGINT NOT NULL,job_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,partition_index INT NOT NULL,
 feature_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,version INT NOT NULL,
 entity_id VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
 observed_at DATETIME(6) NOT NULL,available_at DATETIME(6) NOT NULL,withdrawn_at DATETIME(6) NULL,
 value_exact LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,source_ref_id VARCHAR(128) NOT NULL,
 PRIMARY KEY(job_id,entity_id,observed_at),
 KEY idx_feature_job_lookup(tenant_id,user_id,feature_id,version,entity_id,observed_at,available_at),
 CONSTRAINT chk_feature_job_value CHECK(JSON_VALID(value_exact))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
