package db

import (
	"MeadowWorksTools/src/stockpile"
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ListStockpiles returns the user's stockpiles with their items, sorted by
// name.
func ListStockpiles(ctx context.Context, userID int64) ([]stockpile.Stockpile, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx,
		`SELECT s.stockpile_id, s.name, s.created_by_name, `+prefixed("p.", placeColumns)+`
		FROM lifetime_indy.stockpiles s
		JOIN lifetime_indy.places p ON p.location_id = s.place_id
		WHERE s.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (stockpile.Stockpile, error) {
		var s stockpile.Stockpile
		p := &s.Place
		err := row.Scan(&s.StockpileId, &s.Name, &s.CreatedByName, &p.LocationID, &p.Kind, &p.Accessible,
			&p.Name, &p.TypeName, &p.OwnerName, &p.System, &p.Region, &p.CheckedAt)
		return s, err
	})
	if err != nil {
		return nil, err
	}

	rows, err = conn.Query(ctx,
		`SELECT i.stockpile_id, i.stockpile_item_id, i.type_id, i.name, i.kind, i.quantity
		FROM lifetime_indy.stockpile_items i
		JOIN lifetime_indy.stockpiles s USING (stockpile_id)
		WHERE s.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	itemsByStockpile := map[int64][]stockpile.Item{}
	var stockpileID int64
	var item stockpile.Item
	_, err = pgx.ForEachRow(rows, []any{&stockpileID, &item.StockpileItemId, &item.TypeId, &item.Name, &item.Kind, &item.Quantity}, func() error {
		itemsByStockpile[stockpileID] = append(itemsByStockpile[stockpileID], item)
		return nil
	})
	if err != nil {
		return nil, err
	}

	for i := range list {
		items := itemsByStockpile[list[i].StockpileId]
		slices.SortFunc(items, func(a, b stockpile.Item) int { return stockpile.Compare(a.Name, b.Name) })
		list[i].Items = items
	}
	slices.SortFunc(list, func(a, b stockpile.Stockpile) int { return stockpile.Compare(a.Name, b.Name) })
	return list, nil
}

// CreateStockpile makes a stockpile for the user at a saved place, and
// returns its ID. It's stockpile.ErrNotFound when the place isn't saved.
func CreateStockpile(ctx context.Context, userID int64, name string, placeID, createdByID int64, createdByName string) (int64, error) {
	conn, err := connectDB()
	if err != nil {
		return 0, err
	}
	var id int64
	err = conn.QueryRow(ctx,
		`INSERT INTO lifetime_indy.stockpiles (user_id, name, place_id, created_by_character_id, created_by_name)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING stockpile_id`, userID, name, placeID, createdByID, createdByName).Scan(&id)
	if isForeignKeyViolation(err) {
		return 0, stockpile.ErrNotFound
	}
	return id, err
}

// DeleteStockpile deletes one of the user's stockpiles and its items.
func DeleteStockpile(ctx context.Context, userID, stockpileID int64) error {
	conn, err := connectDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx,
		`DELETE FROM lifetime_indy.stockpiles WHERE stockpile_id = $1 AND user_id = $2`, stockpileID, userID)
	return err
}

// SetStockpileItem adds the item to one of the user's stockpiles, or changes
// the wanted quantity if the stockpile already has it as the same kind. It's
// stockpile.ErrNotFound when the stockpile isn't the user's.
func SetStockpileItem(ctx context.Context, userID, stockpileID int64, item stockpile.Item) error {
	conn, err := connectDB()
	if err != nil {
		return err
	}
	tag, err := conn.Exec(ctx,
		`INSERT INTO lifetime_indy.stockpile_items (stockpile_id, type_id, name, kind, quantity)
		SELECT stockpile_id, $3, $4, $5, $6 FROM lifetime_indy.stockpiles WHERE stockpile_id = $1 AND user_id = $2
		ON CONFLICT (stockpile_id, type_id, kind) DO UPDATE SET quantity = excluded.quantity, name = excluded.name`,
		stockpileID, userID, item.TypeId, item.Name, item.Kind, item.Quantity)
	if err == nil && tag.RowsAffected() == 0 {
		return stockpile.ErrNotFound
	}
	return err
}

