package db

import (
	"MeadowWorksTools/src/places"
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

// placeCandidates lists the stations and structures where owners ($1) have
// assets, blueprints, recent industry jobs or open market orders.
const placeCandidates = `
	SELECT DISTINCT id FROM (
		SELECT place_id AS id FROM lifetime_indy.assets WHERE owner_id = ANY($1)
		UNION SELECT place_id FROM lifetime_indy.blueprints WHERE owner_id = ANY($1)
		UNION SELECT facility_id FROM lifetime_indy.industry_jobs
			WHERE owner_id = ANY($1) AND end_date > now() - interval '30 days'
		UNION SELECT location_id FROM lifetime_indy.market_orders WHERE owner_id = ANY($1) AND state = 'open'
	) candidates
	WHERE id BETWEEN 60000000 AND 63999999 OR id >= 1000000000000`

// LoadPlaceCandidates returns the stations and structures the owners have
// something at, for the refresh to look up.
func LoadPlaceCandidates(ctx context.Context, ownerIDs []int64) ([]int64, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx, placeCandidates, ownerIDs)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

const placeColumns = `location_id, kind, accessible, name, type_name, owner_name, solar_system, region, checked_at`

func scanPlace(row pgx.CollectableRow) (places.Place, error) {
	var p places.Place
	err := row.Scan(&p.LocationID, &p.Kind, &p.Accessible, &p.Name, &p.TypeName, &p.OwnerName, &p.System,
		&p.Region, &p.CheckedAt)
	return p, err
}

// LoadPlaces returns the saved places among locationIDs, keyed by ID.
func LoadPlaces(ctx context.Context, locationIDs []int64) (map[int64]places.Place, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx,
		`SELECT `+placeColumns+` FROM lifetime_indy.places WHERE location_id = ANY($1)`, locationIDs)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, scanPlace)
	if err != nil {
		return nil, err
	}
	found := make(map[int64]places.Place, len(list))
	for _, p := range list {
		found[p.LocationID] = p
	}
	return found, nil
}

// SavePlace adds or updates a place.
func SavePlace(ctx context.Context, p places.Place) error {
	conn, err := connectDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx,
		`INSERT INTO lifetime_indy.places (`+placeColumns+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (location_id) DO UPDATE SET kind = $2, accessible = $3, name = $4, type_name = $5,
		owner_name = $6, solar_system = $7, region = $8, checked_at = $9`,
		p.LocationID, p.Kind, p.Accessible, p.Name, p.TypeName, p.OwnerName, p.System, p.Region, p.CheckedAt)
	return err
}

// SearchOwnerPlaces returns up to limit named stations and structures where
// the owners have something, whose name contains query, ignoring case.
func SearchOwnerPlaces(ctx context.Context, ownerIDs []int64, query string, limit int) ([]places.Place, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx,
		`SELECT `+placeColumns+` FROM lifetime_indy.places
		WHERE location_id IN (`+placeCandidates+`) AND name <> '' AND name ILIKE '%' || $2 || '%'
		ORDER BY lower(name) LIMIT $3`,
		ownerIDs, escapeLike(strings.TrimSpace(query)), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanPlace)
}

// escapeLike stops % and _ in a search from acting as wildcards.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
