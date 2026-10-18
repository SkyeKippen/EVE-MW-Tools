package sde

import (
	"strings"
	"testing"
)

func TestParseTypeMaterials(t *testing.T) {
	got, err := ParseTypeMaterials(strings.NewReader(
		`{"_key":1230,"materials":[{"materialTypeID":34,"quantity":400}]}` + "\n" +
			`{"_key":18,"materials":[{"materialTypeID":34,"quantity":175},{"materialTypeID":35,"quantity":70}]}` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1230][0] != (Material{34, 400}) || got[18][1] != (Material{35, 70}) {
		t.Errorf("got %v", got)
	}
}

func TestParseTypes(t *testing.T) {
	got, err := ParseTypes(strings.NewReader(
		`{"_key":1230,"groupID":462,"marketGroupID":518,"name":{"en":"Veldspar"},"volume":0.1,"portionSize":100}` + "\n" +
			`{"_key":28432,"groupID":1,"volume":0.001}` + "\n" + `{"_key":5,"groupID":2}` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || *got[1230].Volume != 0.1 || got[1230].GroupID != 462 || got[5].Volume != nil ||
		got[1230].Name != "Veldspar" || got[1230].PortionSize != 100 || got[1230].MarketGroupID != 518 ||
		got[5].MarketGroupID != 0 {
		t.Errorf("got %v", got)
	}
}

func TestParseGroupsAndCategories(t *testing.T) {
	groups, err := ParseGroups(strings.NewReader(`{"_key":1136,"categoryID":4,"name":{"de":"Treibstoffblock","en":"Fuel Block"},"published":true}` + "\n"))
	if err != nil || groups[1136] != (Group{Name: "Fuel Block", CategoryID: 4}) {
		t.Errorf("groups = %v, %v", groups, err)
	}
	categories, err := ParseCategories(strings.NewReader(`{"_key":4,"name":{"en":"Material"}}` + "\n"))
	if err != nil || categories[4] != "Material" {
		t.Errorf("categories = %v, %v", categories, err)
	}
}

func TestParseBlueprints(t *testing.T) {
	got, err := ParseBlueprints(strings.NewReader(
		`{"_key":1201,"activities":{"copying":{"time":480},"manufacturing":{"materials":[{"quantity":10,"typeID":34}],"products":[{"quantity":1,"typeID":1200}],"time":600}}}` + "\n" +
			`{"_key":11284,"activities":{"manufacturing":{"products":[{"quantity":100,"typeID":2629}]}}}` + "\n" +
			`{"_key":46166,"activities":{"reaction":{"products":[{"quantity":200,"typeID":16670}]}}}` + "\n" +
			`{"_key":99,"activities":{"research_time":{"time":5}}}` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[1201].Manufacturing[1200] != 1 || got[11284].Manufacturing[2629] != 100 ||
		got[46166].Reaction[16670] != 200 || got[46166].Manufacturing != nil {
		t.Errorf("got %v", got)
	}
}
