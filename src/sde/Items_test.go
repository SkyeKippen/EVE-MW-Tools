package sde

import "testing"

func TestSearch(t *testing.T) {
	data := sdeData{items: map[int64]*Item{}, groups: map[int64]Group{
		18: {Name: "Mineral", CategoryID: 4}, 105: {Name: "Frigate Blueprint", CategoryID: blueprintCategoryID}}}
	data.indexItems(map[int64]TypeInfo{
		34:  {Name: "Tritanium", GroupID: 18, Published: true},
		35:  {Name: "Pyerite", GroupID: 18, Published: true},
		688: {Name: "Rifter Blueprint", GroupID: 105, Published: true, BasePrice: 285000},
		99:  {Name: "Tritanium Test", GroupID: 18},
	})
	setLoaded(data, nil)
	defer func() {
		mu.Lock()
		loaded, loadErr = sdeData{}, ErrNotLoaded
		mu.Unlock()
	}()

	found, err := Search("rit", false, 10)
	if err != nil || len(found) != 2 || found[0].Name != "Pyerite" || found[1].Name != "Tritanium" {
		t.Errorf("search rit = %v, %v", found, err)
	}
	if found, _ := Search("ri", true, 10); len(found) != 1 || !found[0].IsBlueprint {
		t.Errorf("blueprint search = %v", found)
	}
	if item, ok := Find("  tritanium "); !ok || item.ID != 34 {
		t.Errorf("Find tritanium = %v, %v", item, ok)
	}
	if _, ok := Find("Tritanium Test"); ok {
		t.Error("Find found an unpublished type")
	}
	if item, ok := Get(688); !ok || item.BasePrice != 285000 || TypeName(99) != "" {
		t.Errorf("Get = %v, %v", item, ok)
	}
}
