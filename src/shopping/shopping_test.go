package shopping

import (
	"MeadowWorksTools/src/stockpile"
	"reflect"
	"strings"
	"testing"
)

var types = map[string]Type{
	"tritanium":             {ID: 34, Name: "Tritanium", Category: "Material", Group: "Mineral"},
	"pyerite":               {ID: 35, Name: "Pyerite", Category: "Material", Group: "Mineral"},
	"cadmium":               {ID: 16643, Name: "Cadmium", Category: "Material", Group: "Moon Materials"},
	"coolant":               {ID: 9832, Name: "Coolant", Category: "Planetary Commodities", Group: "Refined Commodities"},
	"r.a.m.- starship tech": {ID: 11478, Name: "R.A.M.- Starship Tech", Category: "Commodity", Group: "Tool"},
	"agent 47":              {ID: 1, Name: "Agent 47", Category: "Apparel", Group: "Test"},
}

func find(name string) (Type, bool) {
	t, ok := types[strings.ToLower(strings.TrimSpace(name))]
	return t, ok
}

func TestParse(t *testing.T) {
	text := "Tritanium 1,000\r\n" +
		"\n" +
		"coolant x25\n" +
		"Pyerite\t2,500\tMineral\t\t25 m3\n" +
		"Cadmium\t\tMoon Materials\n" +
		"Not An Item 5\n" +
		"R.A.M.- Starship Tech\n" +
		"Agent 47\n" +
		"Tritanium 500\n" +
		"Tritanium 1.5k\n"
	got := Parse(text, find)
	want := []Line{
		{TypeID: 1, Name: "Agent 47", Quantity: 1, Category: "Apparel", Group: "Test"},
		{TypeID: 11478, Name: "R.A.M.- Starship Tech", Quantity: 1, Category: "Commodity", Group: "Tool"},
		{TypeID: 34, Name: "Tritanium", Quantity: 1500, Category: "Material", Group: "Mineral"},
		{TypeID: 35, Name: "Pyerite", Quantity: 2500, Category: "Material", Group: "Mineral"},
		{TypeID: 16643, Name: "Cadmium", Quantity: 1, Category: "Material", Group: "Moon Materials"},
		{TypeID: 9832, Name: "Coolant", Quantity: 25, Category: "Planetary Commodities", Group: "Refined Commodities"},
		{Name: "Not An Item 5", Bad: true, Text: "Not An Item 5"},
		{Name: "Tritanium 1.5k", Bad: true, Text: "Tritanium 1.5k"},
	}
	// Pyerite before Tritanium by name within Mineral
	want[2], want[3] = want[3], want[2]
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse =\n%+v\nwant\n%+v", got, want)
	}
	if got := Parse(" \n\n", find); len(got) != 0 {
		t.Errorf("Parse of blank lines = %v", got)
	}
}

func TestMissing(t *testing.T) {
	list := []stockpile.Stockpile{
		{StockpileId: 1, Items: []stockpile.Item{
			{TypeId: 34, Kind: stockpile.KindItem, Quantity: 1000},
			{TypeId: 35, Kind: stockpile.KindItem, Quantity: 100},
			{TypeId: 688, Kind: stockpile.KindBlueprintRuns, Quantity: 10},
		}},
		{StockpileId: 2, Items: []stockpile.Item{{TypeId: 34, Kind: stockpile.KindItem, Quantity: 300}}},
		{StockpileId: 3, Items: []stockpile.Item{{TypeId: 36, Kind: stockpile.KindItem, Quantity: 5}}},
	}
	counts := map[string]int64{"1-34": 400, "1-35": 250, "2-34": 0}
	got := Missing(list, map[int64]bool{1: true, 2: true}, counts)
	want := map[int64]int64{34: 900}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Missing = %v, want %v", got, want)
	}
}
