-- Stop workers before rollback. Keep historical receipts and committed assets;
-- an interrupted governed run is closed rather than converted to a sync import.
UPDATE huggingface_imports
SET status='failed', body=JSON_SET(body,'$.status','failed','$.error_code','migration_rollback','$.output_refs',JSON_ARRAY())
WHERE governed_key IS NOT NULL AND status IN ('queued','running','cancel_requested');
ALTER TABLE huggingface_imports
 DROP INDEX idx_hf_governed_pending,
 DROP INDEX uq_hf_governed_key,
 DROP COLUMN claim_token,
 DROP COLUMN governed_hash,
 DROP COLUMN governed_key;
