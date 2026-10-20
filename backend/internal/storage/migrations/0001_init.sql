-- Eco Terminal — initial schema.
-- Conventions: ids are TEXT (UUIDv4); timestamps are RFC3339 UTC TEXT; booleans are INTEGER 0/1;
-- *_json columns hold JSON text. Foreign keys are enforced (PRAGMA foreign_keys=ON set by the Go layer).
-- The sqlite-vec virtual table (rag_chunks_vec) is NOT created here: its dimension depends on the
-- configured embedding model, so internal/rag creates it at runtime (see embedding_index_metadata).

-- ───────────── identity ─────────────
CREATE TABLE roles (
    name        TEXT PRIMARY KEY,            -- citizen | researcher | reviewer | admin
    description TEXT NOT NULL DEFAULT ''
);
INSERT INTO roles(name, description) VALUES
    ('citizen',    'Creates observations and follows own reports'),
    ('researcher', 'Analysis regions, advanced filters, simulations'),
    ('reviewer',   'Reviews and verifies observations'),
    ('admin',      'User/role management, feeds, system configuration, audit log');

CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE COLLATE NOCASE,
    password_hash TEXT NOT NULL,              -- argon2id encoded string
    role          TEXT NOT NULL DEFAULT 'citizen' REFERENCES roles(name),
    status        TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended','deleted')),
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    deleted_at    TEXT
);

