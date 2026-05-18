-- Give each patent record an open-ended set of life-stage documents, so the
-- application, the pre-grant publication, and the grant of one invention are
-- one record. patent.number stays the record's permanent key.

ALTER TABLE patent ADD COLUMN display_number TEXT NOT NULL DEFAULT '';

CREATE TABLE document (
    number        TEXT PRIMARY KEY,
    record_number TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
    country       TEXT NOT NULL,
    serial        TEXT NOT NULL,
    kind          TEXT NOT NULL,
    stage         TEXT NOT NULL,   -- application | publication | grant
    dated         TEXT NOT NULL    -- RFC3339, empty when unknown
);

CREATE INDEX idx_document_record ON document (record_number, stage);

-- Backfill: one document per existing patent, its stage guessed from the kind
-- code (a 'B' kind is a grant, anything else a publication).
INSERT INTO document (number, record_number, country, serial, kind, stage, dated)
SELECT number, number, country, serial, kind,
       CASE WHEN kind LIKE 'B%' THEN 'grant' ELSE 'publication' END,
       CASE WHEN grant_date <> '' THEN grant_date
            WHEN publication_date <> '' THEN publication_date
            ELSE application_date END
FROM patent;

UPDATE patent SET display_number = number;
