// Package stockpile describes stockpiles: named groups of items, and
// blueprint runs, a user wants at one station or structure. Each user has
// their own; db saves them.
package stockpile

import (
	"MeadowWorksTools/src/places"
	"errors"
	"strings"
	"unicode"
)

// Item kinds, stored in stockpile_items.kind.
const (
	KindItem          = "item"           // Quantity is how many of the item
	KindBlueprintRuns = "blueprint_runs" // Quantity is runs of the blueprint
)

// ErrNotFound means the stockpile or place doesn't exist, or isn't the
// user's.
var ErrNotFound = errors.New("stockpile: not found")

// Stockpile belongs to one user.
type Stockpile struct {
	StockpileId   int64
	Name          string
	Place         places.Place
	CreatedByName string
	Items         []Item // sorted by name
}

type Item struct {
	StockpileItemId int64
	TypeId          int64
	Name            string
	Kind            string // KindItem or KindBlueprintRuns
	Quantity        int64
}

// Less orders names alphanumerically, ignoring case and comparing runs of
// digits by value, so "Stockpile 2" comes before "Stockpile 10".
func Less(a, b string) bool {
	ar, br := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	i, j := 0, 0
	for i < len(ar) && j < len(br) {
		if unicode.IsDigit(ar[i]) && unicode.IsDigit(br[j]) {
			si, sj := i, j
			for i < len(ar) && unicode.IsDigit(ar[i]) {
				i++
			}
			for j < len(br) && unicode.IsDigit(br[j]) {
				j++
			}
			na := strings.TrimLeft(string(ar[si:i]), "0")
			nb := strings.TrimLeft(string(br[sj:j]), "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if ar[i] != br[j] {
			return ar[i] < br[j]
		}
		i++
		j++
	}
	if len(ar)-i != len(br)-j {
		return len(ar)-i < len(br)-j
	}
	return a < b
}

// Compare is Less as a comparison, for slices.SortFunc.
func Compare(a, b string) int {
	switch {
	case Less(a, b):
		return -1
	case Less(b, a):
		return 1
	}
	return 0
}
