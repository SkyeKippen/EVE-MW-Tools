// Package shopping builds the Shopping List page's tables: what checked
// stockpiles are missing, and lists pasted in EVE's Multibuy format.
package shopping

import (
	"MeadowWorksTools/src/stockpile"
	"fmt"
	"slices"
	"strings"
)

// Type is an item type as the tables show and sort it.
type Type struct {
	ID        int64
	Name      string
	Category  string // the SDE category, e.g. "Material" or "Planetary Commodities"
	Group     string // the SDE group, e.g. "Mineral" or "Moon Materials"
	Blueprint bool   // a blueprint or reaction formula, which has a different icon
}

// Line is one row of a shopping table. A bad line is one that names no item
// we know; Text is what was pasted for it.
type Line struct {
	TypeID    int64  `json:"typeId"`
	Name      string `json:"name"`
	Quantity  int64  `json:"quantity"`
	Bad       bool   `json:"bad"`
	Text      string `json:"text,omitempty"`
	Blueprint bool   `json:"blueprint,omitempty"`
	// Price is the line's quantity at the Jita sell price, or nil when the
	// item has no price.
	Price    *float64 `json:"price"`
	Category string   `json:"-"`
	Group    string   `json:"-"`
}

// Parse reads a list in EVE's Multibuy format, one item a line, as
//
//	Tritanium 1000
//	Tritanium x1000
//	Tritanium	1,000	Mineral	...   (copied from an inventory window)
//	Tritanium                        (just a name means 1)
//
// find looks an item up by name. Lines for the same item are added up, and
// lines that name no item come back as bad lines, in the order pasted.
// Blank lines are skipped. The result is sorted as Sort does.
func Parse(text string, find func(name string) (Type, bool)) []Line {
	var lines []Line
	byType := map[int64]int{} // type ID to its index in lines
	for _, raw := range strings.Split(text, "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		t, quantity, ok := parseLine(raw, find)
		if !ok {
			lines = append(lines, Line{Name: raw, Bad: true, Text: raw})
			continue
		}
		if i, seen := byType[t.ID]; seen {
			lines[i].Quantity += quantity
			continue
		}
		byType[t.ID] = len(lines)
		lines = append(lines, Line{TypeID: t.ID, Name: t.Name, Quantity: quantity, Category: t.Category, Group: t.Group, Blueprint: t.Blueprint})
	}
	Sort(lines)
	return lines
}

// parseLine reads one non-blank line: a name and a quantity, or just a name.
func parseLine(raw string, find func(name string) (Type, bool)) (Type, int64, bool) {
	if strings.Contains(raw, "\t") {
		fields := strings.Split(raw, "\t")
		t, ok := find(fields[0])
		if !ok {
			return Type{}, 0, false
		}
		// an inventory copy leaves the quantity blank for a single item
		quantity := int64(1)
		if q := strings.TrimSpace(fields[1]); q != "" {
			if quantity, ok = parseCount(q); !ok {
				return Type{}, 0, false
			}
		}
		return t, quantity, true
	}

	if i := strings.LastIndex(raw, " "); i > 0 {
		if quantity, ok := parseCount(raw[i+1:]); ok {
			if t, ok := find(raw[:i]); ok {
				return t, quantity, true
			}
		}
	}
	// a name that ends in a number, or a name alone
	t, ok := find(raw)
	return t, 1, ok
}

// parseCount reads a quantity like 1000, 1,000 or x1000.
func parseCount(s string) (int64, bool) {
	s = strings.TrimPrefix(strings.ToLower(s), "x")
	if s == "" || strings.ContainsAny(s, "kmb.") { // Multibuy takes plain numbers only
		return 0, false
	}
	return stockpile.ParseQuantity(s)
}

// Sort orders lines by category, then group, then name, with bad lines
// last in the order they came.
func Sort(lines []Line) {
	slices.SortStableFunc(lines, func(a, b Line) int {
		switch {
		case a.Bad || b.Bad:
			return compareBool(a.Bad, b.Bad)
		case a.Category != b.Category:
			return stockpile.Compare(a.Category, b.Category)
		case a.Group != b.Group:
			return stockpile.Compare(a.Group, b.Group)
		}
		return stockpile.Compare(a.Name, b.Name)
	})
}

func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// Missing adds up, by type ID, how many of each item the selected
// stockpiles are short, from counts keyed "<stockpileId>-<typeId>" as the
// Stockpiles page uses. Blueprint runs are left out, since they can't be
// bought.
func Missing(list []stockpile.Stockpile, selected map[int64]bool, counts map[string]int64) map[int64]int64 {
	missing := map[int64]int64{}
	for _, s := range list {
		if !selected[s.StockpileId] {
			continue
		}
		for _, item := range s.Items {
			if item.Kind != stockpile.KindItem {
				continue
			}
			if short := item.Quantity - counts[countKey(s.StockpileId, item.TypeId)]; short > 0 {
				missing[item.TypeId] += short
			}
		}
	}
	return missing
}

func countKey(stockpileID, typeID int64) string {
	return fmt.Sprintf("%d-%d", stockpileID, typeID)
}

// SetPrices prices each good line at its item's Jita sell price, from
// prices keyed by type ID. Lines whose item has no price are left nil.
func SetPrices(lines []Line, prices map[int64]float64) {
	for i := range lines {
		if price, ok := prices[lines[i].TypeID]; ok && !lines[i].Bad {
			total := price * float64(lines[i].Quantity)
			lines[i].Price = &total
		}
	}
}

// TypeIDs returns the type IDs of the good lines.
func TypeIDs(lines []Line) []int64 {
	ids := make([]int64, 0, len(lines))
	for _, line := range lines {
		if !line.Bad {
			ids = append(ids, line.TypeID)
		}
	}
	return ids
}
