-- +goose Up
CREATE TABLE reader_settings (
    user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    default_fetch_interval_minutes INTEGER NOT NULL DEFAULT 60,
    retention_days INTEGER NOT NULL DEFAULT 90,
    theme TEXT NOT NULL DEFAULT 'system',
    density TEXT NOT NULL DEFAULT 'balanced',
    respect_cache_headers INTEGER NOT NULL DEFAULT 1,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE filter_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    feed_id INTEGER REFERENCES feeds(id) ON DELETE CASCADE,
    match_type TEXT NOT NULL,
    pattern TEXT NOT NULL,
    action TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_filter_rules_user_feed ON filter_rules(user_id, feed_id);

-- +goose Down
DROP TABLE IF EXISTS filter_rules;
DROP TABLE IF EXISTS reader_settings;
