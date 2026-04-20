-- name: ListFeeds :many
SELECT id, user_id, category_id, url, site_url, title, description, icon_url,
       etag, last_modified, last_fetched_at, last_error, error_count,
       fetch_interval_minutes, created_at
FROM feeds
WHERE user_id = ?
ORDER BY title;

-- name: CreateFeed :one
INSERT INTO feeds (
    user_id, category_id, url, site_url, title, description, icon_url,
    fetch_interval_minutes
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING id, user_id, category_id, url, site_url, title, description, icon_url,
          etag, last_modified, last_fetched_at, last_error, error_count,
          fetch_interval_minutes, created_at;

-- name: GetFeed :one
SELECT id, user_id, category_id, url, site_url, title, description, icon_url,
       etag, last_modified, last_fetched_at, last_error, error_count,
       fetch_interval_minutes, created_at
FROM feeds
WHERE id = ? AND user_id = ?;

-- name: UpdateFeed :one
UPDATE feeds
SET category_id = ?,
    url = ?,
    site_url = ?,
    title = ?,
    description = ?,
    fetch_interval_minutes = ?
WHERE id = ? AND user_id = ?
RETURNING id, user_id, category_id, url, site_url, title, description, icon_url,
          etag, last_modified, last_fetched_at, last_error, error_count,
          fetch_interval_minutes, created_at;

-- name: DeleteFeed :exec
DELETE FROM feeds
WHERE id = ? AND user_id = ?;

-- name: UpdateFeedFetchSuccess :exec
UPDATE feeds
SET title = ?,
    site_url = ?,
    description = ?,
    etag = ?,
    last_modified = ?,
    last_fetched_at = CURRENT_TIMESTAMP,
    last_error = NULL,
    error_count = 0
WHERE id = ? AND user_id = ?;

-- name: UpdateFeedFetchError :exec
UPDATE feeds
SET last_fetched_at = CURRENT_TIMESTAMP,
    last_error = ?,
    error_count = error_count + 1
WHERE id = ? AND user_id = ?;
