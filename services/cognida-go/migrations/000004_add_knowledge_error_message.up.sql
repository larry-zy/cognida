ALTER TABLE `knowledge`
  ADD COLUMN `error_message` text COLLATE utf8mb4_unicode_ci NULL AFTER `parse_status`;
