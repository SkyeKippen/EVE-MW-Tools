package db

import (
	"MeadowWorksTools/src/history"
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// SaveMarketHistoryDay saves one day's market history for every region,
// updating rows already saved for that day, and records how many rows were
// saved and how many records EVE Ref's totals.json listed for the day
// (expected, 0 when it listed none). It's one transaction, so a day is saved
// whole or not at all.
func SaveMarketHistoryDay(ctx context.Context, day time.Time, rows []history.Row, expected int) error {
	conn, err := connectDB()
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		// rows already saved for the day are updated rather than deleted
		// first: deleting a day would scan the whole table, which is keyed
		// by item type
		if _, err := tx.Exec(ctx, `CREATE TEMP TABLE market_history_import
			(LIKE lifetime_indy.market_history INCLUDING DEFAULTS) ON COMMIT DROP`); err != nil {
			return err
		}
		_, err := tx.CopyFrom(ctx, pgx.Identifier{"market_history_import"},
			[]string{"type_id", "region_id", "date", "average", "highest", "lowest", "volume", "order_count"},
			pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
				r := rows[i]
				return []any{r.TypeID, r.RegionID, r.Date, r.Average, r.Highest, r.Lowest, r.Volume, r.OrderCount}, nil
			}))
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO lifetime_indy.market_history
			(type_id, region_id, date, average, highest, lowest, volume, order_count)
			SELECT DISTINCT ON (type_id, region_id, date)
				type_id, region_id, date, average, highest, lowest, volume, order_count
			FROM market_history_import
			ON CONFLICT (type_id, region_id, date) DO UPDATE SET
				average = excluded.average, highest = excluded.highest, lowest = excluded.lowest,
				volume = excluded.volume, order_count = excluded.order_count`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO lifetime_indy.market_history_days (date, rows, expected, imported_at)
			VALUES ($1, $2, $3, now())
			ON CONFLICT (date) DO UPDATE SET rows = excluded.rows, expected = excluded.expected,
				imported_at = excluded.imported_at`,
			day, len(rows), expected)
		return err
	})
}

// LoadMarketHistoryDays returns every day with saved market history.
func LoadMarketHistoryDays(ctx context.Context) (map[time.Time]history.Saved, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx, `SELECT date, rows, expected FROM lifetime_indy.market_history_days`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	saved := map[time.Time]history.Saved{}
	for rows.Next() {
		var day time.Time
		var s history.Saved
		if err := rows.Scan(&day, &s.Rows, &s.Expected); err != nil {
			return nil, err
		}
		saved[day.UTC()] = s
	}
	return saved, rows.Err()
}
