package db

import (
	"MeadowWorksTools/src/esi"
	"context"

	"github.com/jackc/pgx/v5"
)

const journalColumns = `journal_id, date, ref_type, description, amount, balance, context_id, context_id_type,
	first_party_id, second_party_id, reason, tax, tax_receiver_id`

const journalUpdate = `date = EXCLUDED.date, ref_type = EXCLUDED.ref_type, description = EXCLUDED.description,
	amount = EXCLUDED.amount, balance = EXCLUDED.balance, context_id = EXCLUDED.context_id,
	context_id_type = EXCLUDED.context_id_type, first_party_id = EXCLUDED.first_party_id,
	second_party_id = EXCLUDED.second_party_id, reason = EXCLUDED.reason, tax = EXCLUDED.tax,
	tax_receiver_id = EXCLUDED.tax_receiver_id, fetched_at = now()`

func journalValues(entry esi.WalletJournalEntry) []any {
	return []any{entry.Id, entry.Date, entry.RefType, entry.Description, entry.Amount, entry.Balance,
		nullIfZero(entry.ContextId), nullIfZero(entry.ContextIdType), nullIfZero(entry.FirstPartyId),
		nullIfZero(entry.SecondPartyId), entry.Reason, nullIfZero(entry.Tax), nullIfZero(entry.TaxReceiverId)}
}

// SaveCharacterWalletJournal stores the entries, updating any already saved.
func SaveCharacterWalletJournal(ctx context.Context, characterID int64, entries []esi.WalletJournalEntry) error {
	batch := &pgx.Batch{}
	for _, entry := range entries {
		batch.Queue(`INSERT INTO lifetime_indy.character_wallet_journal (character_id, `+journalColumns+`)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			ON CONFLICT (character_id, journal_id) DO UPDATE SET `+journalUpdate,
			append([]any{characterID}, journalValues(entry)...)...)
	}
	return sendBatch(ctx, batch)
}

// SaveCorporationWalletJournal stores one division's entries, updating any
// already saved.
func SaveCorporationWalletJournal(ctx context.Context, corporationID int64, division int, entries []esi.WalletJournalEntry) error {
	batch := &pgx.Batch{}
	for _, entry := range entries {
		batch.Queue(`INSERT INTO lifetime_indy.corp_wallet_journal (corporation_id, division, `+journalColumns+`)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
			ON CONFLICT (corporation_id, division, journal_id) DO UPDATE SET `+journalUpdate,
			append([]any{corporationID, division}, journalValues(entry)...)...)
	}
	return sendBatch(ctx, batch)
}

// SaveWalletBalance stores the current balance of a character's wallet
// (division 0) or a corporation wallet division.
func SaveWalletBalance(ctx context.Context, ownerID int64, ownerType string, division int, balance float64) error {
	conn, err := connectDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx,
		`INSERT INTO lifetime_indy.wallet_balances (owner_id, owner_type, division, balance)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (owner_id, division) DO UPDATE SET owner_type = $2, balance = $4, updated_at = now()`,
		ownerID, ownerType, division, balance)
	return err
}

// sendBatch runs every queued statement in one round trip and one
// transaction.
func sendBatch(ctx context.Context, batch *pgx.Batch) error {
	if batch.Len() == 0 {
		return nil
	}
	conn, err := connectDB()
	if err != nil {
		return err
	}
	return conn.SendBatch(ctx, batch).Close()
}
