CREATE TABLE IF NOT EXISTS schema_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '2')
ON CONFLICT(key) DO NOTHING;

CREATE TABLE IF NOT EXISTS patent (
    number               TEXT PRIMARY KEY,
    country              TEXT NOT NULL DEFAULT '',
    serial               TEXT NOT NULL DEFAULT '',
    kind                 TEXT NOT NULL DEFAULT '',
    display_number       TEXT NOT NULL DEFAULT '',
    title                TEXT NOT NULL DEFAULT '',
    abstract             TEXT NOT NULL DEFAULT '',
    assignee             TEXT NOT NULL DEFAULT '',
    inventors            TEXT NOT NULL DEFAULT '[]',
    fetch_state          TEXT NOT NULL,
    source               TEXT NOT NULL DEFAULT '',
    application_date     TEXT NOT NULL DEFAULT '',
    publication_date     TEXT NOT NULL DEFAULT '',
    grant_date           TEXT NOT NULL DEFAULT '',
    fetched_at           TEXT NOT NULL DEFAULT '',
    updated_at           TEXT NOT NULL DEFAULT '',
    first_claim          TEXT NOT NULL DEFAULT '',
    expiration_date      TEXT NOT NULL DEFAULT '',
    expiration_source    TEXT NOT NULL DEFAULT '',
    source_url           TEXT NOT NULL DEFAULT '',
    classifications      TEXT NOT NULL DEFAULT '[]',
    classifications_text TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS document (
    number        TEXT PRIMARY KEY,
    record_number TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
    country       TEXT NOT NULL DEFAULT '',
    serial        TEXT NOT NULL DEFAULT '',
    kind          TEXT NOT NULL DEFAULT '',
    stage         TEXT NOT NULL,
    dated         TEXT NOT NULL DEFAULT '',
    source        TEXT NOT NULL DEFAULT '',
    source_ref    TEXT NOT NULL DEFAULT '',
    UNIQUE (record_number, stage, number)
);

CREATE INDEX IF NOT EXISTS idx_document_record_stage ON document (record_number, stage);
CREATE INDEX IF NOT EXISTS idx_document_stage_dated ON document (stage, dated);

CREATE TABLE IF NOT EXISTS relation (
    from_number TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
    to_number   TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT '',
    source_ref  TEXT NOT NULL DEFAULT '',
    observed_at TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (from_number, to_number, kind)
);

CREATE INDEX IF NOT EXISTS idx_relation_from_kind ON relation (from_number, kind);
CREATE INDEX IF NOT EXISTS idx_relation_to_kind ON relation (to_number, kind);
CREATE INDEX IF NOT EXISTS idx_relation_kind ON relation (kind);

CREATE TABLE IF NOT EXISTS authority_identifier (
    authority       TEXT NOT NULL,
    identifier_type TEXT NOT NULL,
    identifier      TEXT NOT NULL,
    raw_identifier  TEXT NOT NULL DEFAULT '',
    record_number   TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
    document_number TEXT NOT NULL DEFAULT '',
    country         TEXT NOT NULL DEFAULT '',
    kind            TEXT NOT NULL DEFAULT '',
    dated           TEXT NOT NULL DEFAULT '',
    source          TEXT NOT NULL DEFAULT '',
    confidence      INTEGER NOT NULL DEFAULT 100,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    PRIMARY KEY (authority, identifier_type, identifier)
);

CREATE INDEX IF NOT EXISTS idx_authority_identifier_record ON authority_identifier (record_number);
CREATE INDEX IF NOT EXISTS idx_authority_identifier_lookup ON authority_identifier (identifier);
CREATE INDEX IF NOT EXISTS idx_authority_identifier_document ON authority_identifier (document_number);

CREATE TABLE IF NOT EXISTS uspto_application (
    application_number              TEXT PRIMARY KEY,
    record_number                   TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
    invention_title                 TEXT NOT NULL DEFAULT '',
    filing_date                     TEXT NOT NULL DEFAULT '',
    effective_filing_date           TEXT NOT NULL DEFAULT '',
    application_status_code         TEXT NOT NULL DEFAULT '',
    application_status_text         TEXT NOT NULL DEFAULT '',
    application_status_date         TEXT NOT NULL DEFAULT '',
    application_type_code           TEXT NOT NULL DEFAULT '',
    application_type_label          TEXT NOT NULL DEFAULT '',
    application_type_category       TEXT NOT NULL DEFAULT '',
    first_inventor_to_file          INTEGER NOT NULL DEFAULT 0,
    national_stage                  INTEGER NOT NULL DEFAULT 0,
    first_inventor_name             TEXT NOT NULL DEFAULT '',
    first_applicant_name            TEXT NOT NULL DEFAULT '',
    customer_number                 TEXT NOT NULL DEFAULT '',
    group_art_unit_number           TEXT NOT NULL DEFAULT '',
    examiner_name                   TEXT NOT NULL DEFAULT '',
    docket_number                   TEXT NOT NULL DEFAULT '',
    application_confirmation_number TEXT NOT NULL DEFAULT '',
    uspc_symbol_text                TEXT NOT NULL DEFAULT '',
    uspc_class                      TEXT NOT NULL DEFAULT '',
    uspc_subclass                   TEXT NOT NULL DEFAULT '',
    small_entity_status             INTEGER NOT NULL DEFAULT 0,
    business_entity_status          TEXT NOT NULL DEFAULT '',
    publication_category_json       TEXT NOT NULL DEFAULT '[]',
    last_ingestion_datetime         TEXT NOT NULL DEFAULT '',
    fetched_at                      TEXT NOT NULL,
    pgpub_xml_url                   TEXT NOT NULL DEFAULT '',
    pgpub_xml_name                  TEXT NOT NULL DEFAULT '',
    patent_grant_xml_url            TEXT NOT NULL DEFAULT '',
    patent_grant_xml_name           TEXT NOT NULL DEFAULT '',
    -- Fields populated from the grant XML when it has been parsed.
    grant_doc_number                TEXT NOT NULL DEFAULT '',
    grant_kind                      TEXT NOT NULL DEFAULT '',
    grant_date                      TEXT NOT NULL DEFAULT '',
    grant_dtd_version               TEXT NOT NULL DEFAULT '',
    grant_status                    TEXT NOT NULL DEFAULT '',
    grant_date_produced             TEXT NOT NULL DEFAULT '',
    grant_file_name                 TEXT NOT NULL DEFAULT '',
    grant_lang                      TEXT NOT NULL DEFAULT '',
    term_extension_days             INTEGER NOT NULL DEFAULT 0,
    number_of_claims                INTEGER NOT NULL DEFAULT 0,
    exemplary_claim                 TEXT NOT NULL DEFAULT '',
    number_of_drawing_sheets        INTEGER NOT NULL DEFAULT 0,
    number_of_figures               INTEGER NOT NULL DEFAULT 0,
    primary_examiner_first          TEXT NOT NULL DEFAULT '',
    primary_examiner_last           TEXT NOT NULL DEFAULT '',
    primary_examiner_department     TEXT NOT NULL DEFAULT '',
    assistant_examiners_json        TEXT NOT NULL DEFAULT '[]',
    attorney_org                    TEXT NOT NULL DEFAULT '',
    attorney_type                   TEXT NOT NULL DEFAULT '',
    field_of_search_json            TEXT NOT NULL DEFAULT '[]',
    grant_parsed_at                 TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_uspto_application_record ON uspto_application (record_number);
CREATE INDEX IF NOT EXISTS idx_uspto_application_status ON uspto_application (application_status_code, application_status_date);
CREATE INDEX IF NOT EXISTS idx_uspto_application_filing ON uspto_application (filing_date);

CREATE TABLE IF NOT EXISTS uspto_party (
    application_number    TEXT NOT NULL REFERENCES uspto_application (application_number) ON DELETE CASCADE,
    role                  TEXT NOT NULL,
    ordinal               INTEGER NOT NULL DEFAULT 0,
    name_text             TEXT NOT NULL DEFAULT '',
    first_name            TEXT NOT NULL DEFAULT '',
    middle_name           TEXT NOT NULL DEFAULT '',
    last_name             TEXT NOT NULL DEFAULT '',
    name_suffix           TEXT NOT NULL DEFAULT '',
    organization_name     TEXT NOT NULL DEFAULT '',
    registration_number   TEXT NOT NULL DEFAULT '',
    active_indicator      TEXT NOT NULL DEFAULT '',
    practitioner_category TEXT NOT NULL DEFAULT '',
    address_json          TEXT NOT NULL DEFAULT '{}',
    telecom_json          TEXT NOT NULL DEFAULT '{}',
    PRIMARY KEY (application_number, role, ordinal)
);

CREATE INDEX IF NOT EXISTS idx_uspto_party_name ON uspto_party (name_text);
CREATE INDEX IF NOT EXISTS idx_uspto_party_role ON uspto_party (role);

CREATE TABLE IF NOT EXISTS uspto_event (
    application_number     TEXT NOT NULL REFERENCES uspto_application (application_number) ON DELETE CASCADE,
    ordinal                INTEGER NOT NULL DEFAULT 0,
    event_code             TEXT NOT NULL,
    event_description_text TEXT NOT NULL DEFAULT '',
    event_date             TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (application_number, event_code, event_date, ordinal)
);

CREATE INDEX IF NOT EXISTS idx_uspto_event_date ON uspto_event (event_date);
CREATE INDEX IF NOT EXISTS idx_uspto_event_code ON uspto_event (event_code);

CREATE TABLE IF NOT EXISTS uspto_continuity (
    application_number                    TEXT NOT NULL REFERENCES uspto_application (application_number) ON DELETE CASCADE,
    ordinal                               INTEGER NOT NULL DEFAULT 0,
    parent_application_number_text        TEXT NOT NULL DEFAULT '',
    child_application_number_text         TEXT NOT NULL DEFAULT '',
    parent_application_filing_date        TEXT NOT NULL DEFAULT '',
    parent_application_status_code        TEXT NOT NULL DEFAULT '',
    parent_application_status_text        TEXT NOT NULL DEFAULT '',
    claim_parentage_type_code             TEXT NOT NULL DEFAULT '',
    claim_parentage_type_description_text TEXT NOT NULL DEFAULT '',
    parent_record_number                  TEXT NOT NULL DEFAULT '',
    child_record_number                   TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (application_number, ordinal)
);

CREATE INDEX IF NOT EXISTS idx_uspto_continuity_parent ON uspto_continuity (parent_application_number_text);
CREATE INDEX IF NOT EXISTS idx_uspto_continuity_child ON uspto_continuity (child_application_number_text);

CREATE TABLE IF NOT EXISTS uspto_foreign_priority (
    application_number         TEXT NOT NULL REFERENCES uspto_application (application_number) ON DELETE CASCADE,
    ordinal                    INTEGER NOT NULL DEFAULT 0,
    foreign_application_number TEXT NOT NULL DEFAULT '',
    filing_date                TEXT NOT NULL DEFAULT '',
    ip_office_name             TEXT NOT NULL DEFAULT '',
    authority                  TEXT NOT NULL DEFAULT '',
    linked_record_number       TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (application_number, ordinal)
);

CREATE INDEX IF NOT EXISTS idx_uspto_foreign_priority_number ON uspto_foreign_priority (foreign_application_number);

CREATE TABLE IF NOT EXISTS uspto_assignment (
    application_number               TEXT NOT NULL REFERENCES uspto_application (application_number) ON DELETE CASCADE,
    ordinal                          INTEGER NOT NULL DEFAULT 0,
    reel_and_frame_number            TEXT NOT NULL DEFAULT '',
    reel_number                      TEXT NOT NULL DEFAULT '',
    frame_number                     TEXT NOT NULL DEFAULT '',
    conveyance_text                  TEXT NOT NULL DEFAULT '',
    assignment_received_date         TEXT NOT NULL DEFAULT '',
    assignment_recorded_date         TEXT NOT NULL DEFAULT '',
    assignment_mailed_date           TEXT NOT NULL DEFAULT '',
    assignment_document_location_uri TEXT NOT NULL DEFAULT '',
    attorney_docket_number           TEXT NOT NULL DEFAULT '',
    page_total_quantity              INTEGER NOT NULL DEFAULT 0,
    image_available                  INTEGER NOT NULL DEFAULT 0,
    correspondence_address_json      TEXT NOT NULL DEFAULT '{}',
    PRIMARY KEY (application_number, ordinal)
);

CREATE INDEX IF NOT EXISTS idx_uspto_assignment_recorded ON uspto_assignment (assignment_recorded_date);

CREATE TABLE IF NOT EXISTS uspto_assignment_party (
    application_number TEXT NOT NULL,
    assignment_ordinal INTEGER NOT NULL,
    role               TEXT NOT NULL,
    ordinal            INTEGER NOT NULL DEFAULT 0,
    name_text          TEXT NOT NULL DEFAULT '',
    execution_date     TEXT NOT NULL DEFAULT '',
    address_json       TEXT NOT NULL DEFAULT '{}',
    PRIMARY KEY (application_number, assignment_ordinal, role, ordinal),
    FOREIGN KEY (application_number, assignment_ordinal)
        REFERENCES uspto_assignment (application_number, ordinal)
        ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS uspto_document (
    document_number      TEXT PRIMARY KEY,
    record_number        TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
    application_number   TEXT NOT NULL DEFAULT '',
    document_stage       TEXT NOT NULL,
    country              TEXT NOT NULL DEFAULT 'US',
    kind_code            TEXT NOT NULL DEFAULT '',
    publication_date     TEXT NOT NULL DEFAULT '',
    grant_date           TEXT NOT NULL DEFAULT '',
    filing_date          TEXT NOT NULL DEFAULT '',
    source_product       TEXT NOT NULL DEFAULT '',
    bulk_file_id         TEXT NOT NULL DEFAULT '',
    snapshot_id          TEXT NOT NULL DEFAULT '',
    title                TEXT NOT NULL DEFAULT '',
    abstract             TEXT NOT NULL DEFAULT '',
    first_claim          TEXT NOT NULL DEFAULT '',
    classifications_json TEXT NOT NULL DEFAULT '[]',
    inventors_json       TEXT NOT NULL DEFAULT '[]',
    assignees_json       TEXT NOT NULL DEFAULT '[]',
    fetched_at           TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_uspto_document_record ON uspto_document (record_number);
CREATE INDEX IF NOT EXISTS idx_uspto_document_application ON uspto_document (application_number);
CREATE INDEX IF NOT EXISTS idx_uspto_document_publication_date ON uspto_document (publication_date);

CREATE TABLE IF NOT EXISTS uspto_bulk_file (
    id                  TEXT PRIMARY KEY,
    product             TEXT NOT NULL,
    publication_date    TEXT NOT NULL,
    year                TEXT NOT NULL DEFAULT '',
    url                 TEXT NOT NULL,
    local_path          TEXT NOT NULL DEFAULT '',
    payload_hash        TEXT NOT NULL DEFAULT '',
    bytes_downloaded    INTEGER NOT NULL DEFAULT 0,
    downloaded_at       TEXT NOT NULL DEFAULT '',
    imported_at         TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL DEFAULT '',
    import_summary_json TEXT NOT NULL DEFAULT '{}',
    UNIQUE (product, publication_date)
);

CREATE INDEX IF NOT EXISTS idx_uspto_bulk_file_status ON uspto_bulk_file (status);

CREATE TABLE IF NOT EXISTS uspto_grant_body (
    application_number    TEXT NOT NULL REFERENCES uspto_application (application_number) ON DELETE CASCADE,
    kind                  TEXT NOT NULL,                  -- 'pgpub' or 'grant'
    abstract_text         TEXT NOT NULL DEFAULT '',
    abstract_xml          TEXT NOT NULL DEFAULT '',
    description_text      TEXT NOT NULL DEFAULT '',
    description_xml       TEXT NOT NULL DEFAULT '',
    claim_statement       TEXT NOT NULL DEFAULT '',
    claims_text           TEXT NOT NULL DEFAULT '',
    claims_json           TEXT NOT NULL DEFAULT '[]',
    parsed_at             TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (application_number, kind)
);

CREATE INDEX IF NOT EXISTS idx_uspto_grant_body_kind ON uspto_grant_body (kind);

CREATE TABLE IF NOT EXISTS uspto_drawing (
    application_number    TEXT NOT NULL REFERENCES uspto_application (application_number) ON DELETE CASCADE,
    kind                  TEXT NOT NULL,
    ordinal               INTEGER NOT NULL,
    figure_num            TEXT NOT NULL DEFAULT '',
    figure_id             TEXT NOT NULL DEFAULT '',
    img_id                TEXT NOT NULL DEFAULT '',
    file_name             TEXT NOT NULL DEFAULT '',
    width                 TEXT NOT NULL DEFAULT '',
    height                TEXT NOT NULL DEFAULT '',
    alt_text              TEXT NOT NULL DEFAULT '',
    img_format            TEXT NOT NULL DEFAULT '',
    img_content           TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (application_number, kind, ordinal)
);

CREATE TABLE IF NOT EXISTS uspto_grant_citation (
    application_number    TEXT NOT NULL REFERENCES uspto_application (application_number) ON DELETE CASCADE,
    kind                  TEXT NOT NULL,
    ordinal               INTEGER NOT NULL,
    citation_num          TEXT NOT NULL DEFAULT '',
    citation_type         TEXT NOT NULL DEFAULT '',        -- 'patent' or 'npl'
    category              TEXT NOT NULL DEFAULT '',        -- 'cited by examiner' / 'applicant' / 'other'
    cited_country         TEXT NOT NULL DEFAULT '',
    cited_doc_number      TEXT NOT NULL DEFAULT '',
    cited_kind            TEXT NOT NULL DEFAULT '',
    cited_date            TEXT NOT NULL DEFAULT '',
    cited_name            TEXT NOT NULL DEFAULT '',
    cpc_text              TEXT NOT NULL DEFAULT '',
    national_country      TEXT NOT NULL DEFAULT '',
    national_class        TEXT NOT NULL DEFAULT '',
    npl_text              TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (application_number, kind, ordinal)
);

CREATE INDEX IF NOT EXISTS idx_uspto_grant_citation_cited ON uspto_grant_citation (cited_country, cited_doc_number);

CREATE TABLE IF NOT EXISTS uspto_grant_classification (
    application_number    TEXT NOT NULL REFERENCES uspto_application (application_number) ON DELETE CASCADE,
    kind                  TEXT NOT NULL,
    scheme                TEXT NOT NULL,                   -- 'ipcr' or 'cpc'
    role                  TEXT NOT NULL DEFAULT '',        -- 'main' / 'further' / 'search'
    ordinal               INTEGER NOT NULL,
    full_code             TEXT NOT NULL DEFAULT '',        -- e.g. 'G01N 33/42'
    section               TEXT NOT NULL DEFAULT '',
    class                 TEXT NOT NULL DEFAULT '',
    subclass              TEXT NOT NULL DEFAULT '',
    main_group            TEXT NOT NULL DEFAULT '',
    subgroup              TEXT NOT NULL DEFAULT '',
    symbol_position       TEXT NOT NULL DEFAULT '',
    classification_value  TEXT NOT NULL DEFAULT '',
    classification_level  TEXT NOT NULL DEFAULT '',
    classification_status TEXT NOT NULL DEFAULT '',
    data_source           TEXT NOT NULL DEFAULT '',
    action_date           TEXT NOT NULL DEFAULT '',
    generating_office     TEXT NOT NULL DEFAULT '',
    version_date          TEXT NOT NULL DEFAULT '',
    scheme_origination    TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (application_number, kind, scheme, role, ordinal)
);

CREATE INDEX IF NOT EXISTS idx_uspto_grant_classification_code ON uspto_grant_classification (full_code);

CREATE TABLE IF NOT EXISTS uspto_grant_relation (
    application_number     TEXT NOT NULL REFERENCES uspto_application (application_number) ON DELETE CASCADE,
    kind                   TEXT NOT NULL,
    ordinal                INTEGER NOT NULL,
    relation_kind          TEXT NOT NULL,                  -- continuation / continuation-in-part / division / reissue / provisional / related-publication
    parent_country         TEXT NOT NULL DEFAULT '',
    parent_app_number      TEXT NOT NULL DEFAULT '',
    parent_app_date        TEXT NOT NULL DEFAULT '',
    parent_grant_country   TEXT NOT NULL DEFAULT '',
    parent_grant_number    TEXT NOT NULL DEFAULT '',
    parent_grant_date      TEXT NOT NULL DEFAULT '',
    child_country          TEXT NOT NULL DEFAULT '',
    child_app_number       TEXT NOT NULL DEFAULT '',
    related_country        TEXT NOT NULL DEFAULT '',
    related_doc_number     TEXT NOT NULL DEFAULT '',
    related_kind           TEXT NOT NULL DEFAULT '',
    related_date           TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (application_number, kind, ordinal)
);

CREATE INDEX IF NOT EXISTS idx_uspto_grant_relation_kind ON uspto_grant_relation (relation_kind);

CREATE TABLE IF NOT EXISTS uspto_xml_download (
    application_number    TEXT NOT NULL REFERENCES uspto_application (application_number) ON DELETE CASCADE,
    kind                  TEXT NOT NULL,
    source_url            TEXT NOT NULL,
    local_path            TEXT NOT NULL,
    bytes                 INTEGER NOT NULL DEFAULT 0,
    download_count        INTEGER NOT NULL DEFAULT 0,
    first_downloaded_at   TEXT NOT NULL DEFAULT '',
    last_downloaded_at    TEXT NOT NULL DEFAULT '',
    last_accessed_at      TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (application_number, kind)
);

CREATE INDEX IF NOT EXISTS idx_uspto_xml_download_kind ON uspto_xml_download (kind);

CREATE TABLE IF NOT EXISTS source_snapshot (
    id               TEXT PRIMARY KEY,
    patent_number    TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
    source           TEXT NOT NULL,
    source_record_id TEXT NOT NULL DEFAULT '',
    source_url       TEXT NOT NULL DEFAULT '',
    fetched_at       TEXT NOT NULL,
    payload_kind     TEXT NOT NULL DEFAULT '',
    payload_hash     TEXT NOT NULL DEFAULT '',
    payload_path     TEXT NOT NULL DEFAULT '',
    response_bytes   INTEGER NOT NULL DEFAULT 0,
    http_status      INTEGER NOT NULL DEFAULT 0,
    etag             TEXT NOT NULL DEFAULT '',
    last_modified    TEXT NOT NULL DEFAULT '',
    summary_json     TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_source_snapshot_patent ON source_snapshot (patent_number, fetched_at DESC);
CREATE INDEX IF NOT EXISTS idx_source_snapshot_source ON source_snapshot (source, fetched_at DESC);

CREATE TABLE IF NOT EXISTS source_diff (
    id                 TEXT PRIMARY KEY,
    patent_number      TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
    field_path         TEXT NOT NULL,
    uspto_value        TEXT NOT NULL DEFAULT '',
    google_value       TEXT NOT NULL DEFAULT '',
    chosen_value       TEXT NOT NULL DEFAULT '',
    chosen_source      TEXT NOT NULL DEFAULT '',
    severity           TEXT NOT NULL DEFAULT '',
    recorded_at        TEXT NOT NULL,
    uspto_snapshot_id  TEXT NOT NULL DEFAULT '',
    google_snapshot_id TEXT NOT NULL DEFAULT '',

    -- Reconciliation metadata (Option A)
    reconciled_at      TEXT,
    reconciled_by      TEXT,
    reconciled_choice  TEXT
);

CREATE INDEX IF NOT EXISTS idx_source_diff_patent ON source_diff (patent_number, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_source_diff_field ON source_diff (field_path);

CREATE TABLE IF NOT EXISTS refresh_run (
    id                 TEXT PRIMARY KEY,
    kind               TEXT NOT NULL,
    status             TEXT NOT NULL,
    project_id         TEXT NOT NULL DEFAULT '',
    root_number        TEXT NOT NULL DEFAULT '',
    started_at         TEXT NOT NULL,
    finished_at        TEXT NOT NULL DEFAULT '',
    patents_checked    INTEGER NOT NULL DEFAULT 0,
    patents_updated    INTEGER NOT NULL DEFAULT 0,
    documents_added    INTEGER NOT NULL DEFAULT 0,
    relations_added    INTEGER NOT NULL DEFAULT 0,
    parents_added      INTEGER NOT NULL DEFAULT 0,
    children_added     INTEGER NOT NULL DEFAULT 0,
    citations_added    INTEGER NOT NULL DEFAULT 0,
    differences_found  INTEGER NOT NULL DEFAULT 0,
    uspto_requests     INTEGER NOT NULL DEFAULT 0,
    google_requests    INTEGER NOT NULL DEFAULT 0,
    bytes_downloaded   INTEGER NOT NULL DEFAULT 0,
    failures           INTEGER NOT NULL DEFAULT 0,
    summary_json       TEXT NOT NULL DEFAULT '{}',
    error_text         TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_refresh_run_started ON refresh_run (started_at DESC);
CREATE INDEX IF NOT EXISTS idx_refresh_run_project ON refresh_run (project_id, started_at DESC);

CREATE TABLE IF NOT EXISTS refresh_run_item (
    run_id             TEXT NOT NULL REFERENCES refresh_run (id) ON DELETE CASCADE,
    ordinal            INTEGER NOT NULL DEFAULT 0,
    patent_number      TEXT NOT NULL DEFAULT '',
    application_number TEXT NOT NULL DEFAULT '',
    action             TEXT NOT NULL DEFAULT '',
    source             TEXT NOT NULL DEFAULT '',
    message            TEXT NOT NULL DEFAULT '',
    before_json        TEXT NOT NULL DEFAULT '{}',
    after_json         TEXT NOT NULL DEFAULT '{}',
    PRIMARY KEY (run_id, ordinal)
);

CREATE INDEX IF NOT EXISTS idx_refresh_run_item_patent ON refresh_run_item (patent_number);

CREATE TABLE IF NOT EXISTS project (
    id                     TEXT PRIMARY KEY,
    name                   TEXT NOT NULL,
    created_at             TEXT NOT NULL,
    application_number     TEXT NOT NULL DEFAULT '',
    filing_date            TEXT NOT NULL DEFAULT '',
    art_unit               TEXT NOT NULL DEFAULT '',
    attorney_docket_number TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS project_inventor (
    project_id TEXT NOT NULL REFERENCES project (id) ON DELETE CASCADE,
    ordering   INTEGER NOT NULL,
    name       TEXT NOT NULL,
    PRIMARY KEY (project_id, ordering)
);

CREATE TABLE IF NOT EXISTS project_examiner (
    project_id  TEXT NOT NULL REFERENCES project (id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    recorded_at TEXT NOT NULL,
    PRIMARY KEY (project_id, recorded_at, name)
);

CREATE INDEX IF NOT EXISTS idx_project_examiner_latest ON project_examiner (project_id, recorded_at DESC);

CREATE TABLE IF NOT EXISTS membership (
    project_id            TEXT NOT NULL REFERENCES project (id) ON DELETE CASCADE,
    patent_number         TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
    state                 TEXT NOT NULL,
    added_at              TEXT NOT NULL,
    ids_kind_code         TEXT NOT NULL DEFAULT '',
    ids_in_full           INTEGER NOT NULL DEFAULT 0,
    ids_relevant_passages TEXT NOT NULL DEFAULT '',
    ids_notes             TEXT NOT NULL DEFAULT '',
    ids_status            TEXT NOT NULL DEFAULT '',
    ids_added_at          TEXT NOT NULL DEFAULT '',
    ids_submitted_at      TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (project_id, patent_number)
);

CREATE INDEX IF NOT EXISTS idx_membership_project ON membership (project_id, state);

CREATE TABLE IF NOT EXISTS project_patent_note (
    project_id    TEXT NOT NULL REFERENCES project (id) ON DELETE CASCADE,
    patent_number TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
    markdown      TEXT NOT NULL DEFAULT '',
    added_at      TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    PRIMARY KEY (project_id, patent_number)
);

CREATE INDEX IF NOT EXISTS idx_project_patent_note_project ON project_patent_note (project_id, updated_at DESC);

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

CREATE VIRTUAL TABLE IF NOT EXISTS patent_fts USING fts5 (
    title,
    abstract,
    classifications_text,
    content='patent',
    content_rowid='rowid'
);

CREATE TRIGGER IF NOT EXISTS patent_fts_insert AFTER INSERT ON patent BEGIN
    INSERT INTO patent_fts (rowid, title, abstract, classifications_text)
    VALUES (new.rowid, new.title, new.abstract, new.classifications_text);
END;

CREATE TRIGGER IF NOT EXISTS patent_fts_delete AFTER DELETE ON patent BEGIN
    INSERT INTO patent_fts (patent_fts, rowid, title, abstract, classifications_text)
    VALUES ('delete', old.rowid, old.title, old.abstract, old.classifications_text);
END;

CREATE TRIGGER IF NOT EXISTS patent_fts_update AFTER UPDATE ON patent BEGIN
    INSERT INTO patent_fts (patent_fts, rowid, title, abstract, classifications_text)
    VALUES ('delete', old.rowid, old.title, old.abstract, old.classifications_text);
    INSERT INTO patent_fts (rowid, title, abstract, classifications_text)
    VALUES (new.rowid, new.title, new.abstract, new.classifications_text);
END;

CREATE TABLE IF NOT EXISTS classification_definition (
    system      TEXT NOT NULL,
    code        TEXT NOT NULL,
    section     TEXT NOT NULL DEFAULT '',
    class       TEXT NOT NULL DEFAULT '',
    subclass    TEXT NOT NULL DEFAULT '',
    main_group  TEXT NOT NULL DEFAULT '',
    subgroup    TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (system, code)
);

CREATE TABLE IF NOT EXISTS mutation_group (
    id                      TEXT PRIMARY KEY,
    project_id              TEXT NOT NULL DEFAULT '',
    action                  TEXT NOT NULL,
    created_at              TEXT NOT NULL,
    selection_snapshot_json TEXT NOT NULL DEFAULT '[]',
    attrs_json              TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_mutation_group_project_created ON mutation_group (project_id, created_at DESC);

CREATE TABLE IF NOT EXISTS mutation_item (
    group_id      TEXT NOT NULL REFERENCES mutation_group (id) ON DELETE CASCADE,
    ordinal       INTEGER NOT NULL,
    patent_number TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
    kind          TEXT NOT NULL,
    before_json   TEXT NOT NULL DEFAULT 'null',
    after_json    TEXT NOT NULL DEFAULT 'null',
    inverse_json  TEXT NOT NULL DEFAULT 'null',
    PRIMARY KEY (group_id, ordinal)
);

CREATE INDEX IF NOT EXISTS idx_mutation_item_patent ON mutation_item (patent_number, group_id);

CREATE TABLE IF NOT EXISTS saved_table_view (
    id         TEXT PRIMARY KEY,
    owner      TEXT NOT NULL,
    table_type TEXT NOT NULL,
    name       TEXT NOT NULL,
    scope      TEXT NOT NULL DEFAULT 'user',
    is_default INTEGER NOT NULL DEFAULT 0,
    view_json  TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (owner, table_type, scope, name)
);

CREATE INDEX IF NOT EXISTS idx_saved_table_view_owner_table ON saved_table_view (owner, table_type, scope, updated_at DESC);
