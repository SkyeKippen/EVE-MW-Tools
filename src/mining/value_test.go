package mining

import (
	"math"
	"testing"
	"time"
)

func day(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }

func TestGradeMultiplier(t *testing.T) {
	for name, want := range map[string]float64{
		"Veldspar":                       1,
		"Veldspar II-Grade":              1.05,
		"Scordite III-Grade":             1.10,
		"Compressed Dark Ochre IV-Grade": 1.15,
		"Blue Ice":                       1,
		"Zeolites":                       1,
		"Brimful Zeolites":               1.15,
		"Compressed Glistening Bitumens": 2,
		"Lavish Chromite":                1.15,
		"Shimmering Otavite":             2,
	} {
		if got := GradeMultiplier(name); got != want {
			t.Errorf("GradeMultiplier(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestClosest(t *testing.T) {
	book := PriceBook{34: {{day(5), 5}, {day(9), 9}}}
	for d, want := range map[int]float64{1: 5, 5: 5, 6: 5, 7: 5, 8: 9, 9: 9, 20: 9} {
		if got, _ := book.Closest(34, day(d)); got != want {
			t.Errorf("Closest(day %d) = %v, want %v", d, got, want)
		}
	}
	if _, ok := book.Closest(35, day(1)); ok {
		t.Error("a material with no prices was priced")
	}
}

func TestAddValues(t *testing.T) {
	// a portion of 100 Veldspar refines into 400 Tritanium
	base := map[int64]int64{34: 400}
	recipes := map[int64]Recipe{
		1230:  NewRecipe("Veldspar", base, 100),
		17470: NewRecipe("Veldspar II-Grade", base, 100),
	}
	book := PriceBook{34: {{day(1), 2}, {day(10), 3}}}
	totals := []OreTotal{{BaseOre: "Veldspar"}, {BaseOre: "Fullerite-C50"}}
	AddValues(totals, []MinedDay{
		{TypeID: 1230, BaseOre: "Veldspar", Date: day(2), Quantity: 1000},  // 1000*4*2 = 8000 at full yield
		{TypeID: 17470, BaseOre: "Veldspar", Date: day(9), Quantity: 1000}, // 1000*4.2*3 = 12600 at full yield
		{TypeID: 30375, BaseOre: "Fullerite-C50", Date: day(9), Quantity: 50},
	}, recipes, book)

	if want := 20600 * RefiningYield; math.Abs(totals[0].Value-want) > 1e-6 || totals[0].UnpricedQuantity != 0 {
		t.Errorf("Veldspar = %v (%d unpriced), want %v", totals[0].Value, totals[0].UnpricedQuantity, want)
	}
	if totals[1].Value != 0 || totals[1].UnpricedQuantity != 50 {
		t.Errorf("gas = %v (%d unpriced), want 0 and 50 unpriced", totals[1].Value, totals[1].UnpricedQuantity)
	}
}

func TestAddVolumes(t *testing.T) {
	totals := []OreTotal{{BaseOre: "Veldspar"}, {BaseOre: "Blue Ice"}}
	AddVolumes(totals, []MinedDay{
		{TypeID: 1230, BaseOre: "Veldspar", Date: day(1), Quantity: 1000},
		{TypeID: 17470, BaseOre: "Veldspar", Date: day(2), Quantity: 500},
		{TypeID: 16264, BaseOre: "Blue Ice", Date: day(2), Quantity: 3},
	}, map[int64]float64{1230: 0.1, 17470: 0.1, 16264: 1000})
	if math.Abs(totals[0].Volume-150) > 1e-9 || totals[1].Volume != 3000 {
		t.Errorf("volumes = %v, %v; want 150, 3000", totals[0].Volume, totals[1].Volume)
	}
}
