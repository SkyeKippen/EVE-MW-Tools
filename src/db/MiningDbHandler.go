package db

import (
	"MeadowWorksTools/src/mining"
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// MiningRow is one line of a character's mining ledger: how much of one ore
// type they mined in one solar system on one day.
type MiningRow struct {
	CharacterID     int64
	CharacterName   string
	Date            time.Time
	SolarSystemID   int64
	SolarSystemName string
	TypeID          int64
	OreType         string // e.g. Veldspar II-Grade
	BaseOre         string // e.g. Veldspar
	Quantity        int64
}

// SaveMiningLedger stores or replaces each row. ESI keeps raising a day's
// quantity until the day ends, so a row already saved takes the new amount.
func SaveMiningLedger(ctx context.Context, rows []MiningRow) error {
	batch := &pgx.Batch{}
	for _, row := range rows {
		batch.Queue(
			`INSERT INTO lifetime_indy.mining_ledger
			(character_id, character_name, date, solar_system_id, solar_system_name, type_id, ore_type, base_ore, quantity)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (character_id, date, solar_system_id, type_id) DO UPDATE
			SET character_name = $2, solar_system_name = $5, ore_type = $7, base_ore = $8, quantity = $9, updated_at = now()`,
			row.CharacterID, row.CharacterName, row.Date, row.SolarSystemID, row.SolarSystemName, row.TypeID, row.OreType, row.BaseOre, row.Quantity)
	}
	return sendBatch(ctx, batch)
}

// CorpMiningRow is how much of one ore type one character mined at one of a
// corporation's mining observers on one day.
type CorpMiningRow struct {
	CorporationID         int64
	ObserverID            int64
	ObserverName          string
	SolarSystemID         int64 // 0 when the observer couldn't be looked up
	SolarSystemName       string
	Date                  time.Time
	CharacterID           int64
	CharacterName         string
	RecordedCorporationID int64
	TypeID                int64
	OreType               string
	BaseOre               string
	Quantity              int64
}

// SaveCorporationMiningLedger stores or replaces each row, like
// SaveMiningLedger.
func SaveCorporationMiningLedger(ctx context.Context, rows []CorpMiningRow) error {
	batch := &pgx.Batch{}
	for _, row := range rows {
		batch.Queue(
			`INSERT INTO lifetime_indy.corp_mining_ledger
			(corporation_id, observer_id, observer_name, solar_system_id, solar_system_name, date, character_id,
			character_name, recorded_corporation_id, type_id, ore_type, base_ore, quantity)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			ON CONFLICT (corporation_id, observer_id, date, character_id, type_id) DO UPDATE SET
			observer_name = CASE WHEN $3 = '' THEN corp_mining_ledger.observer_name ELSE $3 END,
			solar_system_id = COALESCE($4, corp_mining_ledger.solar_system_id),
			solar_system_name = CASE WHEN $5 = '' THEN corp_mining_ledger.solar_system_name ELSE $5 END,
			character_name = $8, recorded_corporation_id = $9, ore_type = $11, base_ore = $12, quantity = $13,
			updated_at = now()`,
			row.CorporationID, row.ObserverID, row.ObserverName, nullIfZero(row.SolarSystemID), row.SolarSystemName,
			row.Date, row.CharacterID, row.CharacterName, row.RecordedCorporationID, row.TypeID, row.OreType,
			row.BaseOre, row.Quantity)
	}
	return sendBatch(ctx, batch)
}

// LoadOreTotals adds up everything the characters have ever mined, per base
// ore, largest first. The base ore is worked out from each ore type's name
// rather than read from base_ore, so rows saved before a naming rule was
// added (such as moon ore variants) are grouped correctly too.
func LoadOreTotals(ctx context.Context, characterIDs []int64) ([]mining.OreTotal, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}

	rows, err := conn.Query(ctx,
		`SELECT ore_type, sum(quantity)::bigint
		FROM lifetime_indy.mining_ledger
		WHERE character_id = ANY($1)
		GROUP BY ore_type`,
		characterIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byType := map[string]int64{}
	for rows.Next() {
		var oreType string
		var quantity int64
		if err := rows.Scan(&oreType, &quantity); err != nil {
			return nil, err
		}
		byType[oreType] = quantity
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return mining.TotalsByBaseOre(byType), nil
}
