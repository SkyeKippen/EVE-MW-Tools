// Package worth splits ISK amounts into liquid ISK, assets and PLEX, for the
// My Worth page.
package worth

// PlexTypeID is PLEX's type ID. PLEX is counted on its own, never as an
// asset.
const PlexTypeID = 44992

// Split is an amount of ISK split by what it is in.
type Split struct {
	Liquid float64 // ISK itself
	Assets float64 // items, at their Jita sell price
	Plex   float64 // PLEX, at its sell price
}

// Total is the three parts together.
func (s Split) Total() float64 {
	return s.Liquid + s.Assets + s.Plex
}

// Percent gives part as a percentage of the total, or 0 when the total is 0.
func (s Split) Percent(part float64) float64 {
	total := s.Total()
	if total == 0 {
		return 0
	}
	return part / total * 100
}

// ValueHoldings prices how many of each type are held. PLEX goes into plex
// and everything else into assets. Types with no price are left out and
// counted in unpriced.
func ValueHoldings(quantities map[int64]int64, prices map[int64]float64) (assets, plex float64, unpriced int) {
	for typeID, quantity := range quantities {
		price, ok := prices[typeID]
		switch {
		case !ok:
			unpriced++
		case typeID == PlexTypeID:
			plex += float64(quantity) * price
		default:
			assets += float64(quantity) * price
		}
	}
	return assets, plex, unpriced
}
