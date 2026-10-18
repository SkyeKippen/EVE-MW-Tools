package db

import (
	"LifetimeIndustryTracker/src/esi"
	"LifetimeIndustryTracker/src/worth"
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// SaveWalletTransactions stores a character's wallet transactions (division
// 0) or one corporation wallet division's. A transaction never changes, so
// ones already saved are skipped.
func SaveWalletTransactions(ctx context.Context, ownerID int64, ownerType string, division int, transactions []esi.WalletTransaction) error {
	batch := &pgx.Batch{}
	for _, t := range transactions {
		var isPersonal any
		if ownerType == OwnerCharacter {
			isPersonal = t.IsPersonal
		}
		batch.Queue(`INSERT INTO lifetime_indy.wallet_transactions
			(owner_id, owner_type, division, transaction_id, date, type_id, quantity, unit_price, is_buy,
			is_personal, client_id, location_id, journal_ref_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			ON CONFLICT (owner_id, division, transaction_id) DO NOTHING`,
			ownerID, ownerType, division, t.TransactionId, t.Date, t.TypeId, t.Quantity, t.UnitPrice, t.IsBuy,
			isPersonal, t.ClientId, t.LocationId, t.JournalRefId)
	}
	return sendBatch(ctx, batch)
}

// SetCharacterCorporation records which corporation a linked character is
// in.
func SetCharacterCorporation(ctx context.Context, characterID, corporationID int64) error {
	conn, err := connectDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx,
		`UPDATE lifetime_indy.user_characters SET corporation_id = $2
		WHERE character_id = $1 AND corporation_id IS DISTINCT FROM $2`,
		characterID, corporationID)
	return err
}

