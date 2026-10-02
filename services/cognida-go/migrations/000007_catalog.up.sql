CREATE TABLE asset_table (
 id VARCHAR(36) PRIMARY KEY, tenant_id BIGINT NOT NULL,
 datasource_id VARCHAR(64) NOT NULL, schema_name VARCHAR(128) NOT NULL,
 table_name VARCHAR(128) NOT NULL, comment TEXT NOT NULL,
 tags JSON NOT NULL, deprecated BOOLEAN NOT NULL DEFAULT FALSE, payload JSON NOT NULL,
 last_synced_at DATETIME(6) NOT NULL,
 UNIQUE KEY uk_catalog_coordinate (tenant_id,datasource_id,schema_name,table_name),
 KEY idx_catalog_page (tenant_id,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE asset_column (
 id VARCHAR(36) PRIMARY KEY, tenant_id BIGINT NOT NULL, table_id VARCHAR(36) NOT NULL,
 column_name VARCHAR(128) NOT NULL, deprecated BOOLEAN NOT NULL DEFAULT FALSE,
 payload JSON NOT NULL,
 UNIQUE KEY uk_catalog_column (tenant_id,table_id,column_name),
 KEY idx_catalog_column_table (tenant_id,table_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE business_glossary (
 id VARCHAR(36) PRIMARY KEY, tenant_id BIGINT NOT NULL, term VARCHAR(128) NOT NULL,
 description TEXT NOT NULL, synonyms JSON NOT NULL, asset_ids JSON NOT NULL,
 UNIQUE KEY uk_glossary_term (tenant_id,term), KEY idx_glossary_page (tenant_id,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
