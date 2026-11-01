package db

import (
	"MeadowWorksTools/src/prices"
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// SaveDailyMarketPrices saves one market's buy and sell prices for the day,
// replacing any already saved for that market and day.
func SaveDailyMarketPrices(ctx context.Context, market string, day time.Time, list []prices.MarketPrice) error {
	conn, err := connectDB()
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM lifetime_indy.market_prices WHERE market = $1 AND price_date = $2`,
			market, day); err != nil {
			return err
		}
		_, err := tx.CopyFrom(ctx, pgx.Identifier{"lifetime_indy", "market_prices"},
			[]string{"market", "type_id", "price_date", "sell_price", "min_sell", "sell_volume", "sell_orders",
				"buy_price", "max_buy", "buy_volume", "buy_orders"},
			pgx.CopyFromSlice(len(list), func(i int) ([]any, error) {
				p := list[i]
				row := []any{market, p.TypeId, day, nil, nil, nil, nil, nil, nil, nil, nil}
				if p.HasSell {
					row[3], row[4], row[5], row[6] = p.Sell, p.MinSell, p.SellVolume, p.SellOrders
				}
				if p.HasBuy {
					row[7], row[8], row[9], row[10] = p.Buy, p.MaxBuy, p.BuyVolume, p.BuyOrders
				}
				return row, nil
			}))
		return err
	})
}

// LatestMarketPriceDay returns the newest day with saved prices for the
// market, or the zero time when there are none.
func LatestMarketPriceDay(ctx context.Context, market string) (time.Time, error) {
	conn, err := connectDB()
	if err != nil {
		return time.Time{}, err
	}
	var day *time.Time
	err = conn.QueryRow(ctx, `SELECT max(price_date) FROM lifetime_indy.market_prices WHERE market = $1`, market).Scan(&day)
	if err != nil || day == nil {
		return time.Time{}, err
	}
	return *day, nil
}

// DatedMarketPrice is a type's prices in one market on one day.
type DatedMarketPrice struct {
	prices.MarketPrice
	Market string
	Date   time.Time
}

func scanMarketPrice(row pgx.CollectableRow) (DatedMarketPrice, error) {
	var p DatedMarketPrice
	var sell, minSell, buy, maxBuy *float64
	var sellVolume, buyVolume *int64
	var sellOrders, buyOrders *int
	err := row.Scan(&p.Market, &p.TypeId, &p.Date, &sell, &minSell, &sellVolume, &sellOrders,
		&buy, &maxBuy, &buyVolume, &buyOrders)
	if sell != nil {
		p.HasSell, p.Sell = true, *sell
		p.MinSell, p.SellVolume, p.SellOrders = deref(minSell), deref(sellVolume), deref(sellOrders)
	}
	if buy != nil {
		p.HasBuy, p.Buy = true, *buy
		p.MaxBuy, p.BuyVolume, p.BuyOrders = deref(maxBuy), deref(buyVolume), deref(buyOrders)
	}
	return p, err
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

const marketPriceColumns = `market, type_id, price_date, sell_price::float8, min_sell::float8, sell_volume, sell_orders,
	buy_price::float8, max_buy::float8, buy_volume, buy_orders`

// LoadLatestMarketPrices returns each type's prices on the newest saved day
// of each market, keyed by market and then type ID. A market's newest day is
// used for all its types, so a type missing that day has no price there.
func LoadLatestMarketPrices(ctx context.Context, typeIDs []int64) (map[string]map[int64]DatedMarketPrice, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx, `WITH latest AS (
			SELECT market, max(price_date) AS price_date FROM lifetime_indy.market_prices GROUP BY market)
		SELECT `+marketPriceColumns+` FROM lifetime_indy.market_prices
		JOIN latest USING (market, price_date)
		WHERE type_id = ANY($1)`, typeIDs)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, scanMarketPrice)
	if err != nil {
		return nil, err
	}
	byMarket := map[string]map[int64]DatedMarketPrice{}
	for _, p := range list {
		if byMarket[p.Market] == nil {
			byMarket[p.Market] = map[int64]DatedMarketPrice{}
		}
		byMarket[p.Market][p.TypeId] = p
	}
	return byMarket, nil
}

// LoadMarketPriceHistory returns every saved day's prices of one type in
// every market, oldest first.
func LoadMarketPriceHistory(ctx context.Context, typeID int64) ([]DatedMarketPrice, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx, `SELECT `+marketPriceColumns+` FROM lifetime_indy.market_prices
		WHERE type_id = $1 ORDER BY price_date, market`, typeID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanMarketPrice)
}

// HistoryDay is one day of CCP's market history for a type in a region.
type HistoryDay struct {
	Date       time.Time `json:"date"`
	Average    float64   `json:"average"`
	Highest    float64   `json:"highest"`
	Lowest     float64   `json:"lowest"`
	Volume     int64     `json:"volume"`
	OrderCount int64     `json:"orders"`
}

// LoadMarketHistory returns CCP's daily market history of one type in one
// region since the day given, oldest first.
func LoadMarketHistory(ctx context.Context, typeID, regionID int64, since time.Time) ([]HistoryDay, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx, `SELECT date, average, highest, lowest, volume, order_count
		FROM lifetime_indy.market_history
		WHERE type_id = $1 AND region_id = $2 AND date >= $3 ORDER BY date`, typeID, regionID, since)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (HistoryDay, error) {
		var d HistoryDay
		err := row.Scan(&d.Date, &d.Average, &d.Highest, &d.Lowest, &d.Volume, &d.OrderCount)
		return d, err
	})
}

// LoadPackagedVolumes returns the saved packaged volumes, keyed by type ID.
func LoadPackagedVolumes(ctx context.Context) (map[int64]float64, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx, `SELECT type_id, packaged_volume FROM lifetime_indy.packaged_volumes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	volumes := map[int64]float64{}
	for rows.Next() {
		var typeID int64
		var volume float64
		if err := rows.Scan(&typeID, &volume); err != nil {
			return nil, err
		}
		volumes[typeID] = volume
	}
	return volumes, rows.Err()
}

// SavePackagedVolume saves a type's packaged volume.
func SavePackagedVolume(ctx context.Context, typeID int64, volume float64) error {
	conn, err := connectDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx, `INSERT INTO lifetime_indy.packaged_volumes (type_id, packaged_volume) VALUES ($1, $2)
		ON CONFLICT (type_id) DO UPDATE SET packaged_volume = excluded.packaged_volume, fetched_at = now()`,
		typeID, volume)
	return err
}
