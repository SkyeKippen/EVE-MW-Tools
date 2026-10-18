package db

import (
	"MeadowWorksTools/src/mining"
	"MeadowWorksTools/src/prices"
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// SaveDailyJitaPrices saves the day's Jita sell prices, replacing any
// already saved for that day.
func SaveDailyJitaPrices(ctx context.Context, day time.Time, list []prices.Price) error {
	conn, err := connectDB()
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM lifetime_indy.jita_sell_prices WHERE price_date = $1`, day); err != nil {
			return err
		}
		_, err := tx.CopyFrom(ctx, pgx.Identifier{"lifetime_indy", "jita_sell_prices"},
			[]string{"type_id", "price_date", "sell_price", "min_price", "volume", "order_count"},
			pgx.CopyFromSlice(len(list), func(i int) ([]any, error) {
				p := list[i]
				return []any{p.TypeId, day, p.SellPrice, p.MinPrice, p.Volume, p.OrderCount}, nil
			}))
		return err
	})
}

// LatestPriceDay returns the newest day with saved Jita prices, or the zero
// time when there are none.
func LatestPriceDay(ctx context.Context) (time.Time, error) {
	conn, err := connectDB()
	if err != nil {
		return time.Time{}, err
	}
	var day *time.Time
	err = conn.QueryRow(ctx, `SELECT max(price_date) FROM lifetime_indy.jita_sell_prices`).Scan(&day)
	if err != nil || day == nil {
		return time.Time{}, err
	}
	return *day, nil
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
