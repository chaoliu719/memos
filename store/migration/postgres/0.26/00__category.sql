CREATE TABLE category (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL CHECK (LENGTH(name) <= 100 AND LENGTH(name) > 0),
    path VARCHAR(500) NOT NULL CHECK (LENGTH(path) <= 500 AND LENGTH(path) > 0),
    parent_id INTEGER REFERENCES category(id) ON DELETE CASCADE,
    creator_id INTEGER NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    color VARCHAR(7) NOT NULL DEFAULT '#6366f1' CHECK (color ~ '^#[0-9A-Fa-f]{6}$'),
    icon VARCHAR(20) NOT NULL DEFAULT '📁' CHECK (LENGTH(icon) <= 20),
    created_ts BIGINT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
    updated_ts BIGINT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
    row_status VARCHAR(20) NOT NULL DEFAULT 'NORMAL' CHECK (row_status IN ('NORMAL', 'ARCHIVED'))
);

CREATE UNIQUE INDEX idx_category_path_creator ON category (path, creator_id) WHERE row_status = 'NORMAL';
CREATE INDEX idx_category_creator ON category (creator_id);
CREATE INDEX idx_category_parent ON category (parent_id);
CREATE INDEX idx_category_status ON category (row_status);

CREATE OR REPLACE FUNCTION update_category_updated_ts()
    RETURNS TRIGGER AS $$
    BEGIN
        NEW.updated_ts = EXTRACT(EPOCH FROM NOW());
        RETURN NEW;
    END;
    $$ LANGUAGE plpgsql;

CREATE TRIGGER update_category_updated_ts
    BEFORE UPDATE ON category
    FOR EACH ROW
    EXECUTE FUNCTION update_category_updated_ts();