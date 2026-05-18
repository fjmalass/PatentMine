-- Patent detail fields: claim-1 text, estimated expiration, and the provider
-- page the record was fetched from. Defaults keep existing rows valid.

ALTER TABLE patent ADD COLUMN first_claim       TEXT NOT NULL DEFAULT '';
ALTER TABLE patent ADD COLUMN expiration_date   TEXT NOT NULL DEFAULT '';
ALTER TABLE patent ADD COLUMN expiration_source TEXT NOT NULL DEFAULT '';
ALTER TABLE patent ADD COLUMN source_url        TEXT NOT NULL DEFAULT '';
