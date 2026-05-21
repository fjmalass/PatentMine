CREATE TABLE IF NOT EXISTS patent (
    number            TEXT PRIMARY KEY,
    country           TEXT NOT NULL,
    serial            TEXT NOT NULL,
    kind              TEXT NOT NULL,
    title             TEXT NOT NULL,
    abstract          TEXT NOT NULL,
    assignee          TEXT NOT NULL,
    inventors         TEXT NOT NULL,
    fetch_state       TEXT NOT NULL,
    source            TEXT NOT NULL,
    application_date  TEXT NOT NULL,
    publication_date  TEXT NOT NULL,
    grant_date        TEXT NOT NULL,
    fetched_at        TEXT NOT NULL,
    display_number    TEXT NOT NULL DEFAULT '',
    first_claim       TEXT NOT NULL DEFAULT '',
    expiration_date   TEXT NOT NULL DEFAULT '',
    expiration_source TEXT NOT NULL DEFAULT '',
    source_url        TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS relation (
    from_number TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
    to_number   TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    PRIMARY KEY (from_number, to_number, kind)
);

CREATE INDEX IF NOT EXISTS idx_relation_from ON relation (from_number, kind);
CREATE INDEX IF NOT EXISTS idx_relation_to   ON relation (to_number, kind);

CREATE TABLE IF NOT EXISTS project (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL
);

-- membership links a patent to a project. It also carries that pair's curated
-- IDS data inline (the ids_* columns): an IDS entry exists for the membership
-- exactly when ids_status is non-empty.
CREATE TABLE IF NOT EXISTS membership (
    project_id            TEXT NOT NULL REFERENCES project (id) ON DELETE CASCADE,
    patent_number         TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
    state                 TEXT NOT NULL,
    added_at              TEXT NOT NULL,
    ids_kind_code         TEXT NOT NULL DEFAULT '',
    ids_country_code      TEXT NOT NULL DEFAULT '',
    ids_in_full           INTEGER NOT NULL DEFAULT 0,
    ids_relevant_passages TEXT NOT NULL DEFAULT '',
    ids_notes             TEXT NOT NULL DEFAULT '',
    ids_status            TEXT NOT NULL DEFAULT '',
    ids_added_at          TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (project_id, patent_number)
);

CREATE INDEX IF NOT EXISTS idx_membership_project ON membership (project_id, state);

CREATE TABLE IF NOT EXISTS document (
    number        TEXT PRIMARY KEY,
    record_number TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
    country       TEXT NOT NULL,
    serial        TEXT NOT NULL,
    kind          TEXT NOT NULL,
    stage         TEXT NOT NULL,
    dated         TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_document_record ON document (record_number, stage);

CREATE TABLE IF NOT EXISTS tag (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL REFERENCES project (id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (project_id, name)
);

CREATE TABLE IF NOT EXISTS patent_tag (
    tag_id        INTEGER NOT NULL REFERENCES tag (id) ON DELETE CASCADE,
    patent_number TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
    created_at    TEXT NOT NULL,
    PRIMARY KEY (tag_id, patent_number)
);

CREATE INDEX IF NOT EXISTS idx_patent_tag_patent ON patent_tag (patent_number);

-- patent_fts is an FTS5 full-text index over a patent's title and abstract.
-- It is an external-content table: the text lives in the patent table and the
-- triggers below keep the index in step with every insert, update, and delete.
-- Listing search uses it instead of a LIKE scan over title/abstract.
CREATE VIRTUAL TABLE IF NOT EXISTS patent_fts USING fts5 (
    title,
    abstract,
    content='patent',
    content_rowid='rowid'
);

CREATE TRIGGER IF NOT EXISTS patent_fts_insert AFTER INSERT ON patent BEGIN
    INSERT INTO patent_fts (rowid, title, abstract)
    VALUES (new.rowid, new.title, new.abstract);
END;

CREATE TRIGGER IF NOT EXISTS patent_fts_delete AFTER DELETE ON patent BEGIN
    INSERT INTO patent_fts (patent_fts, rowid, title, abstract)
    VALUES ('delete', old.rowid, old.title, old.abstract);
END;

CREATE TRIGGER IF NOT EXISTS patent_fts_update AFTER UPDATE ON patent BEGIN
    INSERT INTO patent_fts (patent_fts, rowid, title, abstract)
    VALUES ('delete', old.rowid, old.title, old.abstract);
    INSERT INTO patent_fts (rowid, title, abstract)
    VALUES (new.rowid, new.title, new.abstract);
END;
