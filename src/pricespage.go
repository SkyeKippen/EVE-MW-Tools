package main

import (
	"MeadowWorksTools/src/db"
	"MeadowWorksTools/src/prices"
	"MeadowWorksTools/src/sde"
	"context"
	"log"
	"net/http"
	"strconv"
	"time"
)

// The Prices page shows one item's newest buy and sell prices in Jita 4-4,
// the C-J6MT Keepstar and PLEX's market, and a chart of them over time with
// CCP's daily market history, all from what the background refresh saved.

// plexTypeID is PLEX, the one item traded on PLEX's market.
const plexTypeID = 44992

// priceRanges are the chart's periods, in days; 0 is everything saved.
var priceRanges = []struct {
	Days  int
	Label string
}{{30, "30 days"}, {90, "90 days"}, {365, "1 year"}, {0, "All"}}

// pricePoint is one saved day's prices in one market, for the chart.
type pricePoint struct {
	Date   string   `json:"date"`
	Market string   `json:"market"`
	Sell   *float64 `json:"sell"`
	Buy    *float64 `json:"buy"`
}

func renderPrices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	days := 90
	if n, err := strconv.Atoi(q.Get("days")); err == nil && n >= 0 {
		days = n
	}
	data := struct {
		TypeID int64
		Name   string
		Days   int
		Ranges []struct {
			Days  int
			Label string
		}
		Markets     []db.DatedMarketPrice
		MarketNames map[string]string
		Missing     []string // markets with no saved price for the item
		Region      string
		Chart       any
		Error       string
	}{
		Days:        days,
		Ranges:      priceRanges,
		MarketNames: prices.MarketNames,
	}

	if id, err := strconv.ParseInt(q.Get("type_id"), 10, 64); err == nil {
		if item, ok := sde.Get(id); ok {
			data.TypeID, data.Name = item.ID, item.Name
		} else {
			data.Error = "That item isn't known, or EVE's static data is still loading."
		}
	} else if name := q.Get("name"); name != "" {
		if item, ok := sde.Find(name); ok {
			data.TypeID, data.Name = item.ID, item.Name
		} else {
			data.Error = "No item is called " + strconv.Quote(name) + "."
		}
	}

	if data.TypeID != 0 {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		chart, err := loadPriceChart(ctx, data.TypeID, days)
		if err != nil {
			log.Printf("renderPrices: type %d: %v", data.TypeID, err)
			http.Error(w, "could not load prices", http.StatusInternalServerError)
			return
		}
		data.Markets, data.Missing = chart.latest, chart.missing
		data.Region = chart.region
		data.Chart = chart.chart
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "prices.html", data); err != nil {
		log.Printf("renderPrices: %v", err)
	}
}

type priceChart struct {
	latest  []db.DatedMarketPrice // each market's newest saved day
	missing []string
	region  string
	chart   any // embedded in the page as JSON for prices.js
}

// loadPriceChart loads a type's saved daily prices and CCP's history for
// it over the last days days, or all of it for 0. History is The Forge's,
// or PLEX's market for PLEX.
func loadPriceChart(ctx context.Context, typeID int64, days int) (priceChart, error) {
	var c priceChart
	since := time.Time{}
	if days > 0 {
		since = time.Now().UTC().AddDate(0, 0, -days)
	}
	saved, err := db.LoadMarketPriceHistory(ctx, typeID)
	if err != nil {
		return c, err
	}
	newest := map[string]db.DatedMarketPrice{}
	points := []pricePoint{}
	for _, p := range saved {
		newest[p.Market] = p
		if p.Date.Before(since) {
			continue
		}
		point := pricePoint{Date: p.Date.Format(time.DateOnly), Market: p.Market}
		if p.HasSell {
			point.Sell = &p.Sell
		}
		if p.HasBuy {
			point.Buy = &p.Buy
		}
		points = append(points, point)
	}
	markets := []string{prices.MarketJita, prices.MarketCJ}
	region := int64(prices.TheForgeRegionId)
	c.region = "The Forge"
	if typeID == plexTypeID {
		markets = []string{prices.MarketPlex}
		region = prices.PlexRegionId
		c.region = "PLEX's market"
	}
	for _, market := range markets {
		if p, ok := newest[market]; ok {
			c.latest = append(c.latest, p)
		} else {
			c.missing = append(c.missing, prices.MarketNames[market])
		}
	}

	history, err := db.LoadMarketHistory(ctx, typeID, region, since)
	if err != nil {
		return c, err
	}
	if history == nil {
		history = []db.HistoryDay{}
	}
	c.chart = map[string]any{
		"history": history,
		"points":  points,
		"names":   prices.MarketNames,
		"region":  c.region,
	}
	return c, nil
}
