package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// DeleteCharacterToken deletes the character's saved token, so the hourly
// refresh stops reading it, and returns its refresh token so it can be
// revoked with EVE SSO. It returns "" when there was no token or it couldn't
// be decrypted.
func DeleteCharacterToken(ctx context.Context, characterID int64) (string, error) {
	conn, err := connectDB()
	if err != nil {
		return "", err
	}
	var sealed []byte
	err = conn.QueryRow(ctx,
		`DELETE FROM lifetime_indy.esi_tokens WHERE character_id = $1 RETURNING refresh_token`,
		characterID).Scan(&sealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	refreshToken, err := decryptToken(sealed)
	if err != nil {
		return "", nil
	}
	return refreshToken, nil
}

// characterTables are the tables holding a character's own data, and the
// column naming the character.
var characterTables = []struct{ table, column string }{
	{"character_wallet_journal", "character_id"},
	{"wallet_balances", "owner_id"},
	{"wallet_transactions", "owner_id"},
	{"loyalty_points", "character_id"},
	{"market_orders", "owner_id"},
	{"assets", "owner_id"},
	{"blueprints", "owner_id"},
	{"industry_jobs", "owner_id"},
	{"delivered_jobs", "installer_id"},
	{"mining_ledger", "character_id"},
	{"networth_snapshots", "owner_id"},
}

// corporationTables are the tables holding a corporation's data, and the
// column naming the corporation.
var corporationTables = []struct{ table, column string }{
	{"corp_wallet_journal", "corporation_id"},
	{"wallet_balances", "owner_id"},
	{"wallet_transactions", "owner_id"},
	{"market_orders", "owner_id"},
	{"assets", "owner_id"},
	{"blueprints", "owner_id"},
	{"industry_jobs", "owner_id"},
	{"corp_mining_ledger", "corporation_id"},
	{"networth_snapshots", "owner_id"},
}

// DeleteCharacterData deletes everything saved for the characters, unlinks
// them from their users (deleting a user left with no characters, and their
// stockpiles with them) and deletes the data of their corporations that no
// other authorized character is in, since nobody can read those any more.
// Their tokens should be deleted first.
func DeleteCharacterData(ctx context.Context, characterIDs []int64) error {
	conn, err := connectDB()
	if err != nil {
		return err
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	// player corporations of the characters that no remaining authorized
	// character is in
	rows, err := tx.Query(ctx,
		`SELECT DISTINCT c.corporation_id FROM lifetime_indy.user_characters c
		WHERE c.character_id = ANY($1) AND c.corporation_id >= 2000000
		AND NOT EXISTS (
			SELECT 1 FROM lifetime_indy.user_characters o
			JOIN lifetime_indy.esi_tokens t ON t.character_id = o.character_id
			WHERE o.corporation_id = c.corporation_id AND NOT o.character_id = ANY($1))`,
		characterIDs)
	if err != nil {
		return err
	}
	corporationIDs, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return err
	}

	for _, t := range characterTables {
		if _, err := tx.Exec(ctx, `DELETE FROM lifetime_indy.`+t.table+` WHERE `+t.column+` = ANY($1)`, characterIDs); err != nil {
			return err
		}
	}
	for _, t := range corporationTables {
		if _, err := tx.Exec(ctx, `DELETE FROM lifetime_indy.`+t.table+` WHERE `+t.column+` = ANY($1)`, corporationIDs); err != nil {
			return err
		}
	}

	rows, err = tx.Query(ctx,
		`DELETE FROM lifetime_indy.user_characters WHERE character_id = ANY($1) RETURNING user_id`, characterIDs)
	if err != nil {
		return err
	}
	userIDs, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return err
	}
	for _, userID := range userIDs {
		if err := deleteUserIfEmpty(ctx, tx, userID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
