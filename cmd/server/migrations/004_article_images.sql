-- +goose Up
ALTER TABLE articles ADD COLUMN image_url TEXT;

-- +goose Down
-- SQLite cannot drop a column without rebuilding the table. Keep this migration irreversible.
