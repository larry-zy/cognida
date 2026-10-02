-- Memberships are retained: existing tenant_users rows and newly granted access
-- cannot safely be distinguished during rollback.
ALTER TABLE refresh_tokens DROP COLUMN tenant_id;
