package db

import (
	"MeadowWorksTools/src/esi"
	"MeadowWorksTools/src/places"
	"context"

	"github.com/jackc/pgx/v5"
)

// ReplaceAssets swaps the owner's saved assets for the list just fetched, so
// anything sold or moved away disappears. Corporations can have hundreds of
// thousands of items, so they are written with COPY.
func ReplaceAssets(ctx context.Context, ownerID int64, ownerType string, assets []esi.Asset) error {
	conn, err := connectDB()
	if err != nil {
		return err
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	if _, err := tx.Exec(ctx, `DELETE FROM lifetime_indy.assets WHERE owner_id = $1`, ownerID); err != nil {
		return err
	}

	// each item's station or structure, so pages can count what's at a place
	itemPlaces := places.Of(assets)

	// ESI can list the same item twice when it moves between pages being read
	seen := make(map[int64]bool, len(assets))
	rows := make([][]any, 0, len(assets))
	for _, asset := range assets {
		if seen[asset.ItemId] {
			continue
		}
		seen[asset.ItemId] = true
		rows = append(rows, []any{ownerID, ownerType, asset.ItemId, asset.TypeId, asset.Quantity, asset.LocationId,
			asset.LocationFlag, asset.LocationType, asset.IsSingleton, asset.IsBlueprintCopy,
			nullIfZero(itemPlaces[asset.ItemId])})
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"lifetime_indy", "assets"},
		[]string{"owner_id", "owner_type", "item_id", "type_id", "quantity", "location_id",
			"location_flag", "location_type", "is_singleton", "is_blueprint_copy", "place_id"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
