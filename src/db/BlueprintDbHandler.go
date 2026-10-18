package db

import (
	"MeadowWorksTools/src/esi"
	"MeadowWorksTools/src/places"
	"context"

	"github.com/jackc/pgx/v5"
)

// ReplaceBlueprints swaps the owner's saved blueprints for the list just
// fetched. Each is placed at a station or structure through the owner's
// saved assets, so save the assets first.
func ReplaceBlueprints(ctx context.Context, ownerID int64, ownerType string, blueprints []esi.Blueprint) error {
	conn, err := connectDB()
	if err != nil {
		return err
	}
	rows, err := conn.Query(ctx,
		`SELECT item_id, coalesce(place_id, 0) FROM lifetime_indy.assets WHERE owner_id = $1`, ownerID)
	if err != nil {
		return err
	}
	assetPlaces := map[int64]int64{}
	var itemID, placeID int64
	if _, err := pgx.ForEachRow(rows, []any{&itemID, &placeID}, func() error {
		assetPlaces[itemID] = placeID
		return nil
	}); err != nil {
		return err
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	if _, err := tx.Exec(ctx, `DELETE FROM lifetime_indy.blueprints WHERE owner_id = $1`, ownerID); err != nil {
		return err
	}
	seen := make(map[int64]bool, len(blueprints))
	copied := make([][]any, 0, len(blueprints))
	for _, b := range blueprints {
		if seen[b.ItemId] {
			continue
		}
		seen[b.ItemId] = true
		copied = append(copied, []any{ownerID, ownerType, b.ItemId, b.TypeId, b.LocationId, b.LocationFlag,
			b.Quantity, b.Runs, b.MaterialEfficiency, b.TimeEfficiency,
			nullIfZero(places.OfBlueprint(b.ItemId, b.LocationId, b.LocationFlag, assetPlaces))})
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"lifetime_indy", "blueprints"},
		[]string{"owner_id", "owner_type", "item_id", "type_id", "location_id", "location_flag",
			"quantity", "runs", "material_efficiency", "time_efficiency", "place_id"},
		pgx.CopyFromRows(copied))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
