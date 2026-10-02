ALTER TABLE `collection_tasks`
  ADD COLUMN `schedule_enabled` tinyint(1) NOT NULL DEFAULT 0 AFTER `max_response_bytes`,
  ADD COLUMN `schedule_interval_seconds` bigint NOT NULL DEFAULT 0 AFTER `schedule_enabled`,
  ADD COLUMN `next_run_at` datetime(3) NULL AFTER `schedule_interval_seconds`,
  ADD COLUMN `last_scheduled_at` datetime(3) NULL AFTER `next_run_at`,
  ADD KEY `idx_collection_task_schedule_due` (`schedule_enabled`, `next_run_at`);
