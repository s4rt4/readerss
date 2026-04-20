-- +goose Up
CREATE TABLE IF NOT EXISTS filter_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    feed_id INTEGER REFERENCES feeds(id) ON DELETE CASCADE,
    match_type TEXT NOT NULL,
    pattern TEXT NOT NULL,
    action TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_filter_rules_user_feed ON filter_rules(user_id, feed_id);

-- +goose Down
DROP INDEX IF EXISTS idx_filter_rules_user_feed;
DROP TABLE IF EXISTS filter_rules;