CREATE TABLE user_profiles (
    user_id      TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL DEFAULT '',
    locale       TEXT NOT NULL DEFAULT 'tr',
    avatar_path  TEXT,
    consent_location_at TEXT,                 -- user acknowledged location-sharing notice
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,          -- sha256 of opaque bearer token; raw token never stored
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT,
    user_agent TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_sessions_user ON sessions(user_id);

-- ───────────── geography ─────────────
CREATE TABLE geographic_regions (
    id         TEXT PRIMARY KEY,
    kind       TEXT NOT NULL CHECK (kind IN ('province','district','village','grid','custom')),
    name       TEXT NOT NULL,
    parent_id  TEXT REFERENCES geographic_regions(id),
    geojson    TEXT NOT NULL,                 -- Polygon/MultiPolygon geometry (GeoJSON geometry object)
    min_lon REAL NOT NULL, max_lon REAL NOT NULL, min_lat REAL NOT NULL, max_lat REAL NOT NULL,
    centroid_lon REAL NOT NULL, centroid_lat REAL NOT NULL,
    source     TEXT NOT NULL DEFAULT '',      -- provenance (e.g. "OpenStreetMap/ODbL", "generated hex grid")
    is_demo    INTEGER NOT NULL DEFAULT 0,
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_regions_kind ON geographic_regions(kind);
CREATE INDEX idx_regions_parent ON geographic_regions(parent_id);
CREATE VIRTUAL TABLE regions_rtree USING rtree(rid, min_lon, max_lon, min_lat, max_lat);
-- rtree rowid == geographic_regions.rowid; maintained by triggers.
CREATE TRIGGER trg_regions_ai AFTER INSERT ON geographic_regions BEGIN
    INSERT INTO regions_rtree(rid, min_lon, max_lon, min_lat, max_lat)
    VALUES (new.rowid, new.min_lon, new.max_lon, new.min_lat, new.max_lat);
END;
CREATE TRIGGER trg_regions_au AFTER UPDATE ON geographic_regions BEGIN
    UPDATE regions_rtree SET min_lon=new.min_lon, max_lon=new.max_lon, min_lat=new.min_lat, max_lat=new.max_lat
    WHERE rid = old.rowid;
END;
CREATE TRIGGER trg_regions_ad AFTER DELETE ON geographic_regions BEGIN
    DELETE FROM regions_rtree WHERE rid = old.rowid;
END;

-- ───────────── observations ─────────────
CREATE TABLE observations (
    id           TEXT PRIMARY KEY,
    reference_no TEXT NOT NULL UNIQUE,        -- human-friendly report number, e.g. ET-2026-000123
    user_id      TEXT NOT NULL REFERENCES users(id),
    record_type  TEXT NOT NULL CHECK (record_type IN ('observation','issue')),  -- Normal Gözlem | Sorun Bildirimi
    category     TEXT NOT NULL CHECK (category IN
                 ('animal','plant','water','forest','pollution','injured_wildlife','other')),
    title        TEXT NOT NULL DEFAULT '',
    description  TEXT NOT NULL DEFAULT '',
    species_guess TEXT NOT NULL DEFAULT '',   -- USER's guess; never treated as verified science
    species_verified TEXT NOT NULL DEFAULT '',-- set by reviewer only
    count_observed INTEGER,
    details_json TEXT NOT NULL DEFAULT '{}',  -- category-specific dynamic fields (spec §5 Step 4)
    lat          REAL NOT NULL,               -- exact location (owner/reviewer/admin only)
    lon          REAL NOT NULL,
    public_lat   REAL NOT NULL,               -- location exposed to others (generalized if is_sensitive)
    public_lon   REAL NOT NULL,
    location_source TEXT NOT NULL DEFAULT 'gps' CHECK (location_source IN ('gps','manual')),
    accuracy_m   REAL,
    observed_at  TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'submitted' CHECK (status IN
                 ('draft','submitted','ai_processed','under_review','verified','needs_information','rejected','resolved')),
    priority     TEXT NOT NULL DEFAULT 'normal' CHECK (priority IN ('low','normal','high','urgent')),
    is_sensitive INTEGER NOT NULL DEFAULT 0,  -- rare/protected species -> public coords generalized
    is_demo      INTEGER NOT NULL DEFAULT 0,
    primary_region_id TEXT REFERENCES geographic_regions(id),   -- smallest region containing the point
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    deleted_at   TEXT
);
CREATE INDEX idx_obs_user ON observations(user_id, created_at DESC);
CREATE INDEX idx_obs_status ON observations(status, created_at DESC);
CREATE INDEX idx_obs_category ON observations(category, observed_at DESC);
CREATE INDEX idx_obs_observed ON observations(observed_at);
CREATE INDEX idx_obs_region ON observations(primary_region_id);
-- Spatial index over PUBLIC coordinates (what non-privileged callers may see).
CREATE VIRTUAL TABLE observations_rtree USING rtree(oid, min_lon, max_lon, min_lat, max_lat);
CREATE TRIGGER trg_obs_ai AFTER INSERT ON observations BEGIN
    INSERT INTO observations_rtree(oid, min_lon, max_lon, min_lat, max_lat)
    VALUES (new.rowid, new.public_lon, new.public_lon, new.public_lat, new.public_lat);
END;
CREATE TRIGGER trg_obs_au AFTER UPDATE OF public_lat, public_lon ON observations BEGIN
    UPDATE observations_rtree SET min_lon=new.public_lon, max_lon=new.public_lon,
        min_lat=new.public_lat, max_lat=new.public_lat WHERE oid = old.rowid;
END;
CREATE TRIGGER trg_obs_ad AFTER DELETE ON observations BEGIN
    DELETE FROM observations_rtree WHERE oid = old.rowid;
END;

CREATE TABLE observation_media (
    id             TEXT PRIMARY KEY,
    observation_id TEXT NOT NULL REFERENCES observations(id) ON DELETE CASCADE,
    storage_path   TEXT NOT NULL,             -- relative to STORAGE_DIR, random filename
    mime_type      TEXT NOT NULL,
    size_bytes     INTEGER NOT NULL,
    width          INTEGER,
    height         INTEGER,
    sha256         TEXT NOT NULL,
    exif_stripped  INTEGER NOT NULL DEFAULT 1,
    position       INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
CREATE INDEX idx_media_obs ON observation_media(observation_id, position);

CREATE TABLE observation_ai_analysis (
    id             TEXT PRIMARY KEY,
    observation_id TEXT NOT NULL REFERENCES observations(id) ON DELETE CASCADE,
    provider       TEXT NOT NULL,
    model          TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','done','failed','skipped')),
    suggested_category TEXT,
    suggested_species  TEXT,
    suggested_description TEXT,
    confidence     REAL,                      -- 0..1, provider-reported; may be NULL
    raw_json       TEXT NOT NULL DEFAULT '{}',
    error          TEXT,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
CREATE INDEX idx_ai_analysis_obs ON observation_ai_analysis(observation_id);

CREATE TABLE observation_reviews (
    id             TEXT PRIMARY KEY,
    observation_id TEXT NOT NULL REFERENCES observations(id) ON DELETE CASCADE,
    reviewer_id    TEXT NOT NULL REFERENCES users(id),
    decision       TEXT NOT NULL CHECK (decision IN ('verify','reject','request_info','resolve','start_review')),
    corrected_category TEXT,
    corrected_species  TEXT,
    mark_sensitive INTEGER,                   -- NULL = unchanged
    note           TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
CREATE INDEX idx_reviews_obs ON observation_reviews(observation_id, created_at);

CREATE TABLE observation_status_history (
    id             TEXT PRIMARY KEY,
    observation_id TEXT NOT NULL REFERENCES observations(id) ON DELETE CASCADE,
    actor_id       TEXT REFERENCES users(id), -- NULL = system/AI
    actor_type     TEXT NOT NULL DEFAULT 'user' CHECK (actor_type IN ('user','system','ai')),
    from_status    TEXT,
    to_status      TEXT NOT NULL,
    note           TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL
);
CREATE INDEX idx_status_hist_obs ON observation_status_history(observation_id, created_at);

CREATE TABLE observation_region_links (
    observation_id TEXT NOT NULL REFERENCES observations(id) ON DELETE CASCADE,
    region_id      TEXT NOT NULL REFERENCES geographic_regions(id) ON DELETE CASCADE,
    PRIMARY KEY (observation_id, region_id)
);
CREATE INDEX idx_obs_region_links_region ON observation_region_links(region_id);

-- ───────────── official feeds ─────────────
CREATE TABLE rss_sources (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    organization  TEXT NOT NULL DEFAULT '',
    feed_url      TEXT NOT NULL UNIQUE,
    category      TEXT NOT NULL DEFAULT 'general',
    enabled       INTEGER NOT NULL DEFAULT 1,
    interval_minutes INTEGER NOT NULL DEFAULT 60,
    verified_at   TEXT,                       -- when the URL was last confirmed to return a valid feed
    last_fetched_at TEXT,
    last_status   TEXT NOT NULL DEFAULT '',
    last_error    TEXT NOT NULL DEFAULT '',
    etag          TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE rss_articles (
    id           TEXT PRIMARY KEY,
    source_id    TEXT NOT NULL REFERENCES rss_sources(id) ON DELETE CASCADE,
    guid         TEXT NOT NULL,
    url          TEXT NOT NULL,               -- source URL
    title        TEXT NOT NULL,
    summary      TEXT NOT NULL DEFAULT '',    -- sanitized plain text
    content_hash TEXT NOT NULL,
    category     TEXT NOT NULL DEFAULT 'general',
    published_at TEXT,                        -- from the feed (may be NULL)
    ingested_at  TEXT NOT NULL,               -- when Eco Terminal stored it
    location_note TEXT NOT NULL DEFAULT '',   -- e.g. "location uncertain"
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    UNIQUE (source_id, guid)
);
CREATE UNIQUE INDEX idx_rss_articles_hash ON rss_articles(content_hash);
CREATE INDEX idx_rss_articles_pub ON rss_articles(published_at DESC);

CREATE TABLE rss_article_regions (
    article_id TEXT NOT NULL REFERENCES rss_articles(id) ON DELETE CASCADE,
    region_id  TEXT NOT NULL REFERENCES geographic_regions(id) ON DELETE CASCADE,
    PRIMARY KEY (article_id, region_id)
);

-- ───────────── environmental data ─────────────
CREATE TABLE environmental_datasets (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,         -- e.g. "open-meteo-era5-daily"
    source      TEXT NOT NULL,
    license     TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    spatial_resolution TEXT NOT NULL DEFAULT '',
    temporal_resolution TEXT NOT NULL DEFAULT '',
    url         TEXT NOT NULL DEFAULT '',
    retrieved_at TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE environmental_measurements (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    dataset_id  TEXT NOT NULL REFERENCES environmental_datasets(id) ON DELETE CASCADE,
    region_id   TEXT NOT NULL REFERENCES geographic_regions(id) ON DELETE CASCADE,
    ts          TEXT NOT NULL,                -- date (YYYY-MM-DD) or RFC3339
    measurement_type TEXT NOT NULL,           -- precipitation | temperature_mean | temperature_max | temperature_min | ...
    value       REAL NOT NULL,
    unit        TEXT NOT NULL,                -- mm | degC | ...
    spatial_resolution TEXT NOT NULL DEFAULT '',
    quality_flag TEXT NOT NULL DEFAULT 'ok',
    UNIQUE (dataset_id, region_id, ts, measurement_type)
);
CREATE INDEX idx_env_meas_lookup ON environmental_measurements(region_id, measurement_type, ts);

-- ───────────── RAG ─────────────
CREATE TABLE rag_documents (
    id           TEXT PRIMARY KEY,
    source_type  TEXT NOT NULL CHECK (source_type IN ('observation','rss_article','scientific','region_report','manual')),
    source_id    TEXT NOT NULL DEFAULT '',    -- id in the source table (observation id, article id, ...)
    title        TEXT NOT NULL DEFAULT '',
    url          TEXT NOT NULL DEFAULT '',
    region_id    TEXT REFERENCES geographic_regions(id),
    lat          REAL, lon REAL,              -- public coordinates only
    doc_date     TEXT,
    visibility   TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public','verified_only','staff','owner')),
    owner_user_id TEXT REFERENCES users(id),  -- for visibility='owner'
    content_hash TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    UNIQUE (source_type, source_id)
);
CREATE INDEX idx_rag_docs_region ON rag_documents(region_id, doc_date);

CREATE TABLE rag_document_chunks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,   -- also the rowid in rag_chunks_fts / rag_chunks_vec
    document_id TEXT NOT NULL REFERENCES rag_documents(id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL,
    text        TEXT NOT NULL,
    token_estimate INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
    UNIQUE (document_id, chunk_index)
);
CREATE VIRTUAL TABLE rag_chunks_fts USING fts5(text, content='rag_document_chunks', content_rowid='id', tokenize='unicode61 remove_diacritics 2');
CREATE TRIGGER trg_chunks_ai AFTER INSERT ON rag_document_chunks BEGIN
    INSERT INTO rag_chunks_fts(rowid, text) VALUES (new.id, new.text);
END;
CREATE TRIGGER trg_chunks_ad AFTER DELETE ON rag_document_chunks BEGIN
    INSERT INTO rag_chunks_fts(rag_chunks_fts, rowid, text) VALUES ('delete', old.id, old.text);
END;

CREATE TABLE embedding_index_metadata (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    provider    TEXT NOT NULL,
    model       TEXT NOT NULL,
    dimensions  INTEGER NOT NULL,
    vec_table   TEXT NOT NULL DEFAULT 'rag_chunks_vec',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

-- ───────────── async jobs (AI analysis, embedding, feed sync) ─────────────
CREATE TABLE jobs (
    id          TEXT PRIMARY KEY,
    kind        TEXT NOT NULL,                -- analyze_observation | embed_document | sync_feed | ...
    payload_json TEXT NOT NULL DEFAULT '{}',
    status      TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','done','failed')),
    attempts    INTEGER NOT NULL DEFAULT 0,
    last_error  TEXT NOT NULL DEFAULT '',
    run_after   TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE INDEX idx_jobs_queue ON jobs(status, run_after);

-- ───────────── simulation ─────────────
CREATE TABLE simulation_scenarios (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id),
    name        TEXT NOT NULL,
    model       TEXT NOT NULL DEFAULT 'climate_drought_v1',
    region_id   TEXT NOT NULL REFERENCES geographic_regions(id),
    params_json TEXT NOT NULL,                -- reference period, precip_change_pct, temp_change_c, ...
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE simulation_results (
    id          TEXT PRIMARY KEY,
    scenario_id TEXT NOT NULL REFERENCES simulation_scenarios(id) ON DELETE CASCADE,
    status      TEXT NOT NULL CHECK (status IN ('ok','insufficient_data','error')),
    result_json TEXT NOT NULL,                -- reference vs scenario indicators, series, assumptions, limitations, data sources
    narrative   TEXT NOT NULL DEFAULT '',     -- optional LLM explanation; never contains new numbers
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE INDEX idx_sim_results_scenario ON simulation_results(scenario_id, created_at DESC);

-- ───────────── notifications & audit ─────────────
CREATE TABLE notifications (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL,                 -- observation_status | info_requested | system
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    observation_id TEXT REFERENCES observations(id) ON DELETE SET NULL,
    read_at    TEXT,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_notif_user ON notifications(user_id, created_at DESC);

CREATE TABLE audit_logs (
    id          TEXT PRIMARY KEY,
    actor_id    TEXT REFERENCES users(id),
    actor_role  TEXT NOT NULL DEFAULT '',
    action      TEXT NOT NULL,                -- e.g. observation.status_change, user.role_change, feed.sync
    entity_type TEXT NOT NULL DEFAULT '',
    entity_id   TEXT NOT NULL DEFAULT '',
    before_json TEXT NOT NULL DEFAULT '',
    after_json  TEXT NOT NULL DEFAULT '',
    ip          TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL
);
CREATE INDEX idx_audit_entity ON audit_logs(entity_type, entity_id, created_at);
CREATE INDEX idx_audit_actor ON audit_logs(actor_id, created_at);