// LoadUserCorporations returns the player corporations the user's characters
// are in. NPC corporations are left out: they have no wallets or assets to
// read, and counting them as the user's own would hide NPC payments.
func LoadUserCorporations(ctx context.Context, userID int64) ([]int64, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx,
		`SELECT DISTINCT corporation_id FROM lifetime_indy.user_characters
		WHERE user_id = $1 AND corporation_id >= 2000000 ORDER BY corporation_id`,
		userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// LoadIskFlows adds up the ISK the characters and corporations have earned
// and spent, from every wallet entry saved. Market purchases and sales come
// from the wallet transactions and are split into PLEX and other items
// (assets); every other wallet entry, such as bounties, contracts, taxes and
// fees, is liquid. ISK moved between the user's own characters and
// corporations is neither earned nor spent, so it's left out. since is the
// date of the oldest saved wallet entry, zero when there are none.
func LoadIskFlows(ctx context.Context, characterIDs, corporationIDs []int64) (earned, spent worth.Split, since time.Time, err error) {
	conn, err := connectDB()
	if err != nil {
		return earned, spent, since, err
	}
	owners := append(append([]int64{}, characterIDs...), corporationIDs...)

	// market entries are counted from the transactions below instead
	var oldest *time.Time
	err = conn.QueryRow(ctx,
		`WITH journal AS (
			SELECT date, ref_type, amount, first_party_id, second_party_id
			FROM lifetime_indy.character_wallet_journal WHERE character_id = ANY($1)
			UNION ALL
			SELECT date, ref_type, amount, first_party_id, second_party_id
			FROM lifetime_indy.corp_wallet_journal WHERE corporation_id = ANY($2)
		)
		SELECT coalesce(sum(amount) FILTER (WHERE amount > 0 AND counted), 0)::float8,
			coalesce(-sum(amount) FILTER (WHERE amount < 0 AND counted), 0)::float8,
			min(date)
		FROM (SELECT date, amount,
				ref_type NOT IN ('market_transaction', 'market_escrow')
				AND NOT (coalesce(first_party_id = ANY($3), false) AND coalesce(second_party_id = ANY($3), false))
				AS counted
			FROM journal) entries`,
		characterIDs, corporationIDs, owners).Scan(&earned.Liquid, &spent.Liquid, &oldest)
	if err != nil {
		return earned, spent, since, err
	}
	if oldest != nil {
		since = *oldest
	}

	// a character's trades for their corporation are in the corporation's
	// transactions too
	rows, err := conn.Query(ctx,
		`SELECT is_buy, type_id = $4 AS plex, sum(quantity * unit_price)::float8
		FROM lifetime_indy.wallet_transactions
		WHERE ((owner_type = 'character' AND owner_id = ANY($1) AND is_personal)
			OR (owner_type = 'corporation' AND owner_id = ANY($2)))
			AND NOT client_id = ANY($3)
		GROUP BY 1, 2`,
		characterIDs, corporationIDs, owners, worth.PlexTypeID)
	if err != nil {
		return earned, spent, since, err
	}
	defer rows.Close()
	for rows.Next() {
		var isBuy, plex bool
		var amount float64
		if err := rows.Scan(&isBuy, &plex, &amount); err != nil {
			return earned, spent, since, err
		}
		split := &earned
		if isBuy {
			split = &spent
		}
		if plex {
			split.Plex += amount
		} else {
			split.Assets += amount
		}
	}
	return earned, spent, since, rows.Err()
}

// LoadHoldings returns what the owners hold now: the ISK in their wallets
// and in open buy orders, and how many of each type they have in their
// assets and open sell orders. Blueprint copies can't be sold, so they're
// left out. A corporation order also shows under the character who placed
// it, so only the corporation's copy is counted.
func LoadHoldings(ctx context.Context, ownerIDs []int64) (liquid float64, quantities map[int64]int64, err error) {
	conn, err := connectDB()
	if err != nil {
		return 0, nil, err
	}
	err = conn.QueryRow(ctx,
		`SELECT coalesce((SELECT sum(balance) FROM lifetime_indy.wallet_balances WHERE owner_id = ANY($1)), 0)::float8
			+ coalesce((SELECT sum(escrow) FROM lifetime_indy.market_orders
				WHERE owner_id = ANY($1) AND state = 'open' AND is_buy_order AND is_corporation IS NOT TRUE), 0)::float8`,
		ownerIDs).Scan(&liquid)
	if err != nil {
		return 0, nil, err
	}

	rows, err := conn.Query(ctx,
		`SELECT type_id, sum(quantity)::bigint FROM (
			SELECT type_id, quantity FROM lifetime_indy.assets
			WHERE owner_id = ANY($1) AND NOT is_blueprint_copy
			UNION ALL
			SELECT type_id, volume_remain FROM lifetime_indy.market_orders
			WHERE owner_id = ANY($1) AND state = 'open' AND NOT is_buy_order AND is_corporation IS NOT TRUE
		) held
		GROUP BY type_id`,
		ownerIDs)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	quantities = map[int64]int64{}
	for rows.Next() {
		var typeID, quantity int64
		if err := rows.Scan(&typeID, &quantity); err != nil {
			return 0, nil, err
		}
		quantities[typeID] = quantity
	}
	return liquid, quantities, rows.Err()
}

// LoadHeldTypeIDs returns every type anyone holds in assets or open sell
// orders, so the refresh can save their prices.
func LoadHeldTypeIDs(ctx context.Context) ([]int64, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx,
		`SELECT type_id FROM lifetime_indy.assets WHERE NOT is_blueprint_copy
		UNION
		SELECT type_id FROM lifetime_indy.market_orders WHERE state = 'open' AND NOT is_buy_order
		ORDER BY type_id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// LoadLatestJitaPrices returns each type's most recently saved Jita sell
// price.
func LoadLatestJitaPrices(ctx context.Context, typeIDs []int64) (map[int64]float64, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx,
		`SELECT DISTINCT ON (type_id) type_id, sell_price::float8 FROM lifetime_indy.jita_sell_prices
		WHERE type_id = ANY($1) ORDER BY type_id, price_date DESC`,
		typeIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	prices := map[int64]float64{}
	for rows.Next() {
		var typeID int64
		var price float64
		if err := rows.Scan(&typeID, &price); err != nil {
			return nil, err
		}
		prices[typeID] = price
	}
	return prices, rows.Err()
}
