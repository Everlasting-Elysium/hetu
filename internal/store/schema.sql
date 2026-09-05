-- hetu schema (v0).
--
-- Conventions:
--   * Timestamps are unix seconds (INTEGER) for driver-agnostic scanning.
--   * owner_id is reserved on every table so multi-user is additive later.
--
-- Tables are added here as features land. Implemented so far: users, assets,
-- annotations (layered metadata), asset_colors (color-search index), folders,
-- tags, asset_tags, shares, jobs, plus the assets_fts FTS5 full-text index. The
-- remaining target table (the Phase 1 vector table) is described in
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
-- FTS5 full-text search index.
-- Columns: name (asset filename), tags (space-separated tag names),
-- description (highest-priority caption annotation).
-- Non-contentless so rows can be deleted by rowid without tracking originals.
-- Six triggers keep the index in sync: three on assets (name changes), two on
-- asset_tags (tag attach/detach), and three on annotations (caption upsert).
-- ---------------------------------------------------------------------------

CREATE VIRTUAL TABLE IF NOT EXISTS assets_fts USING fts5(
    name,
    tags,
    description,
    tokenize='unicode61'
);

-- Helper expressions used by multiple triggers (duplicated because SQLite
-- triggers do not support shared functions or CTEs across trigger bodies):
--   tags  = space-separated tag names via GROUP_CONCAT
--   desc  = highest-priority caption (manual > ai > extracted), LIMIT 1

-- ---- assets triggers ----

CREATE TRIGGER IF NOT EXISTS trg_assets_ai AFTER INSERT ON assets BEGIN
    INSERT INTO assets_fts(rowid, name, tags, description)
    VALUES (new.rowid, new.name, '', '');
END;

CREATE TRIGGER IF NOT EXISTS trg_assets_au AFTER UPDATE ON assets BEGIN
    DELETE FROM assets_fts WHERE rowid = old.rowid;
    INSERT INTO assets_fts(rowid, name, tags, description)
    VALUES (
        new.rowid,
        new.name,
        COALESCE((SELECT GROUP_CONCAT(t.name, ' ') FROM asset_tags at2 JOIN tags t ON t.id = at2.tag_id WHERE at2.asset_id = new.id), ''),
        COALESCE((SELECT ann.value FROM annotations ann WHERE ann.asset_id = new.id AND ann."key" = 'caption' ORDER BY CASE ann.layer WHEN 'manual' THEN 1 WHEN 'ai' THEN 2 ELSE 3 END LIMIT 1), '')
    );
END;

CREATE TRIGGER IF NOT EXISTS trg_assets_ad AFTER DELETE ON assets BEGIN
    DELETE FROM assets_fts WHERE rowid = old.rowid;
END;

-- ---- asset_tags triggers ----

CREATE TRIGGER IF NOT EXISTS trg_asset_tags_ai AFTER INSERT ON asset_tags BEGIN
    DELETE FROM assets_fts WHERE rowid = (SELECT rowid FROM assets WHERE id = new.asset_id);
    INSERT INTO assets_fts(rowid, name, tags, description)
    SELECT a.rowid, a.name,
        COALESCE((SELECT GROUP_CONCAT(t.name, ' ') FROM asset_tags at2 JOIN tags t ON t.id = at2.tag_id WHERE at2.asset_id = a.id), ''),
        COALESCE((SELECT ann.value FROM annotations ann WHERE ann.asset_id = a.id AND ann."key" = 'caption' ORDER BY CASE ann.layer WHEN 'manual' THEN 1 WHEN 'ai' THEN 2 ELSE 3 END LIMIT 1), '')
    FROM assets a WHERE a.id = new.asset_id;
END;

CREATE TRIGGER IF NOT EXISTS trg_asset_tags_ad AFTER DELETE ON asset_tags BEGIN
    DELETE FROM assets_fts WHERE rowid = (SELECT rowid FROM assets WHERE id = old.asset_id);
    INSERT INTO assets_fts(rowid, name, tags, description)
    SELECT a.rowid, a.name,
        COALESCE((SELECT GROUP_CONCAT(t.name, ' ') FROM asset_tags at2 JOIN tags t ON t.id = at2.tag_id WHERE at2.asset_id = a.id), ''),
        COALESCE((SELECT ann.value FROM annotations ann WHERE ann.asset_id = a.id AND ann."key" = 'caption' ORDER BY CASE ann.layer WHEN 'manual' THEN 1 WHEN 'ai' THEN 2 ELSE 3 END LIMIT 1), '')
    FROM assets a WHERE a.id = old.asset_id;
END;

-- ---- annotations triggers (caption only) ----

