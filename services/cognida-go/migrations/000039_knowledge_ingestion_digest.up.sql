-- Historical proof of the exact original indexed chunks. Legacy rows deliberately
-- remain empty: their current chunks cannot establish an ingestion-time snapshot.
ALTER TABLE knowledge ADD COLUMN ingested_chunk_digest VARCHAR(64) NOT NULL DEFAULT '';
