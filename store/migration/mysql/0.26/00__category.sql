CREATE TABLE category (
    id INT PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(100) NOT NULL,
    path VARCHAR(500) NOT NULL,
    parent_id INT,
    creator_id INT NOT NULL,
    color VARCHAR(7) NOT NULL DEFAULT '#6366f1',
    icon VARCHAR(20) NOT NULL DEFAULT '📁',
    created_ts BIGINT NOT NULL DEFAULT (UNIX_TIMESTAMP()),
    updated_ts BIGINT NOT NULL DEFAULT (UNIX_TIMESTAMP()),
    row_status VARCHAR(20) NOT NULL DEFAULT 'NORMAL',
    
    CONSTRAINT chk_category_name_length CHECK (CHAR_LENGTH(name) <= 100 AND CHAR_LENGTH(name) > 0),
    CONSTRAINT chk_category_path_length CHECK (CHAR_LENGTH(path) <= 500 AND CHAR_LENGTH(path) > 0),
    CONSTRAINT chk_category_color_format CHECK (color REGEXP '^#[0-9A-Fa-f]{6}$'),
    CONSTRAINT chk_category_icon_length CHECK (CHAR_LENGTH(icon) <= 20),
    CONSTRAINT chk_category_row_status CHECK (row_status IN ('NORMAL', 'ARCHIVED')),
    
    CONSTRAINT fk_category_parent FOREIGN KEY (parent_id) REFERENCES category(id) ON DELETE CASCADE,
    CONSTRAINT fk_category_creator FOREIGN KEY (creator_id) REFERENCES user(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_category_path_creator ON category (path, creator_id, row_status);
CREATE INDEX idx_category_creator ON category (creator_id);
CREATE INDEX idx_category_parent ON category (parent_id);
CREATE INDEX idx_category_status ON category (row_status);

CREATE TRIGGER update_category_updated_ts
    BEFORE UPDATE ON category
    FOR EACH ROW
    SET NEW.updated_ts = UNIX_TIMESTAMP();