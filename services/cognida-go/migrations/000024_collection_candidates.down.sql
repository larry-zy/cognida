ALTER TABLE collection_items DROP COLUMN asset_ref_id;
ALTER TABLE collection_runs DROP COLUMN output_refs, DROP COLUMN candidate_only, DROP COLUMN governed_execution;
ALTER TABLE collection_tasks DROP COLUMN mode;
