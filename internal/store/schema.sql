-- hetu schema (v0).
--
-- Conventions:
--   * Timestamps are unix seconds (INTEGER) for driver-agnostic scanning.
--   * owner_id is reserved on every table so multi-user is additive later.
--
-- Tables are added here as features land. Implemented so far: users, assets,
-- annotations (layered metadata), asset_colors (color-search index), folders,
-- tags, asset_tags, plus the assets_fts FTS5 full-text index. The remaining
-- target tables (shares, jobs, plus the Phase 1 vector table) are described in
-- docs/data-model.md and added when implemented.

CREATE TABLE IF NOT EXISTS users (
    id         TEXT PRIMARY KEY,
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS assets (
    id           TEXT PRIMARY KEY,
    owner_id     TEXT NOT NULL,
    kind         TEXT NOT NULL,
    provider     TEXT NOT NULL,
    storage_path TEXT NOT NULL,
    name         TEXT NOT NULL,
    ext          TEXT NOT NULL,
    size         INTEGER NOT NULL,
    hash         TEXT NOT NULL,
    thumb_path   TEXT NOT NULL DEFAULT '',
    width        INTEGER NOT NULL DEFAULT 0,
    height       INTEGER NOT NULL DEFAULT 0,
    created_at   INTEGER NOT NULL,
    indexed_at   INTEGER NOT NULL,
    deleted_at   INTEGER,                          -- NULL = live, unix ts = trashed
    rating       INTEGER NOT NULL DEFAULT 0,       -- 0-5 stars
    color        TEXT NOT NULL DEFAULT '',         -- color label, e.g. '#FF5733'
    display_name TEXT NOT NULL DEFAULT '',         -- user rename; empty = use name
    folder_id    TEXT NOT NULL DEFAULT ''          -- FK -> folders.id; empty = root
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_owner_path
    ON assets (owner_id, provider, storage_path);
CREATE INDEX IF NOT EXISTS idx_assets_owner ON assets (owner_id);
CREATE INDEX IF NOT EXISTS idx_assets_deleted ON assets (owner_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_assets_hash ON assets (owner_id, hash);

-- annotations is the layered-metadata store (manual > ai > extracted). Value is a
-- JSON payload; model is set only for the ai layer. Color extraction writes the
-- palette and dominant color into the extracted layer (see docs/ai-and-3d.md).
CREATE TABLE IF NOT EXISTS annotations (
    asset_id   TEXT NOT NULL,
    layer      TEXT NOT NULL,
    "key"      TEXT NOT NULL,
    value      TEXT NOT NULL,
    model      TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    PRIMARY KEY (asset_id, layer, "key")
);

-- asset_colors is the color-search index: one row per palette swatch with its
-- CIE-Lab coordinates precomputed so a query only has to run CIEDE2000 over
-- candidate rows. ord 0 is the dominant color; weight is the pixel fraction.
CREATE TABLE IF NOT EXISTS asset_colors (
    asset_id TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    ord      INTEGER NOT NULL,
    hex      TEXT NOT NULL,
    l        REAL NOT NULL,
    a        REAL NOT NULL,
    b        REAL NOT NULL,
    weight   REAL NOT NULL,
    PRIMARY KEY (asset_id, ord)
);
CREATE INDEX IF NOT EXISTS idx_asset_colors_owner ON asset_colors (owner_id);

CREATE TABLE IF NOT EXISTS folders (
    id        TEXT PRIMARY KEY,
    owner_id  TEXT NOT NULL,
    parent_id TEXT NOT NULL DEFAULT '',
    name      TEXT NOT NULL,
    path      TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_folders_owner_path ON folders (owner_id, path);

CREATE TABLE IF NOT EXISTS tags (
    id        TEXT PRIMARY KEY,
    owner_id  TEXT NOT NULL,
    parent_id TEXT NOT NULL DEFAULT '',
    name      TEXT NOT NULL,
    color     TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tags_owner_name ON tags (owner_id, name);

CREATE TABLE IF NOT EXISTS asset_tags (
    asset_id TEXT NOT NULL,
    tag_id   TEXT NOT NULL,
    source   TEXT NOT NULL DEFAULT 'manual',
    PRIMARY KEY (asset_id, tag_id)
);

-- ---------------------------------------------------------------------------
-- FTS5 full-text search index (contentless).
-- Columns: name (asset filename), tags (space-separated), description.
-- tags and description are empty until asset_tags/annotations sync is wired;
-- triggers below keep the index in sync with the assets table.
-- ---------------------------------------------------------------------------

CREATE VIRTUAL TABLE IF NOT EXISTS assets_fts USING fts5(
    name,
    tags,
    description,
    content='',
    tokenize='unicode61'
);

-- Sync triggers: mirror assets INSERT/UPDATE/DELETE into assets_fts.
-- Contentless FTS5 tables require the special delete command with original
-- values for removal (INSERT INTO ... VALUES('delete', rowid, ...)).

CREATE TRIGGER IF NOT EXISTS trg_assets_ai AFTER INSERT ON assets BEGIN
    INSERT INTO assets_fts(rowid, name, tags, description)
    VALUES (new.rowid, new.name, '', '');
END;

CREATE TRIGGER IF NOT EXISTS trg_assets_au AFTER UPDATE ON assets BEGIN
    INSERT INTO assets_fts(assets_fts, rowid, name, tags, description)
    VALUES ('delete', old.rowid, old.name, '', '');
    INSERT INTO assets_fts(rowid, name, tags, description)
    VALUES (new.rowid, new.name, '', '');
END;

CREATE TRIGGER IF NOT EXISTS trg_assets_ad AFTER DELETE ON assets BEGIN
    INSERT INTO assets_fts(assets_fts, rowid, name, tags, description)
    VALUES ('delete', old.rowid, old.name, '', '');
END;
