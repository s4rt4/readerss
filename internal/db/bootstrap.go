package db

import (
	"context"
	"database/sql"
	"fmt"
)

const DefaultUsername = "sarta"

func EnsureDefaultData(ctx context.Context, conn *sql.DB) (int64, error) {
	if _, err := conn.ExecContext(ctx, `
INSERT INTO users (username, password_hash)
VALUES (?, ?)
ON CONFLICT(username) DO NOTHING
`, DefaultUsername, "local-dev-password-placeholder"); err != nil {
		return 0, fmt.Errorf("ensure default user: %w", err)
	}

	var userID int64
	if err := conn.QueryRowContext(ctx, `SELECT id FROM users WHERE username = ?`, DefaultUsername).Scan(&userID); err != nil {
		return 0, fmt.Errorf("load default user: %w", err)
	}

	defaultCategories := []string{"Engineering", "Product", "Indie Web"}
	for i, name := range defaultCategories {
		if _, err := conn.ExecContext(ctx, `
INSERT INTO categories (user_id, name, sort_order)
SELECT ?, ?, ?
WHERE NOT EXISTS (
    SELECT 1 FROM categories WHERE user_id = ? AND name = ?
)
`, userID, name, i+1, userID, name); err != nil {
			return 0, fmt.Errorf("ensure category %q: %w", name, err)
		}
	}

	return userID, nil
}
