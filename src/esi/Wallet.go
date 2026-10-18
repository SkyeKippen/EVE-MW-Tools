package esi

import (
	"context"
	"time"
)

// WalletJournalEntry is one line of a character's or corporation's wallet
// journal. Optional fields are zero when ESI leaves them out.
type WalletJournalEntry struct {
	Id            int64     `json:"id"`
	Date          time.Time `json:"date"`
	RefType       string    `json:"ref_type"` // e.g. market_escrow, industry_job_tax
	Description   string    `json:"description"`
	Amount        float64   `json:"amount"`
	Balance       float64   `json:"balance"`
	ContextId     int64     `json:"context_id"`
	ContextIdType string    `json:"context_id_type"` // e.g. market_transaction_id, industry_job_id
	FirstPartyId  int64     `json:"first_party_id"`
	SecondPartyId int64     `json:"second_party_id"`
	Reason        string    `json:"reason"`
	Tax           float64   `json:"tax"`
	TaxReceiverId int64     `json:"tax_receiver_id"`
}

// WalletBalance is the balance of one corporation wallet division (1 to 7).
type WalletBalance struct {
	Division int     `json:"division"`
	Balance  float64 `json:"balance"`
}

// GetCharacterWalletJournal returns the last 30 days of the character's
// journal. Needs the esi-wallet.read_character_wallet.v1 scope.
func (c *Client) GetCharacterWalletJournal(ctx context.Context, characterId int64, accessToken string) ([]WalletJournalEntry, error) {
	return getPages[WalletJournalEntry](ctx, c, c.url("/characters/%d/wallet/journal", characterId), accessToken)
}

// GetCorporationWalletJournal returns the last 30 days of one wallet
// division's journal. Needs the esi-wallet.read_corporation_wallets.v1 scope
// and the Accountant or Junior Accountant role.
func (c *Client) GetCorporationWalletJournal(ctx context.Context, corporationId int64, division int, accessToken string) ([]WalletJournalEntry, error) {
	return getPages[WalletJournalEntry](ctx, c, c.url("/corporations/%d/wallets/%d/journal", corporationId, division), accessToken)
}

// GetCharacterWalletBalance needs the esi-wallet.read_character_wallet.v1
// scope.
func (c *Client) GetCharacterWalletBalance(ctx context.Context, characterId int64, accessToken string) (float64, error) {
	return get[float64](ctx, c, c.url("/characters/%d/wallet", characterId), accessToken)
}

// GetCorporationWalletBalances returns every wallet division's balance.
// Needs the esi-wallet.read_corporation_wallets.v1 scope and the Accountant
// or Junior Accountant role.
func (c *Client) GetCorporationWalletBalances(ctx context.Context, corporationId int64, accessToken string) ([]WalletBalance, error) {
	return get[[]WalletBalance](ctx, c, c.url("/corporations/%d/wallets", corporationId), accessToken)
}

// WalletTransaction is one market purchase or sale from a character's or
// corporation's wallet.
type WalletTransaction struct {
	TransactionId int64     `json:"transaction_id"`
	Date          time.Time `json:"date"`
	TypeId        int64     `json:"type_id"`
	Quantity      int64     `json:"quantity"`
	UnitPrice     float64   `json:"unit_price"`
	IsBuy         bool      `json:"is_buy"`
	IsPersonal    bool      `json:"is_personal"` // character transactions only; false when made for the corporation
	ClientId      int64     `json:"client_id"`
	LocationId    int64     `json:"location_id"`
	JournalRefId  int64     `json:"journal_ref_id"`
}

// GetCharacterWalletTransactions returns the character's latest market
// transactions (ESI keeps 30 days). Needs the
// esi-wallet.read_character_wallet.v1 scope.
func (c *Client) GetCharacterWalletTransactions(ctx context.Context, characterId int64, accessToken string) ([]WalletTransaction, error) {
	return get[[]WalletTransaction](ctx, c, c.url("/characters/%d/wallet/transactions", characterId), accessToken)
}

// GetCorporationWalletTransactions returns one wallet division's latest
// market transactions. Needs the esi-wallet.read_corporation_wallets.v1 scope
// and the Accountant or Junior Accountant role.
func (c *Client) GetCorporationWalletTransactions(ctx context.Context, corporationId int64, division int, accessToken string) ([]WalletTransaction, error) {
	return get[[]WalletTransaction](ctx, c, c.url("/corporations/%d/wallets/%d/transactions", corporationId, division), accessToken)
}
