package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// LoadShoppingList returns the list the user saved on the Shopping List
// page, or "" when they have none saved.
func LoadShoppingList(ctx context.Context, userID int64) (string, error) {
	conn, err := connectDB()
	if err != nil {
		return "", err
	}
	var list string
	err = conn.QueryRow(ctx,
		`SELECT list FROM lifetime_indy.shopping_lists WHERE user_id = $1`, userID).Scan(&list)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return list, err
}

// SaveShoppingList saves the user's list, replacing any saved before.
func SaveShoppingList(ctx context.Context, userID int64, list string) error {
	conn, err := connectDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx,
		`INSERT INTO lifetime_indy.shopping_lists (user_id, list) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET list = excluded.list, saved_at = now()`, userID, list)
	return err
}

// ClearShoppingList deletes the user's saved list.
func ClearShoppingList(ctx context.Context, userID int64) error {
	conn, err := connectDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx, `DELETE FROM lifetime_indy.shopping_lists WHERE user_id = $1`, userID)
	return err
}
