package db

import (
	"LifetimeIndustryTracker/src/mining"
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// SaveJitaSellPrices stores each material's Jita sell price as the day's
// price, replacing one saved earlier the same day.
func SaveJitaSellPrices(ctx context.Context, date time.Time, prices map[int64]float64) error {
	batch := &pgx.Batch{}
	for typeID, price := range prices {
		batch.Queue(`INSERT INTO lifetime_indy.jita_sell_prices (type_id, price_date, sell_price)
			VALUES ($1, $2, $3)
			ON CONFLICT (type_id, price_date) DO UPDATE SET sell_price = $3, updated_at = now()`,
			typeID, date, price)
	}
	return sendBatch(ctx, batch)
}

// PricedOn returns which of the materials already have a price saved for
// the day.
func PricedOn(ctx context.Context, typeIDs []int64, date time.Time) (map[int64]bool, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx,
		`SELECT type_id FROM lifetime_indy.jita_sell_prices WHERE type_id = ANY($1) AND price_date = $2`,
		typeIDs, date)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, err
	}
	priced := make(map[int64]bool, len(ids))
	for _, id := range ids {
		priced[id] = true
	}
	return priced, nil
}

// LoadJitaSellPrices returns every saved daily price of the materials.
func LoadJitaSellPrices(ctx context.Context, typeIDs []int64) (mining.PriceBook, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx,
		`SELECT type_id, price_date, sell_price::float8 FROM lifetime_indy.jita_sell_prices
		WHERE type_id = ANY($1) ORDER BY type_id, price_date`,
		typeIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	book := mining.PriceBook{}
	for rows.Next() {
		var typeID int64
		var price mining.DatedPrice
		if err := rows.Scan(&typeID, &price.Date, &price.Price); err != nil {
			return nil, err
		}
		book[typeID] = append(book[typeID], price)
	}
	return book, rows.Err()
}

// LoadMinedDays returns how much of each ore type the characters mined each
// day, all of them together.
func LoadMinedDays(ctx context.Context, characterIDs []int64) ([]mining.MinedDay, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx,
		`SELECT type_id, ore_type, date, sum(quantity)::bigint
		FROM lifetime_indy.mining_ledger
		WHERE character_id = ANY($1)
		GROUP BY type_id, ore_type, date`,
		characterIDs)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (mining.MinedDay, error) {
		var day mining.MinedDay
		err := row.Scan(&day.TypeID, &day.OreType, &day.Date, &day.Quantity)
		day.BaseOre = mining.BaseOreName(day.OreType)
		return day, err
	})
}

// LoadMinedOreTypes returns every ore type anyone has mined, character or
// corporation ledger, by type ID.
func LoadMinedOreTypes(ctx context.Context) (map[int64]string, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx,
		`SELECT DISTINCT type_id, ore_type FROM lifetime_indy.mining_ledger
		UNION
		SELECT DISTINCT type_id, ore_type FROM lifetime_indy.corp_mining_ledger`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	oreTypes := map[int64]string{}
	for rows.Next() {
		var typeID int64
		var name string
		if err := rows.Scan(&typeID, &name); err != nil {
			return nil, err
		}
		oreTypes[typeID] = name
	}
	return oreTypes, rows.Err()
}
