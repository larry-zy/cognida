-- Explicit user-requested plan revisions. Existing child identities and receipts
-- retain their original immutable plan version across a running replan.
CREATE TABLE task_control_replans (
 tenant_id BIGINT NOT NULL,
 user_id BIGINT NOT NULL,
 task_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 operation_key CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 from_version BIGINT NOT NULL,
 plan_version BIGINT NOT NULL,
 reason VARCHAR(2000) NOT NULL,
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY (tenant_id,user_id,task_id,operation_key),
 UNIQUE KEY uq_tc_replan_version(tenant_id,user_id,task_id,plan_version),
 CONSTRAINT fk_tc_replan_plan FOREIGN KEY(tenant_id,user_id,task_id,plan_version)
 REFERENCES task_control_plans(tenant_id,user_id,task_id,version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
