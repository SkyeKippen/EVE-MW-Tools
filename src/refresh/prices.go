package refresh

import (
	"MeadowWorksTools/src/db"
	"MeadowWorksTools/src/mining"
	"MeadowWorksTools/src/prices"
	"MeadowWorksTools/src/sde"
	"context"
	"fmt"
	"log"
	"slices"
	"time"
)

// OreRecipes works out what one unit of each ore refines into, keyed by the
// ore's type ID: its base ore's refine materials from the SDE, with the
// grade bonus added. Ores whose base can't be refined, such as gas, are left
// out. It reads only the SDE.
func OreRecipes(oreTypes map[int64]string) (map[int64]mining.Recipe, error) {
	materials, err := sde.Materials()
	if err != nil {
		return nil, err
	}
	recipes := make(map[int64]mining.Recipe, len(oreTypes))
	for typeID, name := range oreTypes {
		baseID, ok := sde.TypeIDByName(mining.BaseOreName(name))
		if !ok || len(materials[baseID]) == 0 {
			continue
		}
		quantities := make(map[int64]int64, len(materials[baseID]))
		for _, material := range materials[baseID] {
			quantities[material.TypeID] += material.Quantity
		}
		recipes[typeID] = mining.NewRecipe(name, quantities, sde.PortionSize(baseID))
	}
	return recipes, nil
}

// MaterialIDs returns every material the recipes refine into.
func MaterialIDs(recipes map[int64]mining.Recipe) []int64 {
	var ids []int64
	for _, recipe := range recipes {
		for typeID := range recipe {
			if !slices.Contains(ids, typeID) {
				ids = append(ids, typeID)
			}
		}
	}
	slices.Sort(ids)
	return ids
}

// Jita prices are fetched once a day, after downtime (11:00 to about 11:15
// UTC), when the market has settled.
const (
	priceFetchHourUTC   = 11
	priceFetchMinuteUTC = 30
)

// PriceDay is the UTC day prices fetched at t are saved under. Before the
// daily fetch time they belong to the day before, so a restart in the
// morning doesn't fetch again before downtime.
func PriceDay(t time.Time) time.Time {
	t = t.UTC()
	if t.Hour() < priceFetchHourUTC || t.Hour() == priceFetchHourUTC && t.Minute() < priceFetchMinuteUTC {
		t = t.AddDate(0, 0, -1)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// saveDailyPrices saves the day's Jita sell price of every item on the
// market, unless they're already saved. Blueprints are capped at their NPC
// price, which needs the SDE, so it waits for the SDE to load.
func (r *Refresher) saveDailyPrices(ctx context.Context) error {
	day := PriceDay(time.Now())
	latest, err := db.LatestPriceDay(ctx)
	if err != nil || !latest.Before(day) {
		return err
	}
	if _, err := sde.Materials(); err != nil {
		return fmt.Errorf("waiting for the SDE to price blueprints: %w", err)
	}

	start := time.Now()
	list, err := prices.FetchJita(ctx, r.ESI)
	if err != nil {
		return err
	}
	list = prices.CapBlueprints(list, func(typeId int64) (bool, float64) {
		item, ok := sde.Get(typeId)
		return ok && item.IsBlueprint, item.BasePrice
	})
	if err := db.SaveDailyJitaPrices(ctx, day, list); err != nil {
		return fmt.Errorf("saving prices: %w", err)
	}
	log.Printf("refresh: saved Jita prices of %d types for %s in %s", len(list), day.Format(time.DateOnly),
		time.Since(start).Round(time.Second))
	return nil
}
