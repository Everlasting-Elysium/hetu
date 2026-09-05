-- hetu schema (v0).
--
-- Conventions:
--   * Timestamps are unix seconds (INTEGER) for driver-agnostic scanning.
--   * owner_id is reserved on every table so multi-user is additive later.
--
-- Tables: users, assets (Phase 0); assets_fts FTS5 (Phase 1).
-- The full target data model is described in docs/data-model.md.

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

-- ---------------------------------------------------------------------------
-- FTS5 full-text search index (contentless).
-- Columns: name (asset filename), tags (space-separated), description.
-- tags and description are empty until the tags/annotations tables land;
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
