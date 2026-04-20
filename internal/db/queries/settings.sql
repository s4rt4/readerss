-- name: GetReaderSettings :one
SELECT user_id, default_fetch_interval_minutes, retention_days, theme, density,
       respect_cache_headers, updated_at
FROM reader_settings
WHERE user_id = ?;

-- name: UpsertReaderSettings :one
INSERT INTO reader_settings (
    user_id, default_fetch_interval_minutes, retention_days, theme, density,
    respect_cache_headers
) VALUES (
    ?, ?, ?, ?, ?, ?
)
ON CONFLICT(user_id) DO UPDATE SET
    default_fetch_interval_minutes = excluded.default_fetch_interval_minutes,
    retention_days = excluded.retention_days,
    theme = excluded.theme,
    density = excluded.density,
    respect_cache_headers = excluded.respect_cache_headers,
    updated_at = CURRENT_TIMESTAMP
RETURNING user_id, default_fetch_interval_minutes, retention_days, theme, density,
          respect_cache_headers, updated_at;
