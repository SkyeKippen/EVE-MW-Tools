// Package places works out which station or structure each asset is in,
// following it out of containers, ships and corporation offices, so pages
// can count what's at a place from the saved assets alone.
package places

import (
	"MeadowWorksTools/src/esi"
	"strings"
	"time"
)

// Place kinds.
const (
	KindStation   = "station"
	KindStructure = "structure"
)

// Place is a station or structure, as saved by the refresh.
type Place struct {
	LocationID int64     `json:"id"`
	Kind       string    `json:"kind"` // KindStation or KindStructure
	Accessible bool      `json:"-"`    // false when no character could read the structure
	Name       string    `json:"name"`
	TypeName   string    `json:"type"`
	OwnerName  string    `json:"owner"`
	System     string    `json:"system"`
	Region     string    `json:"region"`
	CheckedAt  time.Time `json:"-"`
}

// How long a saved place is trusted before it's looked up again. Refusals
// count toward ESI's error limit, so unreadable structures aren't asked
// about on every refresh.
const (
	RecheckAfter             = 30 * 24 * time.Hour
	RecheckInaccessibleAfter = 24 * time.Hour
)

// IsStale reports whether the place should be looked up again.
func (p Place) IsStale(now time.Time) bool {
	if p.Accessible {
		return now.Sub(p.CheckedAt) >= RecheckAfter
	}
	return now.Sub(p.CheckedAt) >= RecheckInaccessibleAfter
}

// maxHops stops a broken chain of containers from looping forever.
const maxHops = 10

// IsStation is true for NPC station IDs.
func IsStation(id int64) bool {
	return id >= 60_000_000 && id < 64_000_000
}

// IsSolarSystem is true for solar system IDs.
func IsSolarSystem(id int64) bool {
	return id >= 30_000_000 && id < 33_000_000
}

// IsStructure is true for IDs in the range player structures use. Item IDs
// share the range, so only use it on IDs known not to be an asset.
func IsStructure(id int64) bool {
	return id >= 1_000_000_000_000
}

// Of returns the station or structure each of one owner's assets is in,
// keyed by item ID, or 0 for items in space or whose place can't be told.
// assets must be the owner's whole list, so containers can be followed.
func Of(assets []esi.Asset) map[int64]int64 {
	byID := make(map[int64]esi.Asset, len(assets))
	for _, asset := range assets {
		byID[asset.ItemId] = asset
	}
	places := make(map[int64]int64, len(assets))
	for _, asset := range assets {
		places[asset.ItemId] = walk(asset, byID)
	}
	return places
}

func walk(asset esi.Asset, byID map[int64]esi.Asset) int64 {
	id, flag := asset.LocationId, asset.LocationFlag
	for range maxHops {
		if IsStation(id) {
			return id
		}
		if IsSolarSystem(id) {
			return 0
		}
		parent, found := byID[id]
		if !found {
			// the parent isn't one of the owner's items, so it's the
			// station or structure itself
			if IsStructure(id) {
				return id
			}
			return 0
		}
		// a structure the corporation owns is itself one of its assets,
		// anchored in a solar system; what led here was a hangar or office,
		// not a ship's or container's contents
		if IsSolarSystem(parent.LocationId) && isHangarFlag(flag) {
			return id
		}
		id, flag = parent.LocationId, parent.LocationFlag
	}
	return 0
}

// OfBlueprint is where a blueprint is: where its asset entry is, or
// otherwise wherever its location leads. assetPlaces is Of for the owner's
// assets. A location that isn't one of the owner's items is only taken as a
// structure when the blueprint sits in a hangar there, since item IDs look
// like structure IDs.
func OfBlueprint(itemID, locationID int64, flag string, assetPlaces map[int64]int64) int64 {
	if place, ok := assetPlaces[itemID]; ok {
		return place
	}
	if place, ok := assetPlaces[locationID]; ok {
		return place
	}
	if IsStation(locationID) || (IsStructure(locationID) && isHangarFlag(flag)) {
		return locationID
	}
	return 0
}

// IsFitted is true for modules fitted to a ship, which aren't spare stock
// even when the ship is parked at the place.
func IsFitted(flag string) bool {
	return strings.Contains(flag, "Slot")
}

// isHangarFlag is true for flags whose location is a station, structure or
// corporation office: a character's hangar, a corporation hangar division or
// an office.
func isHangarFlag(flag string) bool {
	switch flag {
	case "Hangar", "Deliveries", "CorpDeliveries", "HangarAll", "OfficeFolder",
		"CorpSAG1", "CorpSAG2", "CorpSAG3", "CorpSAG4", "CorpSAG5", "CorpSAG6", "CorpSAG7":
		return true
	}
	return false
}
