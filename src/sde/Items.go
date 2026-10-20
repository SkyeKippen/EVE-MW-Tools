package sde

import (
	"sort"
	"strings"
)

// blueprintCategoryID is the SDE category of blueprints and reaction
// formulas.
const blueprintCategoryID = 9

// Item is a published type, as offered by item search.
type Item struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	IsBlueprint bool    `json:"isBlueprint"` // a blueprint or reaction formula
	BasePrice   float64 `json:"-"`           // what NPCs sell it for; 0 when unknown

	lower string
}

// indexItems lists the published types for Get, Find and Search. It needs
// the groups loaded first, to tell blueprints apart.
func (data *sdeData) indexItems(types map[int64]TypeInfo) {
	data.byName = make(map[string]*Item, len(types))
	for typeID, info := range types {
		if !info.Published || info.Name == "" {
			continue
		}
		item := &Item{ID: typeID, Name: info.Name, BasePrice: info.BasePrice, lower: strings.ToLower(info.Name),
			IsBlueprint: data.groups[info.GroupID].CategoryID == blueprintCategoryID}
		data.items[typeID] = item
		data.sorted = append(data.sorted, item)
		if other, taken := data.byName[item.lower]; !taken || item.ID < other.ID {
			data.byName[item.lower] = item
		}
	}
	sort.Slice(data.sorted, func(i, j int) bool { return data.sorted[i].lower < data.sorted[j].lower })
}

// Get returns the published type with that ID, and false when there's none
// or the SDE isn't loaded.
func Get(typeID int64) (Item, bool) {
	mu.Lock()
	defer mu.Unlock()
	item, ok := loaded.items[typeID]
	if !ok {
		return Item{}, false
	}
	return *item, true
}

// Find returns the published type with that name, ignoring case and
// surrounding spaces, and false when there's none.
func Find(name string) (Item, bool) {
	mu.Lock()
	defer mu.Unlock()
	item, ok := loaded.byName[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return Item{}, false
	}
	return *item, true
}

// TypeName returns a published type's name, or "" when it's unknown.
func TypeName(typeID int64) string {
	item, _ := Get(typeID)
	return item.Name
}

// Search returns up to limit published types whose name contains query,
// ignoring case. Names starting with query come first, then names with a
// word starting with it, then the rest; shorter names first within each.
// blueprintsOnly limits it to blueprints and reaction formulas.
func Search(query string, blueprintsOnly bool, limit int) ([]Item, error) {
	mu.Lock()
	sorted, err := loaded.sorted, loadErr
	mu.Unlock()
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" || limit <= 0 {
		return []Item{}, nil
	}

	type match struct {
		item *Item
		rank int
	}
	var matches []match
	for _, item := range sorted {
		if blueprintsOnly && !item.IsBlueprint {
			continue
		}
		i := strings.Index(item.lower, q)
		if i < 0 {
			continue
		}
		rank := 2
		if i == 0 {
			rank = 0
		} else if strings.Contains(item.lower, " "+q) {
			rank = 1
		}
		matches = append(matches, match{item, rank})
	}
	// sorted is already alphabetical, so a stable sort keeps ties in order
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].rank != matches[j].rank {
			return matches[i].rank < matches[j].rank
		}
		return len(matches[i].item.Name) < len(matches[j].item.Name)
	})

	out := make([]Item, 0, min(limit, len(matches)))
	for _, m := range matches[:min(limit, len(matches))] {
		out = append(out, *m.item)
	}
	return out, nil
}
