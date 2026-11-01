package db

import (
	"MeadowWorksTools/src/prices"
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// TestMarketPrices saves two days of prices in two markets and reads them
// back. It needs TEST_DATABASE_URL pointing at a database the tools/db
// scripts have been run on; its market_prices table is emptied.
func TestMarketPrices(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	t.Setenv("DATABASE_URL", url)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `TRUNCATE lifetime_indy.market_prices`); err != nil {
		t.Fatal(err)
	}

	day1 := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	day2 := day1.AddDate(0, 0, 1)
	if latest, err := LatestMarketPriceDay(ctx, prices.MarketJita); err != nil || !latest.IsZero() {
		t.Fatalf("latest with none saved: %v, %v", latest, err)
	}
	jita := []prices.MarketPrice{
		{TypeId: 34, HasSell: true, Sell: 4.5, MinSell: 4.2, SellVolume: 1000, SellOrders: 3,
			HasBuy: true, Buy: 4.1, MaxBuy: 4.15, BuyVolume: 500, BuyOrders: 2},
		{TypeId: 35, HasBuy: true, Buy: 9, MaxBuy: 9, BuyVolume: 1, BuyOrders: 1},
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(SaveDailyMarketPrices(ctx, prices.MarketJita, day1, jita))
	jita[0].Sell = 5
	must(SaveDailyMarketPrices(ctx, prices.MarketJita, day2, jita))
	must(SaveDailyMarketPrices(ctx, prices.MarketJita, day2, jita)) // saving again replaces
	must(SaveDailyMarketPrices(ctx, prices.MarketCJ, day1, []prices.MarketPrice{{TypeId: 34, HasSell: true, Sell: 7, MinSell: 6}}))

	latest, err := LatestMarketPriceDay(ctx, prices.MarketJita)
	if err != nil || !latest.Equal(day2) {
		t.Errorf("latest Jita day = %v, %v", latest, err)
	}
	got, err := LoadLatestMarketPrices(ctx, []int64{34, 35})
	must(err)
	if p := got[prices.MarketJita][34]; p.Sell != 5 || !p.HasBuy || p.BuyVolume != 500 || !p.Date.Equal(day2) {
		t.Errorf("Jita 34: %+v", p)
	}
	if p := got[prices.MarketJita][35]; p.HasSell || !p.HasBuy || p.Buy != 9 {
		t.Errorf("Jita 35: %+v", p)
	}
	if p := got[prices.MarketCJ][34]; p.Sell != 7 || p.HasBuy || !p.Date.Equal(day1) {
		t.Errorf("C-J 34: %+v", p)
	}

	history, err := LoadMarketPriceHistory(ctx, 34)
	must(err)
	if len(history) != 3 || history[0].Market != prices.MarketCJ || history[2].Sell != 5 {
		t.Errorf("history: %+v", history)
	}

	must(SavePackagedVolume(ctx, 587, 2500))
	must(SavePackagedVolume(ctx, 587, 2500.5))
	volumes, err := LoadPackagedVolumes(ctx)
	must(err)
	if volumes[587] != 2500.5 {
		t.Errorf("packaged volume = %v", volumes[587])
	}
}
