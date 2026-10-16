ALTER TABLE collection_tasks ADD COLUMN mode varchar(32) NOT NULL DEFAULT 'knowledge';
ALTER TABLE collection_runs
 ADD COLUMN candidate_only BOOLEAN NOT NULL DEFAULT FALSE,
 ADD COLUMN governed_execution JSON NULL,
 ADD COLUMN output_refs JSON NULL;
ALTER TABLE collection_items ADD COLUMN asset_ref_id varchar(128) NULL;
