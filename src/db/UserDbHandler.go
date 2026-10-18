package db

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrNotYourCharacter means the character isn't linked to the signed-in user.
var ErrNotYourCharacter = errors.New("that character is not linked to your account")

// UserCharacter is a character linked to a user.
type UserCharacter struct {
	CharacterID   int64
	CharacterName string
	AddedAt       time.Time
	Authorized    bool // has a saved token, so the hourly refresh reads it
}

// LinkCharacter links a character to a user after they sign in with it and
// returns that user's ID. A userID of 0 is a plain sign-in: the character
// keeps the user it already has, or gets a new user of its own. Otherwise the
// character is added to userID as an alt, moving it from any user it was
// linked to before; a user left with no characters is deleted.
func LinkCharacter(ctx context.Context, characterID int64, characterName string, userID int64) (int64, error) {
	conn, err := connectDB()
	if err != nil {
		return 0, err
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(context.Background())

	var currentUserID int64
	err = tx.QueryRow(ctx,
		`SELECT user_id FROM lifetime_indy.user_characters WHERE character_id = $1 FOR UPDATE`,
		characterID).Scan(&currentUserID)
	linked := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}

	switch {
	case userID == 0 && linked:
		_, err = tx.Exec(ctx,
			`UPDATE lifetime_indy.user_characters SET character_name = $2 WHERE character_id = $1`,
			characterID, characterName)
		userID = currentUserID
	case userID == 0:
		if err := tx.QueryRow(ctx, `INSERT INTO lifetime_indy.users DEFAULT VALUES RETURNING user_id`).Scan(&userID); err != nil {
			return 0, err
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO lifetime_indy.user_characters (character_id, user_id, character_name) VALUES ($1, $2, $3)`,
			characterID, userID, characterName)
	case linked && currentUserID == userID:
		_, err = tx.Exec(ctx,
			`UPDATE lifetime_indy.user_characters SET character_name = $2 WHERE character_id = $1`,
			characterID, characterName)
	default:
		_, err = tx.Exec(ctx,
			`INSERT INTO lifetime_indy.user_characters (character_id, user_id, character_name) VALUES ($1, $2, $3)
			ON CONFLICT (character_id) DO UPDATE SET user_id = $2, character_name = $3, added_at = now()`,
			characterID, userID, characterName)
		if err == nil && linked {
			err = deleteUserIfEmpty(ctx, tx, currentUserID)
		}
	}
	if err != nil {
		return 0, err
	}

	return userID, tx.Commit(ctx)
}

// ListUserCharacters returns the user's characters: the one they first signed in with, then the rest by name.
func ListUserCharacters(ctx context.Context, userID int64) ([]UserCharacter, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}

	rows, err := conn.Query(ctx,
		`SELECT c.character_id, c.character_name, c.added_at, t.character_id IS NOT NULL
		FROM lifetime_indy.user_characters c
		LEFT JOIN lifetime_indy.esi_tokens t ON t.character_id = c.character_id
		WHERE c.user_id = $1`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	characters := make([]UserCharacter, 0)
	for rows.Next() {
		var character UserCharacter
		if err := rows.Scan(&character.CharacterID, &character.CharacterName, &character.AddedAt, &character.Authorized); err != nil {
			return nil, err
		}
		characters = append(characters, character)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sortMainFirst(characters)
	return characters, nil
}

// sortMainFirst puts the earliest added character first and the others in
// name order.
func sortMainFirst(characters []UserCharacter) {
	if len(characters) == 0 {
		return
	}
	main := 0
	for i, character := range characters {
		if character.AddedAt.Before(characters[main].AddedAt) {
			main = i
		}
	}
	characters[0], characters[main] = characters[main], characters[0]

	alts := characters[1:]
	sort.Slice(alts, func(i, j int) bool {
		return alts[i].CharacterName < alts[j].CharacterName
	})
}

func deleteUserIfEmpty(ctx context.Context, tx pgx.Tx, userID int64) error {
	_, err := tx.Exec(ctx,
		`DELETE FROM lifetime_indy.users u WHERE user_id = $1
		AND NOT EXISTS (SELECT 1 FROM lifetime_indy.user_characters c WHERE c.user_id = u.user_id)`,
		userID)
	return err
}
