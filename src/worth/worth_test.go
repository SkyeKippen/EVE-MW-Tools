package worth

import "testing"

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
