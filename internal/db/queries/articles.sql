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

-- name: CountUnreadArticles :one
SELECT COUNT(*)
FROM articles a
JOIN feeds f ON f.id = a.feed_id
WHERE f.user_id = ? AND a.is_read = 0;

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
WHERE id = ?;

-- name: StarArticle :exec
UPDATE articles
SET is_starred = ?
WHERE id = ?;

-- name: SearchArticles :many
SELECT
    a.id,
    a.feed_id,
    a.url,
    a.title,
    a.excerpt,
    a.published_at,
    f.title AS feed_title,
    a.title AS title_snippet,
    COALESCE(a.excerpt, '') AS content_snippet
FROM articles a
JOIN feeds f ON f.id = a.feed_id
WHERE f.user_id = ?
  AND (
    a.title LIKE '%' || ? || '%'
    OR a.content LIKE '%' || ? || '%'
    OR a.author LIKE '%' || ? || '%'
  )
ORDER BY a.published_at DESC, a.id DESC
LIMIT 50;
