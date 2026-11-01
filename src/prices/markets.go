package prices

import (
	"MeadowWorksTools/src/esi"
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
)

// The markets saved each day with buy and sell prices.
const (
	MarketJita   = "jita"  // Jita IV - Moon 4 - Caldari Navy Assembly Plant
	MarketCJ     = "cj6mt" // the C-J6MT Keepstar, 1st Taj Mahgoon
	MarketPlex   = "plex"  // PLEX's global market
	CJKeepstarId = 1049588174021
)

// MarketNames are the markets' names as pages show them.
var MarketNames = map[string]string{
	MarketJita: "Jita 4-4",
	MarketCJ:   "C-J6MT",
	MarketPlex: "PLEX market",
}

// MarketPrice is one item type's prices in one market on one day. Sell is
// the volume-weighted average of the cheapest 5% of units for sale and Buy
// the same of the highest-paying 5% of units wanted. HasSell and HasBuy are
// false when the type had no orders on that side.
type MarketPrice struct {
	TypeId     int64
	HasSell    bool
	Sell       float64
	MinSell    float64 // the cheapest sell order
	SellVolume int64   // units for sale
	SellOrders int
	HasBuy     bool
	Buy        float64
	MaxBuy     float64 // the best buy order
	BuyVolume  int64   // units wanted
	BuyOrders  int
}

// Daily is the day's order books, fetched once after downtime.
type Daily struct {
	Jita     []esi.RegionOrder // every buy and sell order in Jita 4-4
	Plex     []esi.RegionOrder // every PLEX order
	Averages map[int64]float64 // CCP's average traded price by type
}

// FetchDaily fetches every order in The Forge, keeping Jita 4-4's, every
// order on PLEX's global market and CCP's average prices.
func FetchDaily(ctx context.Context, client *esi.Client) (Daily, error) {
	forge, err := client.GetRegionOrders(ctx, TheForgeRegionId, "all")
	if err != nil {
		return Daily{}, fmt.Errorf("fetching The Forge orders: %w", err)
	}
	jita := slices.DeleteFunc(forge, func(o esi.RegionOrder) bool { return o.LocationId != Jita44StationId })
	plex, err := client.GetRegionOrders(ctx, PlexRegionId, "all")
	if err != nil {
		return Daily{}, fmt.Errorf("fetching PLEX orders: %w", err)
	}
	averages, err := client.GetAveragePrices(ctx)
	if err != nil {
		return Daily{}, fmt.Errorf("fetching average prices: %w", err)
	}
	return Daily{Jita: jita, Plex: plex, Averages: averages}, nil
}

// JitaSellPrices is every type's Jita sell price for valuing things: Jita
// 4-4's and PLEX's sell orders, capped at averageCap times CCP's average
// price.
func (d Daily) JitaSellPrices() []Price {
	return CapAtAverage(SellPrices(append(slices.Clone(d.Jita), d.Plex...)), d.Averages)
}

// Summarize works out each type's buy and sell prices in one market. Sell
// prices are capped at averageCap times CCP's average price, as for
// JitaSellPrices.
func Summarize(orders []esi.RegionOrder, averages map[int64]float64) []MarketPrice {
	byType := map[int64]*MarketPrice{}
	get := func(typeId int64) *MarketPrice {
		p, ok := byType[typeId]
		if !ok {
			p = &MarketPrice{TypeId: typeId}
			byType[typeId] = p
		}
		return p
	}
	for _, s := range CapAtAverage(SellPrices(orders), averages) {
		p := get(s.TypeId)
		p.HasSell, p.Sell, p.MinSell, p.SellVolume, p.SellOrders = true, s.SellPrice, s.MinPrice, s.Volume, s.OrderCount
	}
	for _, b := range BuyPrices(orders) {
		p := get(b.TypeId)
		p.HasBuy, p.Buy, p.MaxBuy, p.BuyVolume, p.BuyOrders = true, b.SellPrice, b.MinPrice, b.Volume, b.OrderCount
	}
	list := make([]MarketPrice, 0, len(byType))
	for _, p := range byType {
		list = append(list, *p)
	}
	slices.SortFunc(list, func(a, b MarketPrice) int { return cmp.Compare(a.TypeId, b.TypeId) })
	return list
}

// BuyPrices combines each type's buy orders into one price: the
// volume-weighted average of the highest-paying 5% of units wanted. In the
// returned Prices, SellPrice holds that buy price and MinPrice the best
// (highest) buy order.
func BuyPrices(orders []esi.RegionOrder) []Price {
	byType := map[int64][]esi.RegionOrder{}
	for _, o := range orders {
		if o.IsBuyOrder && o.VolumeRemain > 0 {
			byType[o.TypeId] = append(byType[o.TypeId], o)
		}
	}
	prices := make([]Price, 0, len(byType))
	for typeId, typeOrders := range byType {
		slices.SortFunc(typeOrders, func(a, b esi.RegionOrder) int { return cmp.Compare(b.Price, a.Price) })
		prices = append(prices, topShare(typeId, typeOrders))
	}
	slices.SortFunc(prices, func(a, b Price) int { return cmp.Compare(a.TypeId, b.TypeId) })
	return prices
}

// topShare averages the first cheapestShare of units in orders, which are
// sorted best first.
func topShare(typeId int64, orders []esi.RegionOrder) Price {
	var volume int64
	for _, o := range orders {
		volume += o.VolumeRemain
	}
	target := max(1, int64(math.Ceil(float64(volume)*cheapestShare)))
	var counted int64
	var total float64
	for _, o := range orders {
		take := min(o.VolumeRemain, target-counted)
		total += float64(take) * o.Price
		counted += take
		if counted >= target {
			break
		}
	}
	return Price{
		TypeId:     typeId,
		SellPrice:  math.Round(total/float64(counted)*100) / 100,
		MinPrice:   orders[0].Price,
		Volume:     volume,
		OrderCount: len(orders),
	}
}
