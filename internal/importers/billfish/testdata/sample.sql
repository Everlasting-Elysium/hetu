-- Billfish sample library schema + data, mirroring a real .bf/billfish.db.
-- Rebuild the fixture DB with:
--   sqlite3 internal/importers/billfish/testdata/sample/.bf/billfish.db < internal/importers/billfish/testdata/sample.sql
--
-- Column shapes verified against real Billfish exporters (zyx0814/FilePress):
--   bf_file f LEFT JOIN bf_material_v2 m ON m.file_id=f.id
--            LEFT JOIN bf_material_userdata mu ON mu.file_id=f.id
--   selecting f.*, m.w, m.h, m.colors, mu.note, mu.score, mu.origin.
-- Folder membership is bf_file.pid -> bf_folder.id; tag membership is the
-- bf_tag_join_file (file_id, tag_id) table; tag/folder hierarchy is the pid col.

CREATE TABLE bf_folder (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    pid         INTEGER NOT NULL DEFAULT 0,   -- parent folder id; 0 = root
    seq         INTEGER NOT NULL DEFAULT 0
);
INSERT INTO bf_folder (id, name, pid, seq) VALUES
    (1, 'Nature',   0, 0),
    (2, 'Work',     0, 1),
    (3, 'Diagrams', 2, 0);   -- child of Work

CREATE TABLE bf_file (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL,
    ext          TEXT NOT NULL,
    path         TEXT NOT NULL,               -- relative to the library root
    pid          INTEGER NOT NULL DEFAULT 0,  -- folder id (bf_folder.id)
    create_time  INTEGER NOT NULL,            -- unix seconds
    modify_time  INTEGER NOT NULL,
    import_time  INTEGER NOT NULL
);
INSERT INTO bf_file (id, name, ext, path, pid, create_time, modify_time, import_time) VALUES
    (10, 'photo',   'png', 'files/photo.png',       1, 1700000000, 1700000050, 1700000060),
    (11, 'diagram', 'png', 'files/sub/diagram.png', 3, 1700001000, 1700001000, 1700001010);

CREATE TABLE bf_material_v2 (
    file_id     INTEGER PRIMARY KEY,
    w           INTEGER NOT NULL DEFAULT 0,
    h           INTEGER NOT NULL DEFAULT 0,
    is_recycle  INTEGER NOT NULL DEFAULT 0,   -- 1 = in trash
    thumb_tid   INTEGER NOT NULL DEFAULT 0,
    colors      TEXT NOT NULL DEFAULT ''
);
INSERT INTO bf_material_v2 (file_id, w, h, is_recycle, thumb_tid, colors) VALUES
    (10, 88, 72, 0, 0, '[[59,130,232]]'),
    (11, 96, 80, 0, 0, '[[76,175,80]]');

CREATE TABLE bf_material_userdata (
    file_id          INTEGER PRIMARY KEY,
    comments_detail  TEXT NOT NULL DEFAULT '',
    note             TEXT NOT NULL DEFAULT '',
    score            INTEGER NOT NULL DEFAULT 0,   -- rating 0-5
    origin           TEXT NOT NULL DEFAULT ''      -- source URL
);
INSERT INTO bf_material_userdata (file_id, comments_detail, note, score, origin) VALUES
    (10, '', 'Mountain vista at dawn', 4, 'https://source.example.com/photo'),
    (11, '', 'Architecture sketch',    2, '');

CREATE TABLE bf_tag_v2 (
    id    INTEGER PRIMARY KEY,
    name  TEXT NOT NULL,
    pid   INTEGER NOT NULL DEFAULT 0,   -- parent tag id; 0 = root
    seq   INTEGER NOT NULL DEFAULT 0
);
INSERT INTO bf_tag_v2 (id, name, pid, seq) VALUES
    (100, 'landscape', 0,   0),
    (101, 'mountain',  100, 0),   -- child of landscape
    (102, 'work',      0,   1);

CREATE TABLE bf_tag_join_file (
    file_id  INTEGER NOT NULL,
    tag_id   INTEGER NOT NULL,
    PRIMARY KEY (file_id, tag_id)
);
INSERT INTO bf_tag_join_file (file_id, tag_id) VALUES
    (10, 100),
    (10, 101),
    (11, 102);
