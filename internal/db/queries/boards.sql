-- name: ListBoards :many
SELECT
    b.id,
    b.user_id,
    b.name,
    b.description,
    b.created_at,
    COUNT(ba.article_id) AS article_count
FROM boards b
LEFT JOIN board_articles ba ON ba.board_id = b.id
WHERE b.user_id = ?
GROUP BY b.id
ORDER BY b.name;

-- name: GetBoard :one
SELECT id, user_id, name, description, created_at
FROM boards
WHERE id = ? AND user_id = ?;

-- name: CreateBoard :one
INSERT INTO boards (user_id, name, description)
VALUES (?, ?, ?)
ON CONFLICT(user_id, name) DO UPDATE SET description = excluded.description
RETURNING id, user_id, name, description, created_at;

-- name: DeleteBoard :exec
DELETE FROM boards
WHERE id = ? AND user_id = ?;

-- name: AddArticleToBoard :exec
INSERT OR IGNORE INTO board_articles (board_id, article_id)
SELECT sqlc.arg(board_id), sqlc.arg(article_id)
WHERE EXISTS (SELECT 1 FROM boards b WHERE b.id = sqlc.arg(board_id) AND b.user_id = sqlc.arg(owner_id))
  AND EXISTS (
      SELECT 1
      FROM articles a
      JOIN feeds f ON f.id = a.feed_id
      WHERE a.id = sqlc.arg(article_id) AND f.user_id = sqlc.arg(owner_id)
  );

-- name: RemoveArticleFromBoard :exec
DELETE FROM board_articles
WHERE board_id = ?
  AND article_id = ?
  AND board_id IN (SELECT id FROM boards WHERE user_id = ?);

-- name: ListBoardArticles :many
SELECT
    a.id,
    a.feed_id,
    a.guid,
    a.url,
    a.title,
    a.author,
    a.content,
    a.excerpt,
    a.image_url,
    a.published_at,
    a.created_at,
    a.is_read,
    a.is_starred,
    a.is_read_later,
    f.title AS feed_title,
    COALESCE(c.name, 'Uncategorized') AS category
FROM board_articles ba
JOIN boards b ON b.id = ba.board_id
JOIN articles a ON a.id = ba.article_id
JOIN feeds f ON f.id = a.feed_id
LEFT JOIN categories c ON c.id = f.category_id
WHERE b.id = ? AND b.user_id = ?
ORDER BY ba.created_at DESC, a.id DESC
LIMIT ? OFFSET ?;
