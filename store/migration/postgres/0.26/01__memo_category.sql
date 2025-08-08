ALTER TABLE memo ADD COLUMN category_id INTEGER REFERENCES category(id) ON DELETE SET NULL;

CREATE INDEX idx_memo_category ON memo (category_id);