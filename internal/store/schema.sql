-- hetu schema (v0).
--
-- Conventions:
--   * Timestamps are unix seconds (INTEGER) for driver-agnostic scanning.
--   * owner_id is reserved on every table so multi-user is additive later.
--
-- Tables are added here as features land. Implemented so far: users, assets,
-- annotations (layered metadata), and asset_colors (the color-search index).
-- The remaining target tables (folders, tags, asset_tags, shares, jobs, plus
-- Phase 1 FTS5 + vector tables) are described in docs/data-model.md and added
-- when implemented.

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
    indexed_at   INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_owner_path
    ON assets (owner_id, provider, storage_path);
CREATE INDEX IF NOT EXISTS idx_assets_owner ON assets (owner_id);

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
