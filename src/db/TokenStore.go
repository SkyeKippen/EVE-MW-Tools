package db

import (
	"MeadowWorksTools/src/esi"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrNoToken means the character has never signed in to the web app, so
// there is no token to call ESI with.
var ErrNoToken = errors.New("no saved token for this character; they need to sign in to the tracker")

// refresh a little early so a token doesn't expire mid-request
const tokenExpiryMargin = time.Minute

// TokenCharacter is a character with a saved token.
type TokenCharacter struct {
	CharacterID   int64
	CharacterName string
	Scopes        []string
}

// SaveCharacterToken stores or replaces a character's tokens. It matches
// auth.TokenSaver and is called after every sign-in.
func SaveCharacterToken(ctx context.Context, token esi.CharacterToken) error {
	accessToken, err := encryptToken(token.AccessToken)
	if err != nil {
		return err
	}
	refreshToken, err := encryptToken(token.RefreshToken)
	if err != nil {
		return err
	}

	conn, err := connectDB()
	if err != nil {
		return err
	}

	_, err = conn.Exec(ctx,
		`INSERT INTO lifetime_indy.esi_tokens
		(character_id, character_name, scopes, access_token, refresh_token, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (character_id) DO UPDATE
		SET character_name = $2, scopes = $3, access_token = $4, refresh_token = $5, expires_at = $6, updated_at = now()`,
		token.CharacterID, token.CharacterName, strings.Join(token.Scopes, " "), accessToken, refreshToken, token.ExpiresAt)
	return err
}

// GetAccessToken returns a usable access token for the character, refreshing
// it through EVE SSO and saving the new pair when the stored one has expired.
// The row is locked while refreshing, since EVE may rotate the refresh token
// and two refreshes racing would leave one of them holding a dead token.
func GetAccessToken(ctx context.Context, cfg *esi.Config, characterID int64) (string, error) {
	conn, err := connectDB()
	if err != nil {
		return "", err
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(context.Background())

	var encAccess, encRefresh []byte
	var expiresAt time.Time
	err = tx.QueryRow(ctx,
		`SELECT access_token, refresh_token, expires_at FROM lifetime_indy.esi_tokens
		WHERE character_id = $1 FOR UPDATE`, characterID).Scan(&encAccess, &encRefresh, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoToken
	}
	if err != nil {
		return "", err
	}

	if time.Now().Add(tokenExpiryMargin).Before(expiresAt) {
		return decryptToken(encAccess)
	}

	refreshToken, err := decryptToken(encRefresh)
	if err != nil {
		return "", err
	}

	tok, err := cfg.RefreshAccessToken(ctx, refreshToken)
	if err != nil {
		return "", fmt.Errorf("refreshing token for character %d (they may need to sign in again): %w", characterID, err)
	}
	if tok.RefreshToken != "" {
		refreshToken = tok.RefreshToken
	}

	newAccess, err := encryptToken(tok.AccessToken)
	if err != nil {
		return "", err
	}
	newRefresh, err := encryptToken(refreshToken)
	if err != nil {
		return "", err
	}

	_, err = tx.Exec(ctx,
		`UPDATE lifetime_indy.esi_tokens
		SET access_token = $2, refresh_token = $3, expires_at = $4, updated_at = now()
		WHERE character_id = $1`,
		characterID, newAccess, newRefresh, tok.ExpiresAt)
	if err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

// ListTokenCharacters returns every character with a saved token.
func ListTokenCharacters(ctx context.Context) ([]TokenCharacter, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}

	rows, err := conn.Query(ctx,
		`SELECT character_id, character_name, scopes FROM lifetime_indy.esi_tokens ORDER BY character_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	characters := make([]TokenCharacter, 0)
	for rows.Next() {
		var character TokenCharacter
		var scopes string
		if err := rows.Scan(&character.CharacterID, &character.CharacterName, &scopes); err != nil {
			return nil, err
		}
		character.Scopes = strings.Fields(scopes)
		characters = append(characters, character)
	}
	return characters, rows.Err()
}

// HasScope reports whether the character granted scope at their last sign-in.
func (c TokenCharacter) HasScope(scope string) bool {
	return slices.Contains(c.Scopes, scope)
}

// LoadTokenCharacter returns one character with a saved token, or ErrNoToken.
func LoadTokenCharacter(ctx context.Context, characterID int64) (TokenCharacter, error) {
	conn, err := connectDB()
	if err != nil {
		return TokenCharacter{}, err
	}
	character := TokenCharacter{CharacterID: characterID}
	var scopes string
	err = conn.QueryRow(ctx,
		`SELECT character_name, scopes FROM lifetime_indy.esi_tokens WHERE character_id = $1`,
		characterID).Scan(&character.CharacterName, &scopes)
	if errors.Is(err, pgx.ErrNoRows) {
		return TokenCharacter{}, ErrNoToken
	}
	character.Scopes = strings.Fields(scopes)
	return character, err
}

// tokenCipher builds the AES-256-GCM cipher from TOKEN_ENCRYPTION_KEY, a
// base64-encoded 32 byte key.
func tokenCipher() (cipher.AEAD, error) {
	rawKey := os.Getenv("TOKEN_ENCRYPTION_KEY")
	if rawKey == "" {
		return nil, errors.New("TOKEN_ENCRYPTION_KEY is not set in " + esi.EnvFile)
	}
	key, err := base64.StdEncoding.DecodeString(rawKey)
	if err != nil {
		return nil, fmt.Errorf("TOKEN_ENCRYPTION_KEY is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("TOKEN_ENCRYPTION_KEY must decode to 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// encryptToken returns the nonce followed by the sealed token.
func encryptToken(token string) ([]byte, error) {
	gcm, err := tokenCipher()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(token), nil), nil
}

func decryptToken(sealed []byte) (string, error) {
	gcm, err := tokenCipher()
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", errors.New("stored token is too short to decrypt")
	}
	nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("could not decrypt stored token (was TOKEN_ENCRYPTION_KEY changed?): %w", err)
	}
	return string(plain), nil
}
