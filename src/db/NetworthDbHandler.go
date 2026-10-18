package db

import (
	"MeadowWorksTools/src/worth"
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// HoldingOwner is a character or corporation with a saved wallet or assets.
type HoldingOwner struct {
	ID   int64
	Type string
}

// LoadHoldingOwners returns every character and corporation with a saved
// wallet balance or assets, for the hourly net worth snapshots.
func LoadHoldingOwners(ctx context.Context) ([]HoldingOwner, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx,
		`SELECT owner_id, owner_type FROM lifetime_indy.wallet_balances
		UNION
		SELECT DISTINCT owner_id, owner_type FROM lifetime_indy.assets`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (HoldingOwner, error) {
		var owner HoldingOwner
		err := row.Scan(&owner.ID, &owner.Type)
		return owner, err
	})
}

// SaveNetworthSnapshot records what an owner held at takenAt, replacing a
// snapshot already taken then.
func SaveNetworthSnapshot(ctx context.Context, owner HoldingOwner, takenAt time.Time, split worth.Split, unpricedTypes int) error {
	conn, err := connectDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx,
		`INSERT INTO lifetime_indy.networth_snapshots (owner_id, owner_type, taken_at, liquid, assets, plex, unpriced_types)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (owner_id, taken_at) DO UPDATE SET owner_type = $2, liquid = $4, assets = $5, plex = $6,
		unpriced_types = $7`,
		owner.ID, owner.Type, takenAt, split.Liquid, split.Assets, split.Plex, unpricedTypes)
	return err
}

// LoadNetworthSnapshots returns every snapshot of the owners, oldest first.
func LoadNetworthSnapshots(ctx context.Context, ownerIDs []int64) ([]worth.Snapshot, error) {
	conn, err := connectDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx,
		`SELECT owner_id, taken_at, liquid::float8, assets::float8, plex::float8
		FROM lifetime_indy.networth_snapshots WHERE owner_id = ANY($1)
		ORDER BY taken_at, owner_id`, ownerIDs)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (worth.Snapshot, error) {
		var s worth.Snapshot
		err := row.Scan(&s.OwnerID, &s.TakenAt, &s.Liquid, &s.Assets, &s.Plex)
		return s, err
	})
}
