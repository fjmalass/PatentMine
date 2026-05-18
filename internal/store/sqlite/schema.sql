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
    from_number TEXT NOT NULL,
    to_number   TEXT NOT NULL,
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

CREATE TABLE IF NOT EXISTS membership (
    project_id    TEXT NOT NULL REFERENCES project (id),
    patent_number TEXT NOT NULL REFERENCES patent (number),
    state         TEXT NOT NULL,
    added_at      TEXT NOT NULL,
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
    PRIMARY KEY (tag_id, patent_number)
);

CREATE INDEX IF NOT EXISTS idx_patent_tag_patent ON patent_tag (patent_number);
