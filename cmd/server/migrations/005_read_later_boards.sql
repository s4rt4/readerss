-- +goose Up
ALTER TABLE articles ADD COLUMN is_read_later INTEGER NOT NULL DEFAULT 0;

CREATE TABLE boards (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, name)
);

CREATE TABLE board_articles (
    board_id INTEGER NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    article_id INTEGER NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(board_id, article_id)
);

CREATE INDEX idx_boards_user_name ON boards(user_id, name);
CREATE INDEX idx_board_articles_article ON board_articles(article_id);

-- +goose Down
DROP TABLE IF EXISTS board_articles;
DROP TABLE IF EXISTS boards;
-- SQLite cannot drop is_read_later without rebuilding articles.
