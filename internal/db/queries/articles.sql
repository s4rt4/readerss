-- name: ListArticlesByFeed :many
SELECT id, feed_id, guid, url, title, author, content, excerpt, published_at,
       created_at, is_read, is_starred
FROM articles
WHERE feed_id = ?
ORDER BY published_at DESC, id DESC
LIMIT ? OFFSET ?;

-- name: ListRecentArticles :many
SELECT
    a.id,
    a.feed_id,
    a.guid,
    a.url,
    a.title,
    a.author,
    a.content,
    a.excerpt,
    a.published_at,
    a.created_at,
    a.is_read,
    a.is_starred,
    f.title AS feed_title,
    COALESCE(c.name, 'Uncategorized') AS category
FROM articles a
JOIN feeds f ON f.id = a.feed_id
LEFT JOIN categories c ON c.id = f.category_id
WHERE f.user_id = ?
ORDER BY COALESCE(a.published_at, a.created_at) DESC, a.id DESC
LIMIT ? OFFSET ?;

-- name: ListUnreadArticles :many
SELECT
    a.id,
    a.feed_id,
    a.guid,
    a.url,
    a.title,
    a.author,
    a.content,
    a.excerpt,
    a.published_at,
    a.created_at,
    a.is_read,
    a.is_starred,
    f.title AS feed_title,
    COALESCE(c.name, 'Uncategorized') AS category
FROM articles a
JOIN feeds f ON f.id = a.feed_id
LEFT JOIN categories c ON c.id = f.category_id
WHERE f.user_id = ? AND a.is_read = 0
ORDER BY COALESCE(a.published_at, a.created_at) DESC, a.id DESC
LIMIT ? OFFSET ?;

-- name: ListStarredArticles :many
SELECT
    a.id,
    a.feed_id,
    a.guid,
    a.url,
    a.title,
    a.author,
    a.content,
    a.excerpt,
    a.published_at,
    a.created_at,
    a.is_read,
    a.is_starred,
    f.title AS feed_title,
    COALESCE(c.name, 'Uncategorized') AS category
FROM articles a
JOIN feeds f ON f.id = a.feed_id
LEFT JOIN categories c ON c.id = f.category_id
WHERE f.user_id = ? AND a.is_starred = 1
ORDER BY COALESCE(a.published_at, a.created_at) DESC, a.id DESC
LIMIT ? OFFSET ?;

-- name: ListRecentArticlesByFeed :many
SELECT
    a.id,
    a.feed_id,
    a.guid,
    a.url,
    a.title,
    a.author,
    a.content,
    a.excerpt,
    a.published_at,
    a.created_at,
    a.is_read,
    a.is_starred,
    f.title AS feed_title,
    COALESCE(c.name, 'Uncategorized') AS category
FROM articles a
JOIN feeds f ON f.id = a.feed_id
LEFT JOIN categories c ON c.id = f.category_id
WHERE f.user_id = ? AND f.id = ?
ORDER BY COALESCE(a.published_at, a.created_at) DESC, a.id DESC
LIMIT ? OFFSET ?;

-- name: ListRecentArticlesByCategory :many
SELECT
    a.id,
    a.feed_id,
    a.guid,
    a.url,
    a.title,
    a.author,
    a.content,
    a.excerpt,
    a.published_at,
    a.created_at,
    a.is_read,
    a.is_starred,
    f.title AS feed_title,
    COALESCE(c.name, 'Uncategorized') AS category
FROM articles a
JOIN feeds f ON f.id = a.feed_id
LEFT JOIN categories c ON c.id = f.category_id
WHERE f.user_id = ? AND f.category_id = ?
ORDER BY COALESCE(a.published_at, a.created_at) DESC, a.id DESC
LIMIT ? OFFSET ?;

-- name: CountUnreadArticles :one
SELECT COUNT(*)
FROM articles a
JOIN feeds f ON f.id = a.feed_id
WHERE f.user_id = ? AND a.is_read = 0;

-- name: CountAllArticles :one
SELECT COUNT(*)
FROM articles a
JOIN feeds f ON f.id = a.feed_id
WHERE f.user_id = ?;

-- name: CountStarredArticles :one
SELECT COUNT(*)
FROM articles a
JOIN feeds f ON f.id = a.feed_id
WHERE f.user_id = ? AND a.is_starred = 1;

-- name: CountUnreadArticlesByFeed :many
SELECT feed_id, COUNT(*) AS unread_count
FROM articles
WHERE is_read = 0
  AND feed_id IN (SELECT id FROM feeds WHERE user_id = ?)
GROUP BY feed_id;

-- name: CountUnreadArticlesByCategory :many
SELECT f.category_id, COUNT(a.id) AS unread_count
FROM articles a
JOIN feeds f ON f.id = a.feed_id
WHERE f.user_id = ? AND a.is_read = 0
GROUP BY f.category_id;

-- name: CreateArticle :one
INSERT OR IGNORE INTO articles (
    feed_id, guid, url, title, author, content, excerpt, published_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING id, feed_id, guid, url, title, author, content, excerpt, published_at,
          created_at, is_read, is_starred;

-- name: MarkArticleRead :exec
UPDATE articles
SET is_read = ?
WHERE articles.id = ?
  AND articles.feed_id IN (SELECT feeds.id FROM feeds WHERE feeds.user_id = ?);

-- name: StarArticle :exec
UPDATE articles
SET is_starred = ?
WHERE articles.id = ?
  AND articles.feed_id IN (SELECT feeds.id FROM feeds WHERE feeds.user_id = ?);

-- name: DeleteArticle :exec
DELETE FROM articles
WHERE articles.id = ?
  AND articles.feed_id IN (SELECT feeds.id FROM feeds WHERE feeds.user_id = ?);

-- name: MarkAllArticlesRead :exec
UPDATE articles
SET is_read = 1
WHERE feed_id IN (SELECT id FROM feeds WHERE user_id = ?);

-- name: MarkFeedArticlesRead :exec
UPDATE articles
SET is_read = 1
WHERE feed_id = ?
  AND feed_id IN (SELECT id FROM feeds WHERE user_id = ?);

-- name: MarkCategoryArticlesRead :exec
UPDATE articles
SET is_read = 1
WHERE feed_id IN (
    SELECT id FROM feeds WHERE user_id = ? AND category_id = ?
);

-- name: DeleteReadArticlesOlderThan :exec
DELETE FROM articles
WHERE is_read = 1
  AND created_at < datetime('now', printf('-%d days', ?))
  AND feed_id IN (SELECT id FROM feeds WHERE user_id = ?);

-- name: SearchArticles :many
SELECT
    a.id,
    a.feed_id,
    a.url,
    a.title,
    a.excerpt,
    a.published_at,
    f.title AS feed_title,
    snippet(fts, 0, '<mark>', '</mark>', '...', 12) AS title_snippet,
    snippet(fts, 1, '<mark>', '</mark>', '...', 36) AS content_snippet
FROM articles_fts fts
JOIN articles a ON a.id = fts.rowid
JOIN feeds f ON f.id = a.feed_id
WHERE fts.content MATCH ?
  AND f.user_id = ?
ORDER BY bm25(fts)
LIMIT 50;
