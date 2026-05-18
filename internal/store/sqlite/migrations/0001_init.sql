-- Initial schema: patents, family-graph relations, projects, memberships.

CREATE TABLE patent (
    number           TEXT PRIMARY KEY,
    country          TEXT NOT NULL,
    serial           TEXT NOT NULL,
    kind             TEXT NOT NULL,
    title            TEXT NOT NULL,
    abstract         TEXT NOT NULL,
    assignee         TEXT NOT NULL,
    inventors        TEXT NOT NULL,   -- JSON array of strings
    fetch_state      TEXT NOT NULL,
    source           TEXT NOT NULL,
    application_date TEXT NOT NULL,   -- RFC3339, empty when unknown
    publication_date TEXT NOT NULL,
    grant_date       TEXT NOT NULL,
    fetched_at       TEXT NOT NULL
);

CREATE TABLE relation (
    from_number TEXT NOT NULL,
    to_number   TEXT NOT NULL,
    kind        TEXT NOT NULL,
    PRIMARY KEY (from_number, to_number, kind)
);

CREATE INDEX idx_relation_from ON relation (from_number, kind);
CREATE INDEX idx_relation_to ON relation (to_number, kind);

CREATE TABLE project (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE membership (
    project_id    TEXT NOT NULL REFERENCES project (id),
    patent_number TEXT NOT NULL REFERENCES patent (number),
    state         TEXT NOT NULL,
    added_at      TEXT NOT NULL,
    PRIMARY KEY (project_id, patent_number)
);

CREATE INDEX idx_membership_project ON membership (project_id, state);
