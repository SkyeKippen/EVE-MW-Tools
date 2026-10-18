package places

import (
	"MeadowWorksTools/src/esi"
	"testing"
)

func TestOf(t *testing.T) {
	const structure, office, ownStructure, ship = 1_035_000_000_001, 1_100_000_000_001, 1_035_000_000_002, 1_100_000_000_002
	got := Of([]esi.Asset{
		{ItemId: 1, LocationId: 60003760, LocationFlag: "Hangar"},                // station hangar
		{ItemId: 2, LocationId: structure, LocationFlag: "Hangar"},               // structure hangar
		{ItemId: ship, LocationId: structure, LocationFlag: "Hangar"},            // a ship docked there
		{ItemId: 3, LocationId: ship, LocationFlag: "Cargo"},                     // in its cargo
		{ItemId: office, LocationId: ownStructure, LocationFlag: "OfficeFolder"}, // the corp's office
		{ItemId: ownStructure, LocationId: 30000142, LocationFlag: "AutoFit"},    // the corp's own structure
		{ItemId: 4, LocationId: office, LocationFlag: "CorpSAG2"},                // in a corp hangar there
		{ItemId: 5, LocationId: 30000142, LocationFlag: "AutoFit"},               // something in space
		{ItemId: 6, LocationId: 5, LocationFlag: "Cargo"},                        // inside it
	})
	want := map[int64]int64{1: 60003760, 2: structure, ship: structure, 3: structure, office: ownStructure,
		ownStructure: 0, 4: ownStructure, 5: 0, 6: 0}
	for item, place := range want {
		if got[item] != place {
			t.Errorf("item %d is at %d, want %d", item, got[item], place)
		}
	}
}

func TestOfBlueprint(t *testing.T) {
	assetPlaces := map[int64]int64{10: 60003760, 20: 1_035_000_000_001}
	cases := []struct {
		item, location int64
		flag           string
		want           int64
	}{
		{10, 60003760, "Hangar", 60003760},                     // its asset entry
		{11, 20, "Cargo", 1_035_000_000_001},                   // in a container that's an asset
		{12, 60008494, "Hangar", 60008494},                     // straight in a station
		{13, 1_035_000_000_002, "CorpSAG2", 1_035_000_000_002}, // in a structure's hangar
		{14, 1_100_000_000_009, "Cargo", 0},                    // in something unknown
	}
	for _, c := range cases {
		if got := OfBlueprint(c.item, c.location, c.flag, assetPlaces); got != c.want {
			t.Errorf("blueprint %d at %d = %d, want %d", c.item, c.location, got, c.want)
		}
	}
	if !IsFitted("HiSlot0") || IsFitted("Cargo") {
		t.Error("IsFitted")
	}
}
