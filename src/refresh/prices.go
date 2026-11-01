package refresh

import (
	"MeadowWorksTools/src/db"
	"MeadowWorksTools/src/esi"
	"MeadowWorksTools/src/mining"
	"MeadowWorksTools/src/prices"
	"MeadowWorksTools/src/sde"
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"time"
)

// OreRecipes works out what one unit of each ore refines into, keyed by the
// ore's type ID: its base ore's refine materials from the SDE, with the
// grade bonus added. Something mined that can't be refined, such as gas, is
// valued as itself: its recipe is one unit of its own type, so it gets its
// own Jita sell price. Types that neither refine nor trade on the market are
// left out. It reads only the SDE.
func OreRecipes(oreTypes map[int64]string) (map[int64]mining.Recipe, error) {
	materials, err := sde.Materials()
	if err != nil {
		return nil, err
	}
	recipes := make(map[int64]mining.Recipe, len(oreTypes))
	for typeID, name := range oreTypes {
		baseID, ok := sde.TypeIDByName(mining.BaseOreName(name))
		if !ok || len(materials[baseID]) == 0 {
			if len(materials[typeID]) == 0 && sde.OnMarket(typeID) {
				recipes[typeID] = mining.Recipe{typeID: 1}
			}
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

// saveDailyPrices saves the day's prices once, after downtime: every item's
// Jita sell price for valuing things, and buy and sell prices in Jita 4-4,
// the C-J6MT Keepstar and PLEX's market. Blueprints' Jita sell prices are
// capped at their NPC price, which needs the SDE, so it waits for the SDE to
// load. Each part is skipped when it's already saved for the day.
func (r *Refresher) saveDailyPrices(ctx context.Context) error {
	day := PriceDay(time.Now())
	latest, err := db.LatestPriceDay(ctx)
	if err != nil {
		return err
	}
	jitaDay, err := db.LatestMarketPriceDay(ctx, prices.MarketJita)
	if err != nil {
		return err
	}
	plexDay, err := db.LatestMarketPriceDay(ctx, prices.MarketPlex)
	if err != nil {
		return err
	}
	if latest.Before(day) || jitaDay.Before(day) || plexDay.Before(day) {
		if err := r.saveJitaAndPlex(ctx, day, latest.Before(day), jitaDay.Before(day), plexDay.Before(day)); err != nil {
			return err
		}
	}
	if err := r.saveCJPrices(ctx, day); err != nil {
		log.Printf("refresh: C-J6MT prices: %v", err)
	}
	if err := r.savePackagedVolumes(ctx); err != nil {
		log.Printf("refresh: ships' packaged volumes: %v", err)
	}
	return nil
}

// saveJitaAndPlex fetches Jita 4-4's and PLEX's order books and saves the
// parts asked for.
func (r *Refresher) saveJitaAndPlex(ctx context.Context, day time.Time, valuing, jita, plex bool) error {
	if _, err := sde.Materials(); err != nil {
		return fmt.Errorf("waiting for the SDE to price blueprints: %w", err)
	}
	start := time.Now()
	daily, err := prices.FetchDaily(ctx, r.ESI)
	if err != nil {
		return err
	}
	if valuing {
		list := prices.CapBlueprints(daily.JitaSellPrices(), func(typeId int64) (bool, float64) {
			item, ok := sde.Get(typeId)
			return ok && item.IsBlueprint, item.BasePrice
		})
		if err := db.SaveDailyJitaPrices(ctx, day, list); err != nil {
			return fmt.Errorf("saving prices: %w", err)
		}
		log.Printf("refresh: saved Jita prices of %d types for %s", len(list), day.Format(time.DateOnly))
	}
	save := func(market string, orders []esi.RegionOrder) error {
		list := prices.Summarize(orders, daily.Averages)
		if err := db.SaveDailyMarketPrices(ctx, market, day, list); err != nil {
			return fmt.Errorf("saving %s prices: %w", prices.MarketNames[market], err)
		}
		log.Printf("refresh: saved %s buy and sell prices of %d types for %s", prices.MarketNames[market],
			len(list), day.Format(time.DateOnly))
		return nil
	}
	if jita {
		if err := save(prices.MarketJita, daily.Jita); err != nil {
			return err
		}
	}
	if plex {
		if err := save(prices.MarketPlex, daily.Plex); err != nil {
			return err
		}
	}
	log.Printf("refresh: daily prices took %s", time.Since(start).Round(time.Second))
	return nil
}

// saveCJPrices saves the day's buy and sell prices in the C-J6MT Keepstar.
// A structure's market can only be read by a character with docking access
// who granted ScopeStructureMarkets, so it tries each such character until
// one is let in. With no such character it does nothing.
func (r *Refresher) saveCJPrices(ctx context.Context, day time.Time) error {
	latest, err := db.LatestMarketPriceDay(ctx, prices.MarketCJ)
	if err != nil || !latest.Before(day) {
		return err
	}
	characters, err := db.ListTokenCharacters(ctx)
	if err != nil {
		return err
	}
	var lastErr error
	for _, character := range characters {
		if !character.HasScope(ScopeStructureMarkets) {
			continue
		}
		token, err := db.GetAccessToken(ctx, r.SSO, character.CharacterID)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", character.CharacterName, err)
			continue
		}
		orders, err := r.ESI.GetStructureOrders(ctx, prices.CJKeepstarId, token)
		if errors.Is(err, esi.ErrForbidden) {
			lastErr = fmt.Errorf("%s can't see the market there", character.CharacterName)
			continue
		}
		if err != nil {
			return err
		}
		averages, err := r.ESI.GetAveragePrices(ctx)
		if err != nil {
			return fmt.Errorf("fetching average prices: %w", err)
		}
		list := prices.Summarize(orders, averages)
		if err := db.SaveDailyMarketPrices(ctx, prices.MarketCJ, day, list); err != nil {
			return err
		}
		log.Printf("refresh: saved C-J6MT buy and sell prices of %d types for %s, read by %s", len(list),
			day.Format(time.DateOnly), character.CharacterName)
		return nil
	}
	return lastErr // nil when no character granted the scope
}

// savePackagedVolumes fetches the packaged volume of each market ship that
// doesn't have one saved yet, for hauling costs. Usually that's none, or the
// few ships added by a patch.
func (r *Refresher) savePackagedVolumes(ctx context.Context) error {
	ships := sde.MarketShips()
	if len(ships) == 0 {
		return nil // the SDE isn't loaded yet
	}
	saved, err := db.LoadPackagedVolumes(ctx)
	if err != nil {
		return err
	}
	var count int
	for _, typeID := range ships {
		if _, ok := saved[typeID]; ok {
			continue
		}
		volume, err := r.ESI.GetPackagedVolume(ctx, typeID)
		if err != nil {
			return fmt.Errorf("type %d: %w", typeID, err)
		}
		if err := db.SavePackagedVolume(ctx, typeID, volume); err != nil {
			return err
		}
		count++
	}
	if count > 0 {
		log.Printf("refresh: saved the packaged volumes of %d ships", count)
	}
	return nil
}
