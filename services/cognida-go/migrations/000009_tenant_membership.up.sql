ALTER TABLE refresh_tokens ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 0;
UPDATE refresh_tokens r JOIN users u ON u.id = r.user_id SET r.tenant_id = u.tenant_id;
INSERT IGNORE INTO tenant_users (tenant_id,user_id,role,status,created_at)
SELECT u.tenant_id,u.id,IF(u.id = first_user.id,'owner','member'),'active',NOW(3)
FROM users u JOIN (SELECT tenant_id,MIN(id) AS id FROM users WHERE deleted_at IS NULL GROUP BY tenant_id) first_user ON first_user.tenant_id = u.tenant_id
JOIN tenants t ON t.id = u.tenant_id
WHERE u.deleted_at IS NULL AND t.deleted_at IS NULL;
