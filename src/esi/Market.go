package esi

import "context"

// Jita is where ore values are priced: The Forge region and the Jita IV -
// Moon 4 - Caldari Navy Assembly Plant station.
const (
	TheForgeRegionId = 10000002
	JitaStationId    = 60003760
)

// PLEX trades on EVE's global PLEX market, not in Jita, so its price comes
// from that market's region.
const (
	PlexTypeId         = 44992
	GlobalPlexRegionId = 19000001
)

// GetJitaSellPrice returns the lowest sell order for the type at the Jita
// 4-4 station, and false when nobody is selling it there. PLEX is priced
// from the lowest sell order on the global PLEX market instead. No scope
// needed.
func (c *Client) GetJitaSellPrice(ctx context.Context, typeId int64) (float64, bool, error) {
	region, station := int64(TheForgeRegionId), int64(JitaStationId)
	if typeId == PlexTypeId {
		region, station = GlobalPlexRegionId, 0
	}
	orders, err := getPages[struct {
		LocationId int64   `json:"location_id"`
		Price      float64 `json:"price"`
	}](ctx, c, c.url("/markets/%d/orders?order_type=sell&type_id=%d", region, typeId), "")
	if err != nil {
		return 0, false, err
	}
	lowest, found := 0.0, false
	for _, order := range orders {
		if (station == 0 || order.LocationId == station) && (!found || order.Price < lowest) {
			lowest, found = order.Price, true
		}
	}
	return lowest, found, nil
}
