CREATE TABLE lifecycle_definitions (
 id varchar(64) NOT NULL, version int NOT NULL, body json NOT NULL, created_at datetime(3) NOT NULL,
 PRIMARY KEY(id,version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE lifecycle_assets (
 asset_id varchar(36) NOT NULL, version int NOT NULL, tenant_id bigint NOT NULL,
 object_key varchar(512) NOT NULL, body json NOT NULL, created_at datetime(3) NOT NULL,
 PRIMARY KEY(asset_id,version), KEY idx_lifecycle_asset_tenant(tenant_id,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE lifecycle_runs (
 id varchar(36) NOT NULL, tenant_id bigint NOT NULL, status varchar(32) NOT NULL,
 idempotency_key varchar(128) NOT NULL, body json NOT NULL, created_at datetime(3) NOT NULL, updated_at datetime(3) NOT NULL,
 PRIMARY KEY(id), UNIQUE KEY uk_lifecycle_run_key(tenant_id,idempotency_key), KEY idx_lifecycle_run_tenant(tenant_id,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE lifecycle_stages (
 id varchar(36) NOT NULL, run_id varchar(36) NOT NULL DEFAULT '', tenant_id bigint NOT NULL,
 status varchar(32) NOT NULL, idempotency_key varchar(128) NOT NULL, body json NOT NULL,
 created_at datetime(3) NOT NULL, updated_at datetime(3) NOT NULL,
 PRIMARY KEY(id), UNIQUE KEY uk_lifecycle_stage_key(tenant_id,idempotency_key),
 KEY idx_lifecycle_stage_run(tenant_id,run_id), KEY idx_lifecycle_stage_pending(status,updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE lifecycle_approvals (
 id varchar(36) NOT NULL, tenant_id bigint NOT NULL, run_id varchar(36) NOT NULL DEFAULT '', stage_id varchar(36) NOT NULL,
 status varchar(32) NOT NULL, body json NOT NULL, expires_at datetime(3) NOT NULL, created_at datetime(3) NOT NULL,
 PRIMARY KEY(id), UNIQUE KEY uk_lifecycle_approval_stage(stage_id), KEY idx_lifecycle_approval_run(tenant_id,run_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE lifecycle_events (
 id bigint NOT NULL AUTO_INCREMENT, tenant_id bigint NOT NULL, run_id varchar(36) NOT NULL DEFAULT '', stage_id varchar(36) NOT NULL DEFAULT '',
 type varchar(64) NOT NULL, request_id varchar(128) NOT NULL DEFAULT '', detail text NOT NULL, created_at datetime(3) NOT NULL,
 PRIMARY KEY(id), KEY idx_lifecycle_event_run(tenant_id,run_id,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
