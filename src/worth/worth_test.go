package worth

import (
	"testing"
	"time"
)

func TestSplit(t *testing.T) {
	s := Split{Liquid: 50, Assets: 30, Plex: 20}
	if s.Total() != 100 || s.Percent(s.Assets) != 30 {
		t.Errorf("total %v, assets %v%%", s.Total(), s.Percent(s.Assets))
	}
	if (Split{}).Percent(0) != 0 {
		t.Error("an empty split should be 0%")
	}
}

func TestValueHoldings(t *testing.T) {
	quantities := map[int64]int64{34: 1000, PlexTypeID: 10, 99: 5}
	prices := map[int64]float64{34: 4.5, PlexTypeID: 5_000_000}
	assets, plex, unpriced := ValueHoldings(quantities, prices)
	if assets != 4500 || plex != 50_000_000 || unpriced != 1 {
		t.Errorf("got assets %v, plex %v, unpriced %d", assets, plex, unpriced)
	}
}

func TestSeriesCarriesForwardMissingSnapshots(t *testing.T) {
	h0 := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	h1 := h0.Add(time.Hour)
	points := Series([]Snapshot{
		{OwnerID: 1, TakenAt: h0, Split: Split{Liquid: 10, Assets: 100}},
		{OwnerID: 2, TakenAt: h0, Split: Split{Liquid: 50, Assets: 1000, Plex: 5}},
		{OwnerID: 1, TakenAt: h1, Split: Split{Liquid: 20, Assets: 120}},
		// owner 2 couldn't be read at h1
	})
	want := []Point{
		{Time: h0, Liquid: 60, Assets: 1100, Plex: 5, NetWorth: 1165},
		{Time: h1, Liquid: 70, Assets: 1120, Plex: 5, NetWorth: 1195},
	}
	if len(points) != len(want) {
		t.Fatalf("got %d points, want %d", len(points), len(want))
	}
	for i := range want {
		if points[i] != want[i] {
			t.Errorf("point %d = %+v, want %+v", i, points[i], want[i])
		}
	}
}
