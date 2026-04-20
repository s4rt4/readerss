-- +goose Up
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE feeds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    url TEXT NOT NULL,
    site_url TEXT,
    title TEXT NOT NULL,
    description TEXT,
    icon_url TEXT,
    etag TEXT,
    last_modified TEXT,
    last_fetched_at DATETIME,
    last_error TEXT,
    error_count INTEGER NOT NULL DEFAULT 0,
    fetch_interval_minutes INTEGER NOT NULL DEFAULT 60,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, url)
);

CREATE TABLE articles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    feed_id INTEGER NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
    guid TEXT NOT NULL,
    url TEXT NOT NULL,
    title TEXT NOT NULL,
    author TEXT,
    content TEXT,
    excerpt TEXT,
    published_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    is_read INTEGER NOT NULL DEFAULT 0,
    is_starred INTEGER NOT NULL DEFAULT 0,
    UNIQUE(feed_id, guid)
);

CREATE INDEX idx_categories_user_sort ON categories(user_id, sort_order, name);
CREATE INDEX idx_feeds_user_title ON feeds(user_id, title);
CREATE INDEX idx_articles_feed_published ON articles(feed_id, published_at DESC);
CREATE INDEX idx_articles_unread ON articles(is_read, published_at DESC) WHERE is_read = 0;
CREATE INDEX idx_articles_starred ON articles(is_starred, published_at DESC) WHERE is_starred = 1;

CREATE VIRTUAL TABLE articles_fts USING fts5(
    title,
    content,
    author,
    content='articles',
    content_rowid='id',
    tokenize='porter unicode61'
);

-- +goose StatementBegin
CREATE TRIGGER articles_ai AFTER INSERT ON articles BEGIN
    INSERT INTO articles_fts(rowid, title, content, author)
    VALUES (new.id, new.title, new.content, new.author);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER articles_ad AFTER DELETE ON articles BEGIN
    INSERT INTO articles_fts(articles_fts, rowid, title, content, author)
    VALUES ('delete', old.id, old.title, old.content, old.author);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER articles_au AFTER UPDATE ON articles BEGIN
    INSERT INTO articles_fts(articles_fts, rowid, title, content, author)
    VALUES ('delete', old.id, old.title, old.content, old.author);
    INSERT INTO articles_fts(rowid, title, content, author)
    VALUES (new.id, new.title, new.content, new.author);
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS articles_au;
DROP TRIGGER IF EXISTS articles_ad;
DROP TRIGGER IF EXISTS articles_ai;
DROP TABLE IF EXISTS articles_fts;
DROP TABLE IF EXISTS articles;
DROP TABLE IF EXISTS feeds;
DROP TABLE IF EXISTS categories;
DROP TABLE IF EXISTS users;
