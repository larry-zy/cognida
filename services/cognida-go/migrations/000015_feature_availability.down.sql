DROP TABLE feature_materializations;
-- Exact values remain in the legacy JSON projection with its original numeric
-- limits when rolling back. Export exact values before a production downgrade.
ALTER TABLE feature_values DROP CHECK chk_feature_value_exact, DROP COLUMN value_exact, DROP COLUMN available_at,
 MODIFY entity_id VARCHAR(128) CHARACTER SET utf8mb4 NOT NULL;
