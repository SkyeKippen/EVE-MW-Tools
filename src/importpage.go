package main

import (
	"MeadowWorksTools/src/db"
	"MeadowWorksTools/src/imports"
	"MeadowWorksTools/src/prices"
	"MeadowWorksTools/src/sde"
	"MeadowWorksTools/src/shopping"
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// The Import page prices a pasted fit or Multibuy list bought in Jita and
// hauled to C-J6MT, against buying it in the C-J6MT Keepstar, from the
// prices the daily refresh saved.

// maxImportBatch caps how many copies of a fit the page multiplies by.
const maxImportBatch = 10_000

// marketDay is the day a market's prices are from, for the page to show.
type marketDay struct {
	Name string
	Date time.Time // zero when none are saved
}

func renderImport(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Text        string
		Batch       int64
		Shipping    float64
		IncludeHull bool
		Hull        string
		Posted      bool
		Result      imports.Result
		Jita, CJ    marketDay
		Error       string
	}{
		Batch:       1,
		Shipping:    imports.DefaultShipping,
		IncludeHull: true,
		Jita:        marketDay{Name: prices.MarketNames[prices.MarketJita]},
		CJ:          marketDay{Name: prices.MarketNames[prices.MarketCJ]},
	}

	if r.Method == http.MethodPost {
		data.Posted = true
		data.Text = strings.ReplaceAll(r.FormValue("text"), "\r\n", "\n")
		data.IncludeHull = r.FormValue("include_hull") != ""
		if n, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("batch")), 10, 64); err == nil && n >= 1 {
			data.Batch = min(n, maxImportBatch)
		}
		if f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(r.FormValue("shipping")), ",", ""), 64); err == nil && f >= 0 {
			data.Shipping = f
		}
		if len(data.Text) > maxShoppingListLength {
			data.Error = "That's too long to price."
		} else {
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			lines, hull := imports.Parse(data.Text, data.IncludeHull, data.Batch, findType)
			data.Hull = hull
			p, jitaDay, cjDay, err := loadImportPrices(ctx, shopping.TypeIDs(lines))
			if err != nil {
				log.Printf("renderImport: %v", err)
				http.Error(w, "could not load prices", http.StatusInternalServerError)
				return
			}
			data.Jita.Date, data.CJ.Date = jitaDay, cjDay
			data.Result = imports.Calculate(lines, data.Shipping, p)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "import.html", data); err != nil {
		log.Printf("renderImport: %v", err)
	}
}

// loadImportPrices loads the newest saved Jita 4-4 and C-J6MT sell prices
// of the types, and the volume to haul each: packaged for ships.
func loadImportPrices(ctx context.Context, typeIDs []int64) (imports.Prices, time.Time, time.Time, error) {
	p := imports.Prices{Jita: map[int64]float64{}, CJ: map[int64]float64{}, Volumes: map[int64]float64{}}
	latest, err := db.LoadLatestMarketPrices(ctx, typeIDs)
	if err != nil {
		return p, time.Time{}, time.Time{}, err
	}
	var jitaDay, cjDay time.Time
	for typeID, price := range latest[prices.MarketJita] {
		jitaDay = price.Date
		if price.HasSell {
			p.Jita[typeID] = price.Sell
		}
	}
	for typeID, price := range latest[prices.MarketCJ] {
		cjDay = price.Date
		if price.HasSell {
			p.CJ[typeID] = price.Sell
		}
	}
	if jitaDay.IsZero() {
		jitaDay, err = db.LatestMarketPriceDay(ctx, prices.MarketJita)
	}
	if err == nil && cjDay.IsZero() {
		cjDay, err = db.LatestMarketPriceDay(ctx, prices.MarketCJ)
	}
	if err != nil {
		return p, jitaDay, cjDay, err
	}
	packaged, err := db.LoadPackagedVolumes(ctx)
	if err != nil {
		return p, jitaDay, cjDay, err
	}
	volumes, _ := sde.Volumes() // nil until the SDE loads, like the items found
	for _, typeID := range typeIDs {
		if v, ok := packaged[typeID]; ok {
			p.Volumes[typeID] = v
		} else {
			p.Volumes[typeID] = volumes[typeID]
		}
	}
	return p, jitaDay, cjDay, nil
}