CREATE TRIGGER IF NOT EXISTS trg_annotations_ai_caption AFTER INSERT ON annotations
WHEN new."key" = 'caption' BEGIN
    DELETE FROM assets_fts WHERE rowid = (SELECT rowid FROM assets WHERE id = new.asset_id);
    INSERT INTO assets_fts(rowid, name, tags, description)
    SELECT a.rowid, a.name,
        COALESCE((SELECT GROUP_CONCAT(t.name, ' ') FROM asset_tags at2 JOIN tags t ON t.id = at2.tag_id WHERE at2.asset_id = a.id), ''),
        COALESCE((SELECT ann.value FROM annotations ann WHERE ann.asset_id = a.id AND ann."key" = 'caption' ORDER BY CASE ann.layer WHEN 'manual' THEN 1 WHEN 'ai' THEN 2 ELSE 3 END LIMIT 1), '')
    FROM assets a WHERE a.id = new.asset_id;
END;

CREATE TRIGGER IF NOT EXISTS trg_annotations_au_caption AFTER UPDATE ON annotations
WHEN new."key" = 'caption' BEGIN
    DELETE FROM assets_fts WHERE rowid = (SELECT rowid FROM assets WHERE id = new.asset_id);
    INSERT INTO assets_fts(rowid, name, tags, description)
    SELECT a.rowid, a.name,
        COALESCE((SELECT GROUP_CONCAT(t.name, ' ') FROM asset_tags at2 JOIN tags t ON t.id = at2.tag_id WHERE at2.asset_id = a.id), ''),
        COALESCE((SELECT ann.value FROM annotations ann WHERE ann.asset_id = a.id AND ann."key" = 'caption' ORDER BY CASE ann.layer WHEN 'manual' THEN 1 WHEN 'ai' THEN 2 ELSE 3 END LIMIT 1), '')
    FROM assets a WHERE a.id = new.asset_id;
END;

CREATE TRIGGER IF NOT EXISTS trg_annotations_ad_caption AFTER DELETE ON annotations
WHEN old."key" = 'caption' BEGIN
    DELETE FROM assets_fts WHERE rowid = (SELECT rowid FROM assets WHERE id = old.asset_id);
    INSERT INTO assets_fts(rowid, name, tags, description)
    SELECT a.rowid, a.name,
        COALESCE((SELECT GROUP_CONCAT(t.name, ' ') FROM asset_tags at2 JOIN tags t ON t.id = at2.tag_id WHERE at2.asset_id = a.id), ''),
        COALESCE((SELECT ann.value FROM annotations ann WHERE ann.asset_id = a.id AND ann."key" = 'caption' ORDER BY CASE ann.layer WHEN 'manual' THEN 1 WHEN 'ai' THEN 2 ELSE 3 END LIMIT 1), '')
    FROM assets a WHERE a.id = old.asset_id;
END;

-- shares are shareable links to an asset/folder/tag with optional expiry,
-- password protection, and permission. token is the URL-facing secret and is
-- globally unique; expires_at NULL means the link never expires.
CREATE TABLE IF NOT EXISTS shares (
    id            TEXT PRIMARY KEY,
    owner_id      TEXT NOT NULL,
    target_type   TEXT NOT NULL,               -- 'asset' / 'folder' / 'tag'
    target_id     TEXT NOT NULL,
    token         TEXT NOT NULL,               -- URL token
    expires_at    INTEGER,                     -- NULL = never expires
    password_hash TEXT NOT NULL DEFAULT '',    -- empty = no password
    permission    TEXT NOT NULL DEFAULT 'read',
    created_at    INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_shares_token ON shares (token);
CREATE INDEX IF NOT EXISTS idx_shares_owner ON shares (owner_id);

-- jobs is the persisted background-task queue (thumbnail generation, AI
-- tagging, 3D render, ...). This table is persistence only; execution is owned
-- by the job runtime (kernel.JobQueue and issues #8/#9).
CREATE TABLE IF NOT EXISTS jobs (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL,
    type       TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'pending', -- 'pending'/'running'/'done'/'failed'
    payload    TEXT NOT NULL DEFAULT '',        -- JSON job parameters
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_jobs_owner ON jobs (owner_id);
CREATE INDEX IF NOT EXISTS idx_jobs_owner_status ON jobs (owner_id, status);

-- embeddings stores CLIP vector embeddings as float32 BLOB for semantic search
-- and visual similarity. Vector similarity is computed in Go (brute-force
-- cosine over L2-normalized vectors); no C extension required.
CREATE TABLE IF NOT EXISTS embeddings (
    asset_id   TEXT PRIMARY KEY,
    embedding  BLOB NOT NULL,       -- float32 array, little-endian
    model      TEXT NOT NULL,       -- producing model, e.g. "openai/clip-vit-base-patch32"
    created_at INTEGER NOT NULL     -- unix seconds
);
