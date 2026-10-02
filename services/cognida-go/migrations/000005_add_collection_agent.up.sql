CREATE TABLE `collection_tasks` (
  `id` varchar(64) NOT NULL,
  `tenant_id` bigint NOT NULL,
  `user_id` bigint NOT NULL,
  `knowledge_base_id` varchar(64) NOT NULL,
  `idempotency_key` varchar(128) NOT NULL DEFAULT '',
  `status` varchar(16) NOT NULL,
  `max_sources` int NOT NULL,
  `max_response_bytes` bigint NOT NULL,
  `error_message` text NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_collection_task_tenant_idempotency` (`tenant_id`, `idempotency_key`),
  KEY `idx_collection_task_tenant_created` (`tenant_id`, `created_at`),
  KEY `idx_collection_task_tenant_status` (`tenant_id`, `status`),
  KEY `idx_collection_task_kb` (`knowledge_base_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `collection_sources` (
  `id` varchar(64) NOT NULL,
  `task_id` varchar(64) NOT NULL,
  `url` text NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_collection_source_task` (`task_id`),
  CONSTRAINT `fk_collection_source_task` FOREIGN KEY (`task_id`) REFERENCES `collection_tasks` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `collection_runs` (
  `id` varchar(64) NOT NULL,
  `task_id` varchar(64) NOT NULL,
  `tenant_id` bigint NOT NULL,
  `status` varchar(16) NOT NULL,
  `error_message` text NULL,
  `started_at` datetime(3) NOT NULL,
  `completed_at` datetime(3) NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_collection_run_tenant_task` (`tenant_id`, `task_id`),
  KEY `idx_collection_run_tenant_status` (`tenant_id`, `status`),
  CONSTRAINT `fk_collection_run_task` FOREIGN KEY (`task_id`) REFERENCES `collection_tasks` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `collection_items` (
  `id` varchar(64) NOT NULL,
  `tenant_id` bigint NOT NULL,
  `task_id` varchar(64) NOT NULL,
  `run_id` varchar(64) NOT NULL,
  `source_id` varchar(64) NOT NULL,
  `source_url` text NOT NULL,
  `canonical_url` text NULL,
  `status` varchar(16) NOT NULL,
  `content_hash` char(64) NULL,
  `knowledge_id` varchar(64) NULL,
  `http_status` int NULL,
  `content_type` varchar(255) NULL,
  `error_reason` text NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_collection_item_run_source` (`run_id`, `source_id`),
  KEY `idx_collection_item_tenant_run` (`tenant_id`, `run_id`),
  KEY `idx_collection_item_task` (`task_id`),
  KEY `idx_collection_item_content_hash` (`content_hash`),
  CONSTRAINT `fk_collection_item_task` FOREIGN KEY (`task_id`) REFERENCES `collection_tasks` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_collection_item_run` FOREIGN KEY (`run_id`) REFERENCES `collection_runs` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_collection_item_source` FOREIGN KEY (`source_id`) REFERENCES `collection_sources` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
