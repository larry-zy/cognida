ALTER TABLE `collection_tasks`
  DROP INDEX `idx_collection_task_schedule_due`,
  DROP COLUMN `last_scheduled_at`,
  DROP COLUMN `next_run_at`,
  DROP COLUMN `schedule_interval_seconds`,
  DROP COLUMN `schedule_enabled`;
