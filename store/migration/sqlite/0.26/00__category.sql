CREATE TABLE category (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL CHECK (length(name) <= 100 AND length(name) > 0),
    path TEXT NOT NULL CHECK (length(path) <= 500 AND length(path) > 0),
    parent_id INTEGER REFERENCES category(id) ON DELETE CASCADE,
    creator_id INTEGER NOT NULL REFERENCES user(id) ON DELETE CASCADE,
    color TEXT NOT NULL DEFAULT '#6366f1' CHECK (color REGEXP '^#[0-9A-Fa-f]{6}$'),
    icon TEXT NOT NULL DEFAULT '📁' CHECK (length(icon) <= 20),
    created_ts BIGINT NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_ts BIGINT NOT NULL DEFAULT (strftime('%s', 'now')),
    row_status TEXT NOT NULL DEFAULT 'NORMAL' CHECK (row_status IN ('NORMAL', 'ARCHIVED'))
);

CREATE UNIQUE INDEX idx_category_path_creator ON category (path, creator_id) WHERE row_status = 'NORMAL';
CREATE INDEX idx_category_creator ON category (creator_id);
CREATE INDEX idx_category_parent ON category (parent_id);
CREATE INDEX idx_category_status ON category (row_status);

CREATE TRIGGER update_category_updated_ts 
    AFTER UPDATE ON category FOR EACH ROW
    BEGIN
        UPDATE category SET updated_ts = strftime('%s', 'now') WHERE id = NEW.id;
    END;