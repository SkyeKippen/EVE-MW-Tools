// Package mining adds up what characters have mined.
package mining

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
)

// OreTotal is how much of one base ore has been mined, with its graded
// variants (e.g. "Veldspar III-Grade") counted as the base ore.
type OreTotal struct {
	BaseOre  string
	Quantity int64
	Variants []string // every type mined that counts as this ore, base included

	// Value is what the ore refined into was worth at Jita sell prices on
	// the days it was mined, leaving out UnpricedQuantity units that
	// couldn't be valued.
	Value            float64
	UnpricedQuantity int64

	// Volume is how many m3 were mined, each variant at its own volume.
	Volume float64
}

// gradeSuffix matches the grade EVE puts after a variant ore's name.
var gradeSuffix = regexp.MustCompile(`\s+[IVX]+-Grade$`)

// moonOreVariant is a prefix EVE puts before a moon ore's name for a richer
// variant, and how much more the variant refines into.
type moonOreVariant struct {
	yield float64
	ores  []string // the base moon ores of the rarity that uses this prefix
}

// Each moon ore rarity has its own pair of variant prefixes: the first
// gives +15%, the second +100%.
var moonOreVariants = func() map[string]moonOreVariant {
	variants := map[string]moonOreVariant{}
	for _, rarity := range []struct {
		plus15, plus100 string
		ores            []string
	}{
		{"Brimful", "Glistening", []string{"Bitumens", "Coesite", "Sylvite", "Zeolites"}},     // R4
		{"Copious", "Twinkling", []string{"Cobaltite", "Euxenite", "Scheelite", "Titanite"}},  // R8
		{"Lavish", "Shimmering", []string{"Chromite", "Otavite", "Sperrylite", "Vanadinite"}}, // R16
		{"Replete", "Glowing", []string{"Carnotite", "Cinnabar", "Pollucite", "Zircon"}},      // R32
		{"Bountiful", "Shining", []string{"Loparite", "Monazite", "Xenotime", "Ytterbite"}},   // R64
	} {
		variants[rarity.plus15] = moonOreVariant{yield: 1.15, ores: rarity.ores}
		variants[rarity.plus100] = moonOreVariant{yield: 2, ores: rarity.ores}
	}
	return variants
}()

// uncompressed drops the compression EVE puts before an ore's name.
func uncompressed(name string) string {
	name = strings.TrimPrefix(name, "Batch Compressed ")
	return strings.TrimPrefix(name, "Compressed ")
}

// moonVariant splits a moon ore variant's name, e.g. "Glistening Zeolites",
// into its base ore and variant, and false for any other ore.
func moonVariant(name string) (string, moonOreVariant, bool) {
	prefix, base, found := strings.Cut(uncompressed(name), " ")
	variant, ok := moonOreVariants[prefix]
	if !found || !ok || !slices.Contains(variant.ores, base) {
		return "", moonOreVariant{}, false
	}
	return base, variant, true
}

// BaseOreName turns an ore's name into its base ore, e.g.
// "Compressed Veldspar III-Grade" into "Veldspar" and "Glistening Zeolites"
// into "Zeolites".
func BaseOreName(name string) string {
	if base, _, ok := moonVariant(name); ok {
		return base
	}
	return gradeSuffix.ReplaceAllString(uncompressed(name), "")
}

// TotalsByBaseOre adds up the quantity mined of each ore type under its base
// ore, largest first, listing the types that went into each.
func TotalsByBaseOre(byType map[string]int64) []OreTotal {
	index := map[string]int{}
	var totals []OreTotal
	for oreType, quantity := range byType {
		base := BaseOreName(oreType)
		i, ok := index[base]
		if !ok {
			i = len(totals)
			index[base] = i
			totals = append(totals, OreTotal{BaseOre: base})
		}
		totals[i].Quantity += quantity
		totals[i].Variants = append(totals[i].Variants, oreType)
	}
	for i := range totals {
		slices.Sort(totals[i].Variants)
	}
	slices.SortFunc(totals, func(a, b OreTotal) int {
		if a.Quantity != b.Quantity {
			return cmp.Compare(b.Quantity, a.Quantity)
		}
		return strings.Compare(a.BaseOre, b.BaseOre)
	})
	return totals
}
