CREATE TABLE agent_preferences (
 tenant_id BIGINT NOT NULL,
 user_id BIGINT NOT NULL,
 version BIGINT NOT NULL,
 config_json JSON NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 PRIMARY KEY (tenant_id,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE agent_preference_updates (
 tenant_id BIGINT NOT NULL,
 user_id BIGINT NOT NULL,
 operation_key CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 result_json JSON NOT NULL,
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY (tenant_id,user_id,operation_key),
 CONSTRAINT fk_agent_preferences_update FOREIGN KEY(tenant_id,user_id)
 REFERENCES agent_preferences(tenant_id,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
