package refresh

import (
	"MeadowWorksTools/src/db"
	"MeadowWorksTools/src/worth"
	"context"
	"fmt"
	"maps"
	"slices"
	"time"
)

// TakeSnapshots records, for the hour of now, what every character and
// corporation with a saved wallet or assets holds, from what the refresh
// saved: no ESI calls. They make the net worth graph on My Worth.
func (r *Refresher) TakeSnapshots(ctx context.Context, now time.Time) error {
	owners, err := db.LoadHoldingOwners(ctx)
	if err != nil {
		return err
	}
	takenAt := now.UTC().Truncate(time.Hour)
	for _, owner := range owners {
		liquid, quantities, err := db.LoadHoldings(ctx, []int64{owner.ID})
		if err != nil {
			return fmt.Errorf("holdings of %d: %w", owner.ID, err)
		}
		prices, err := db.LoadLatestJitaPrices(ctx, slices.Collect(maps.Keys(quantities)))
		if err != nil {
			return err
		}
		assets, plex, unpriced := worth.ValueHoldings(quantities, prices)
		split := worth.Split{Liquid: liquid, Assets: assets, Plex: plex}
		if err := db.SaveNetworthSnapshot(ctx, owner, takenAt, split, unpriced); err != nil {
			return err
		}
	}
	return nil
}
