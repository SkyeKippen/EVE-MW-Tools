package main

import (
	"LifetimeIndustryTracker/src/auth"
	"LifetimeIndustryTracker/src/db"
	"LifetimeIndustryTracker/src/worth"
	"context"
	"log"
	"maps"
	"net/http"
	"slices"
	"time"
)

// WorthPart is one cell of the My Worth table: an amount and its share of
// the row's total.
type WorthPart struct {
	Amount  float64
	Percent float64
}

// WorthRow is one line of the My Worth table.
type WorthRow struct {
	Label  string
	Total  float64
	Liquid WorthPart
	Assets WorthPart
	Plex   WorthPart
}

func newWorthRow(label string, split worth.Split) WorthRow {
	return WorthRow{Label: label, Total: split.Total(),
		Liquid: WorthPart{split.Liquid, split.Percent(split.Liquid)},
		Assets: WorthPart{split.Assets, split.Percent(split.Assets)},
		Plex:   WorthPart{split.Plex, split.Percent(split.Plex)},
	}
}

func renderMyWorth(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.CurrentSession(r)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// wallets, assets and prices are saved by the hourly refresh; the page
	// only reads what's saved
	linked, err := db.ListUserCharacters(ctx, sess.UserID)
	if err != nil {
		log.Printf("renderMyWorth: characters of user %d: %v", sess.UserID, err)
		http.Error(w, "could not load your characters", http.StatusInternalServerError)
		return
	}
	characterIDs := make([]int64, 0, len(linked))
	for _, character := range linked {
		characterIDs = append(characterIDs, character.CharacterID)
	}
	corporationIDs, err := db.LoadUserCorporations(ctx, sess.UserID)
	if err != nil {
		log.Printf("renderMyWorth: corporations of user %d: %v", sess.UserID, err)
		http.Error(w, "could not load your corporations: "+err.Error(), http.StatusInternalServerError)
		return
	}

	earned, spent, since, err := db.LoadIskFlows(ctx, characterIDs, corporationIDs)
	if err != nil {
		log.Printf("renderMyWorth: ISK earned and spent: %v", err)
		http.Error(w, "could not load saved wallets: "+err.Error(), http.StatusInternalServerError)
		return
	}

	owners := append(append([]int64{}, characterIDs...), corporationIDs...)
	liquid, quantities, err := db.LoadHoldings(ctx, owners)
	if err != nil {
		log.Printf("renderMyWorth: holdings: %v", err)
		http.Error(w, "could not load saved assets: "+err.Error(), http.StatusInternalServerError)
		return
	}
	prices, err := db.LoadLatestJitaPrices(ctx, slices.Collect(maps.Keys(quantities)))
	if err != nil {
		log.Printf("renderMyWorth: prices: %v", err)
		http.Error(w, "could not load saved prices: "+err.Error(), http.StatusInternalServerError)
		return
	}
	assets, plex, unpriced := worth.ValueHoldings(quantities, prices)
	netWorth := worth.Split{Liquid: liquid, Assets: assets, Plex: plex}

	data := struct {
		CharacterName string
		Rows          []WorthRow
		Since         time.Time
		Corporations  int
		Unpriced      int
	}{CharacterName: sess.CharacterName,
		Rows: []WorthRow{
			newWorthRow("All-time ISK earned", earned),
			newWorthRow("All-time ISK spent", spent),
			newWorthRow("Current net worth", netWorth),
		},
		Since: since, Corporations: len(corporationIDs), Unpriced: unpriced}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "myworth.html", data); err != nil {
		log.Printf("renderMyWorth: %v", err)
	}
}
