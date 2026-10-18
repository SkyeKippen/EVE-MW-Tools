package mining

import (
	"sort"
	"strings"
	"time"
)

// gradeYield is how much more a graded ore refines into than its base ore.
var gradeYield = map[string]float64{
	"II-Grade":  1.05,
	"III-Grade": 1.10,
	"IV-Grade":  1.15,
}

// GradeMultiplier returns how much a unit of the ore refines into compared
// with its base ore: 1.05 for II-Grade, 1.10 for III-Grade, 1.15 for
// IV-Grade and 1 for anything else.
func GradeMultiplier(name string) float64 {
	if match := gradeSuffix.FindString(name); match != "" {
		if multiplier, ok := gradeYield[strings.TrimSpace(match)]; ok {
			return multiplier
		}
	}
	return 1
}

// RefiningYield is the best share of an ore's materials refining gives in
// EVE, with max skills, implant and structure bonuses.
const RefiningYield = 0.90628

// Recipe is what one unit of an ore refines into at RefiningYield:
// material type ID to quantity.
type Recipe map[int64]float64

// NewRecipe works out what one unit of an ore refines into from what a
// portion of its base ore refines into, applying the ore's grade bonus and
// RefiningYield.
func NewRecipe(oreName string, baseMaterials map[int64]int64, portionSize int64) Recipe {
	multiplier := GradeMultiplier(oreName)
	recipe := make(Recipe, len(baseMaterials))
	for typeID, quantity := range baseMaterials {
		recipe[typeID] = float64(quantity) * multiplier * RefiningYield / float64(max(portionSize, 1))
	}
	return recipe
}

// DatedPrice is a material's price on one day.
type DatedPrice struct {
	Date  time.Time
	Price float64
}

// PriceBook holds each material's saved daily prices, oldest first.
type PriceBook map[int64][]DatedPrice

// Closest returns the material's price on the saved day nearest to date,
// taking the earlier day on a tie, and false when it has no prices at all.
func (book PriceBook) Closest(typeID int64, date time.Time) (float64, bool) {
	prices := book[typeID]
	if len(prices) == 0 {
		return 0, false
	}
	i := sort.Search(len(prices), func(i int) bool { return !prices[i].Date.Before(date) })
	switch {
	case i == len(prices):
		return prices[i-1].Price, true
	case i == 0 || prices[i].Date.Equal(date):
		return prices[i].Price, true
	case date.Sub(prices[i-1].Date) <= prices[i].Date.Sub(date):
		return prices[i-1].Price, true
	default:
		return prices[i].Price, true
	}
}

// UnitValue is what one unit of ore refines into, valued at the prices
// closest to date, and false when a material has no price.
func (recipe Recipe) UnitValue(book PriceBook, date time.Time) (float64, bool) {
	value := 0.0
	for typeID, quantity := range recipe {
		price, ok := book.Closest(typeID, date)
		if !ok {
			return 0, false
		}
		value += quantity * price
	}
	return value, true
}

// MinedDay is how much of one ore type was mined on one day, all
// characters together.
type MinedDay struct {
	TypeID   int64
	OreType  string
	BaseOre  string
	Date     time.Time
	Quantity int64
}

// AddValues sets each total's Value to what its ore refined into was worth
// on the days it was mined. Mining of an ore type with no recipe, or with a
// material that has no price, is left out of Value and counted in
// UnpricedQuantity.
func AddValues(totals []OreTotal, days []MinedDay, recipes map[int64]Recipe, book PriceBook) {
	index := make(map[string]int, len(totals))
	for i, total := range totals {
		index[total.BaseOre] = i
	}
	for _, day := range days {
		i, ok := index[day.BaseOre]
		if !ok {
			continue
		}
		recipe, ok := recipes[day.TypeID]
		value, priced := 0.0, false
		if ok {
			value, priced = recipe.UnitValue(book, day.Date)
		}
		if priced {
			totals[i].Value += value * float64(day.Quantity)
		} else {
			totals[i].UnpricedQuantity += day.Quantity
		}
	}
}

// AddVolumes sets each total's Volume to the m3 mined, each ore type at its
// own volume per unit. An ore type with no known volume adds nothing.
func AddVolumes(totals []OreTotal, days []MinedDay, volumes map[int64]float64) {
	index := make(map[string]int, len(totals))
	for i, total := range totals {
		index[total.BaseOre] = i
	}
	for _, day := range days {
		if i, ok := index[day.BaseOre]; ok {
			totals[i].Volume += volumes[day.TypeID] * float64(day.Quantity)
		}
	}
}
