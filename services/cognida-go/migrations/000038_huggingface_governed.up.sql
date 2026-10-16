ALTER TABLE huggingface_imports
 ADD COLUMN governed_key CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD COLUMN governed_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD COLUMN claim_token CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD UNIQUE KEY uq_hf_governed_key (tenant_id,user_id,governed_key),
 ADD KEY idx_hf_governed_pending (status,deadline_us,governed_key);

-- Restore immutable bindings if a previous down retained historical receipts.
UPDATE huggingface_imports
SET governed_key=JSON_UNQUOTE(JSON_EXTRACT(body,'$.governed.operation_key')),
    governed_hash=JSON_UNQUOTE(JSON_EXTRACT(body,'$.governed.payload_hash'))
WHERE JSON_TYPE(JSON_EXTRACT(body,'$.governed'))='OBJECT';
