ALTER TABLE memo ADD COLUMN category_id INT;

ALTER TABLE memo ADD CONSTRAINT fk_memo_category FOREIGN KEY (category_id) REFERENCES category(id) ON DELETE SET NULL;

CREATE INDEX idx_memo_category ON memo (category_id);