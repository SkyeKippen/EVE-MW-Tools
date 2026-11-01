// Package imports works out the Import page: what a pasted fit or Multibuy
// list costs bought in Jita and hauled to C-J6MT, against buying it in the
// C-J6MT Keepstar, and where each item is cheaper.
package imports

import (
	"MeadowWorksTools/src/shopping"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// DefaultShipping is the hauling rate from Jita to C-J6MT, in ISK per m3.
const DefaultShipping = 700

// eftHeader is a fit's first line in EFT format, "[Hull, Fit name]".
var eftHeader = regexp.MustCompile(`^\[([^,\]]+),[^\]]*\]$`)

// Parse reads a fit in EFT format, as copied from the fitting window, or a
// list in EVE's Multibuy format. For a fit, the hull is included when
// includeHull is true, empty slots are skipped, and a module loaded with a
// charge ("Heavy Missile Launcher II, Scourge Heavy Missile") counts the
// module only; charges to bring list separately, as the fitting window does
// under the cargo. Every quantity is multiplied by batch. It also returns the
// hull's name, or "" when the text isn't a fit.
func Parse(text string, includeHull bool, batch int64, find func(name string) (shopping.Type, bool)) ([]shopping.Line, string) {
	text = strings.ReplaceAll(text, "\r", "")
	var hull string
	var list []string
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if hull == "" && len(list) == 0 {
			if m := eftHeader.FindStringSubmatch(line); m != nil {
				hull = strings.TrimSpace(m[1])
				if includeHull {
					list = append(list, hull)
				}
				continue
			}
		}
		if hull != "" {
			if strings.HasPrefix(line, "[") { // [Empty High slot] and the like
				continue
			}
			line = strings.TrimSpace(strings.TrimSuffix(line, "/OFFLINE"))
			if module, _, loaded := strings.Cut(line, ","); loaded && !strings.Contains(line, "\t") {
				line = strings.TrimSpace(module)
			}
		}
		list = append(list, line)
	}
	lines := shopping.Parse(strings.Join(list, "\n"), find)
	if batch > 1 {
		for i := range lines {
			lines[i].Quantity *= batch
		}
	}
	return lines, hull
}

// Where an item is cheaper to get.
const (
	BuyJita = "jita" // buy in Jita and haul it
	BuyCJ   = "cj"   // buy in C-J6MT
	BuyNone = ""     // neither market has it
)

// Item is one row of the Import page, per unit and in total. Jita and CJ
// are sell prices, nil where that market has none.
type Item struct {
	shopping.Line
	Jita      *float64 // Jita sell price per unit
	CJ        *float64 // C-J6MT sell price per unit
	Volume    float64  // m3 per unit, packaged for ships
	Haul      float64  // hauling cost per unit
	Landed    *float64 // Jita price plus hauling, per unit
	BuyAt     string
	Best      float64 // total at the cheaper market; 0 for BuyNone
	Saving    float64 // total saved by buying at BuyAt, when both have it
	TotalVol  float64
	TotalHaul float64
}

// Prices are what Calculate needs to price items, by type ID.
type Prices struct {
	Jita    map[int64]float64 // Jita 4-4 sell prices
	CJ      map[int64]float64 // C-J6MT sell prices
	Volumes map[int64]float64 // m3 per unit to haul
}

// Result is the whole Import page.
type Result struct {
	Items []Item
	Bad   []shopping.Line // lines that name no item
	// Totals over the items each market has
	JitaTotal   float64 // bought in Jita
	HaulTotal   float64 // hauling what's bought in Jita
	LandedTotal float64 // bought in Jita and hauled
	CJTotal     float64 // bought in C-J6MT
	// what buying each item at its cheaper market costs, and how much of it
	BestTotal  float64
	BestHaul   float64 // hauling what's cheaper bought in Jita
	Volume     float64 // m3 of everything
	JitaVolume float64 // m3 of what's cheaper bought in Jita
	// items missing from a market, so its total leaves them out
	MissingJita, MissingCJ, Unpriced int
	// Multibuy lists of what to buy at each market
	JitaList, CJList string
}

// Calculate prices lines at each market with shipping ISK per m3 added to
// what's bought in Jita, and picks the cheaper market for each item. An item
// only one market has is bought there.
func Calculate(lines []shopping.Line, shipping float64, p Prices) Result {
	var r Result
	var jitaList, cjList []string
	for _, line := range lines {
		if line.Bad {
			r.Bad = append(r.Bad, line)
			continue
		}
		q := float64(line.Quantity)
		it := Item{Line: line, Volume: p.Volumes[line.TypeID]}
		it.Haul = it.Volume * shipping
		it.TotalVol = it.Volume * q
		it.TotalHaul = it.Haul * q
		r.Volume += it.TotalVol
		var landed, cj float64
		if price, ok := p.Jita[line.TypeID]; ok {
			it.Jita = &price
			landed = price + it.Haul
			it.Landed = &landed
			r.JitaTotal += price * q
			r.HaulTotal += it.TotalHaul
			r.LandedTotal += landed * q
		} else {
			r.MissingJita++
		}
		if price, ok := p.CJ[line.TypeID]; ok {
			it.CJ = &price
			cj = price
			r.CJTotal += price * q
		} else {
			r.MissingCJ++
		}
		switch {
		case it.Landed != nil && (it.CJ == nil || landed <= cj):
			it.BuyAt, it.Best = BuyJita, landed*q
			if it.CJ != nil {
				it.Saving = (cj - landed) * q
			}
			r.BestHaul += it.TotalHaul
			r.JitaVolume += it.TotalVol
			jitaList = append(jitaList, fmt.Sprintf("%s %d", line.Name, line.Quantity))
		case it.CJ != nil:
			it.BuyAt, it.Best = BuyCJ, cj*q
			if it.Landed != nil {
				it.Saving = (landed - cj) * q
			}
			cjList = append(cjList, fmt.Sprintf("%s %d", line.Name, line.Quantity))
		default:
			r.Unpriced++
		}
		r.BestTotal += it.Best
		r.Items = append(r.Items, it)
	}
	slices.Sort(jitaList)
	slices.Sort(cjList)
	r.JitaList = strings.Join(jitaList, "\n")
	r.CJList = strings.Join(cjList, "\n")
	return r
}
