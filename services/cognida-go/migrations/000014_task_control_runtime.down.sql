DROP TABLE task_control_leases;
ALTER TABLE task_control_children
  DROP COLUMN cost_micros_reserved, DROP COLUMN calls_reserved,
  DROP COLUMN observed_at, DROP COLUMN error, DROP COLUMN output_refs, DROP COLUMN domain_status;
ALTER TABLE task_control_tasks
  DROP COLUMN finished_at, DROP COLUMN deadline_at, DROP COLUMN started_at,
  DROP COLUMN execution_hash, DROP COLUMN execution_key, DROP COLUMN execution_plan_version;