// RemoveStockpileItem removes an item from one of the user's stockpiles.
func RemoveStockpileItem(ctx context.Context, userID, stockpileID, stockpileItemID int64) error {
	conn, err := connectDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx,
		`DELETE FROM lifetime_indy.stockpile_items i USING lifetime_indy.stockpiles s
		WHERE i.stockpile_id = s.stockpile_id AND s.user_id = $1 AND s.stockpile_id = $2 AND i.stockpile_item_id = $3`,
		userID, stockpileID, stockpileItemID)
	return err
}

// PlaceType is one item type at one station or structure.
type PlaceType struct {
	PlaceID int64
	TypeID  int64
}

// BlueprintRuns is what's at a place of one blueprint: the runs left on its
// copies, and whether there's an original.
type BlueprintRuns struct {
	Runs int64 `json:"runs"`
	BPO  bool  `json:"bpo"`
}

// LoadStockAtPlaces counts the owners' saved items of each type at each of
// placeIDs. Items in containers and ship holds count; modules fitted to
// ships don't.
func LoadStockAtPlaces(ctx context.Context, ownerIDs, placeIDs []int64) (map[PlaceType]int64, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx,
		`SELECT place_id, type_id, sum(quantity)::bigint FROM lifetime_indy.assets
		WHERE owner_id = ANY($1) AND place_id = ANY($2) AND location_flag NOT LIKE '%Slot%'
		GROUP BY place_id, type_id`, ownerIDs, placeIDs)
	if err != nil {
		return nil, err
	}
	counts := map[PlaceType]int64{}
	var key PlaceType
	var quantity int64
	_, err = pgx.ForEachRow(rows, []any{&key.PlaceID, &key.TypeID, &quantity}, func() error {
		counts[key] = quantity
		return nil
	})
	return counts, err
}

// LoadBlueprintRunsAtPlaces totals the owners' saved blueprints of each type
// at each of placeIDs.
func LoadBlueprintRunsAtPlaces(ctx context.Context, ownerIDs, placeIDs []int64) (map[PlaceType]BlueprintRuns, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx,
		`SELECT place_id, type_id, coalesce(sum(runs) FILTER (WHERE quantity = -2), 0)::bigint, bool_or(quantity <> -2)
		FROM lifetime_indy.blueprints
		WHERE owner_id = ANY($1) AND place_id = ANY($2)
		GROUP BY place_id, type_id`, ownerIDs, placeIDs)
	if err != nil {
		return nil, err
	}
	runs := map[PlaceType]BlueprintRuns{}
	var key PlaceType
	var total BlueprintRuns
	_, err = pgx.ForEachRow(rows, []any{&key.PlaceID, &key.TypeID, &total.Runs, &total.BPO}, func() error {
		runs[key] = total
		return nil
	})
	return runs, err
}

// SavedOwnerData reports which of the owners have assets and blueprints
// saved, by owner type, so a page can say why a count is missing.
func SavedOwnerData(ctx context.Context, ownerIDs []int64) (assets, blueprints map[string]bool, err error) {
	conn, err := connectDB()
	if err != nil {
		return nil, nil, err
	}
	assets, blueprints = map[string]bool{}, map[string]bool{}
	rows, err := conn.Query(ctx,
		`SELECT 'assets', owner_type FROM lifetime_indy.assets WHERE owner_id = ANY($1) GROUP BY owner_type
		UNION ALL
		SELECT 'blueprints', owner_type FROM lifetime_indy.blueprints WHERE owner_id = ANY($1) GROUP BY owner_type`,
		ownerIDs)
	if err != nil {
		return nil, nil, err
	}
	var table, ownerType string
	_, err = pgx.ForEachRow(rows, []any{&table, &ownerType}, func() error {
		if table == "assets" {
			assets[ownerType] = true
		} else {
			blueprints[ownerType] = true
		}
		return nil
	})
	return assets, blueprints, err
}

// prefixed puts prefix before each of a comma-separated column list.
func prefixed(prefix, columns string) string {
	parts := strings.Split(columns, ",")
	for i, part := range parts {
		parts[i] = prefix + strings.TrimSpace(part)
	}
	return strings.Join(parts, ", ")
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
