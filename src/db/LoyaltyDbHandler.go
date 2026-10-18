package db

import (
	"MeadowWorksTools/src/esi"
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// SaveLoyaltyPoints stores the character's LP with each NPC corporation as
// that day's balance, replacing one saved earlier the same day.
func SaveLoyaltyPoints(ctx context.Context, characterID int64, date time.Time, points []esi.LoyaltyPoints) error {
	batch := &pgx.Batch{}
	for _, lp := range points {
		batch.Queue(`INSERT INTO lifetime_indy.loyalty_points (character_id, lp_corporation_id, date, loyalty_points)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (character_id, lp_corporation_id, date) DO UPDATE SET loyalty_points = $4, updated_at = now()`,
			characterID, lp.CorporationId, date, lp.LoyaltyPoints)
	}
	return sendBatch(ctx, batch)
}
