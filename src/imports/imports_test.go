package imports

import (
	"MeadowWorksTools/src/shopping"
	"strings"
	"testing"
)

var types = map[string]shopping.Type{
	"drake":                                {ID: 24698, Name: "Drake", Category: "Ship"},
	"heavy missile launcher ii":            {ID: 2410, Name: "Heavy Missile Launcher II", Category: "Module"},
	"scourge heavy missile":                {ID: 209, Name: "Scourge Heavy Missile", Category: "Charge"},
	"large shield extender ii":             {ID: 3841, Name: "Large Shield Extender II", Category: "Module"},
	"ballistic control system ii":          {ID: 22291, Name: "Ballistic Control System II", Category: "Module"},
	"medium core defense field extender i": {ID: 31788, Name: "Medium Core Defense Field Extender I", Category: "Module"},
}

func find(name string) (shopping.Type, bool) {
	t, ok := types[strings.ToLower(strings.TrimSpace(name))]
	return t, ok
}

const fit = `[Drake, PvE Drake]
Ballistic Control System II
Ballistic Control System II
[Empty Low slot]

Large Shield Extender II /OFFLINE

Heavy Missile Launcher II, Scourge Heavy Missile
Heavy Missile Launcher II, Scourge Heavy Missile

Medium Core Defense Field Extender I


Scourge Heavy Missile x1000
Mystery Thing x3
`

func quantities(lines []shopping.Line) map[string]int64 {
	q := map[string]int64{}
	for _, l := range lines {
		q[l.Name] = l.Quantity
	}
	return q
}

func TestParseFit(t *testing.T) {
	lines, hull := Parse(fit, true, 2, find)
	if hull != "Drake" {
		t.Errorf("hull = %q", hull)
	}
	got := quantities(lines)
	want := map[string]int64{
		"Drake": 2, "Ballistic Control System II": 4, "Large Shield Extender II": 2,
		"Heavy Missile Launcher II": 4, "Medium Core Defense Field Extender I": 2,
		"Scourge Heavy Missile": 2000, "Mystery Thing x3": 0, // a bad line
	}
	for name, q := range want {
		if got[name] != q {
			t.Errorf("%s: %d, want %d", name, got[name], q)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %v", got)
	}

	lines, _ = Parse(fit, false, 1, find)
	if _, ok := quantities(lines)["Drake"]; ok {
		t.Error("hull included when it shouldn't be")
	}
}

func TestParseMultibuy(t *testing.T) {
	lines, hull := Parse("Drake 2\r\nScourge Heavy Missile x500\n", true, 3, find)
	if hull != "" {
		t.Errorf("hull = %q for a list", hull)
	}
	got := quantities(lines)
	if got["Drake"] != 6 || got["Scourge Heavy Missile"] != 1500 {
		t.Errorf("got %v", got)
	}
}

func TestCalculate(t *testing.T) {
	lines := []shopping.Line{
		{TypeID: 1, Name: "Cheap in Jita", Quantity: 10},
		{TypeID: 2, Name: "Bulky", Quantity: 1},
		{TypeID: 3, Name: "Only C-J", Quantity: 2},
		{TypeID: 4, Name: "Only Jita", Quantity: 1},
		{TypeID: 5, Name: "Nowhere", Quantity: 1},
		{Name: "junk", Bad: true, Text: "junk"},
	}
	p := Prices{
		Jita:    map[int64]float64{1: 100, 2: 1000, 4: 50},
		CJ:      map[int64]float64{1: 200, 2: 1500, 3: 30},
		Volumes: map[int64]float64{1: 0.1, 2: 1, 4: 1},
	}
	r := Calculate(lines, 700, p)
	if len(r.Items) != 5 || len(r.Bad) != 1 {
		t.Fatalf("%d items, %d bad", len(r.Items), len(r.Bad))
	}
	byName := map[string]Item{}
	for _, it := range r.Items {
		byName[it.Name] = it
	}
	// 100 + 70 hauling = 170 < 200
	if it := byName["Cheap in Jita"]; it.BuyAt != BuyJita || it.Best != 1700 || it.Saving != 300 {
		t.Errorf("cheap in Jita: %+v", it)
	}
	// 1000 + 700 = 1700 > 1500
	if it := byName["Bulky"]; it.BuyAt != BuyCJ || it.Best != 1500 || it.Saving != 200 {
		t.Errorf("bulky: %+v", it)
	}
	if it := byName["Only C-J"]; it.BuyAt != BuyCJ || it.Best != 60 {
		t.Errorf("only C-J: %+v", it)
	}
	if it := byName["Only Jita"]; it.BuyAt != BuyJita || it.Best != 750 {
		t.Errorf("only Jita: %+v", it)
	}
	if it := byName["Nowhere"]; it.BuyAt != BuyNone {
		t.Errorf("nowhere: %+v", it)
	}
	if r.BestTotal != 1700+1500+60+750 || r.Unpriced != 1 || r.MissingJita != 2 || r.MissingCJ != 2 {
		t.Errorf("totals: %+v", r)
	}
	if r.JitaList != "Cheap in Jita 10\nOnly Jita 1" || r.CJList != "Bulky 1\nOnly C-J 2" {
		t.Errorf("lists: %q, %q", r.JitaList, r.CJList)
	}
}
