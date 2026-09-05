-- hetu schema (v0).
--
-- Conventions:
--   * Timestamps are unix seconds (INTEGER) for driver-agnostic scanning.
--   * owner_id is reserved on every table so multi-user is additive later.
--
-- Only the tables the Phase 0 scan->index->list chain exercises live here.
-- The full target data model (folders, tags, asset_tags, annotations with
-- layered metadata, shares, jobs, plus Phase 1 FTS5 + vector tables) is
-- described in docs/data-model.md and added to this file when implemented.

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
