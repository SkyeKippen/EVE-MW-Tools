package refresh

import (
	"MeadowWorksTools/src/db"
	"MeadowWorksTools/src/esi"
	"MeadowWorksTools/src/places"
	"MeadowWorksTools/src/sde"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// SavePlaces looks up the stations and structures where the character or
// their corporation has assets, blueprints, industry jobs or market orders,
// so stockpiles can be tied to them. Saved places are only looked up again
// once stale. Structures need the character's structures scope, and the
// character on the structure's access list.
func (r *Refresher) SavePlaces(ctx context.Context, character db.TokenCharacter, corporationID int64) error {
	owners := []int64{character.CharacterID}
	if !isNpcCorporation(corporationID) {
		owners = append(owners, corporationID)
	}
	candidates, err := db.LoadPlaceCandidates(ctx, owners)
	if err != nil {
		return err
	}
	saved, err := db.LoadPlaces(ctx, candidates)
	if err != nil {
		return err
	}

	var accessToken string
	now := time.Now()
	for _, id := range candidates {
		old, known := saved[id]
		if known && !old.IsStale(now) {
			continue
		}
		var place places.Place
		if places.IsStation(id) {
			place, err = r.station(ctx, id)
		} else {
			if !character.HasScope(ScopeStructures) {
				continue
			}
			if accessToken == "" {
				if accessToken, err = db.GetAccessToken(ctx, r.SSO, character.CharacterID); err != nil {
					return err
				}
			}
			place, err = r.structure(ctx, id, accessToken)
		}
		if err != nil {
			return fmt.Errorf("looking up %d: %w", id, err)
		}
		place.CheckedAt = now
		if !place.Accessible && known && old.Name != "" {
			// keep the last known details of a structure that stopped
			// being readable
			old.Accessible, old.CheckedAt = false, now
			place = old
		}
		if err := db.SavePlace(ctx, place); err != nil {
			return err
		}
	}
	return nil
}

func (r *Refresher) station(ctx context.Context, id int64) (places.Place, error) {
	station, err := r.ESI.GetStation(ctx, id)
	if err != nil {
		return places.Place{}, err
	}
	place := places.Place{LocationID: id, Kind: places.KindStation, Accessible: true, Name: station.Name,
		TypeName: "NPC Station", OwnerName: "NPC"}
	place.System, place.Region, err = r.system(ctx, station.SystemId)
	return place, err
}

// structure looks up a player structure; one the token can't read comes back
// inaccessible, not as an error.
func (r *Refresher) structure(ctx context.Context, id int64, accessToken string) (places.Place, error) {
	place := places.Place{LocationID: id, Kind: places.KindStructure}
	structure, err := r.ESI.GetStructure(ctx, id, accessToken)
	if errors.Is(err, esi.ErrForbidden) || errors.Is(err, esi.ErrNotFound) {
		return place, nil
	}
	if err != nil {
		return place, err
	}
	names, err := r.ESI.GetNames(ctx, []int64{structure.OwnerId})
	if err != nil {
		return place, err
	}
	place.Accessible, place.Name, place.TypeName, place.OwnerName = true, structure.Name,
		sde.TypeName(structure.TypeId), names[structure.OwnerId]
	place.System, place.Region, err = r.system(ctx, structure.SolarSystemId)
	return place, err
}

// solar systems and their regions never change, so they're looked up once
// per server run
var (
	systemsMu sync.Mutex
	systems   = map[int64][2]string{} // system ID to system and region names
)

func (r *Refresher) system(ctx context.Context, systemID int64) (name, region string, err error) {
	systemsMu.Lock()
	known, ok := systems[systemID]
	systemsMu.Unlock()
	if ok {
		return known[0], known[1], nil
	}
	system, err := r.ESI.GetSolarSystem(ctx, systemID)
	if err != nil {
		return "", "", err
	}
	constellation, err := r.ESI.GetConstellation(ctx, system.ConstellationId)
	if err != nil {
		return "", "", err
	}
	names, err := r.ESI.GetNamesCached(ctx, []int64{constellation.RegionId})
	if err != nil {
		return "", "", err
	}
	systemsMu.Lock()
	systems[systemID] = [2]string{system.Name, names[constellation.RegionId]}
	systemsMu.Unlock()
	return system.Name, names[constellation.RegionId], nil
}

// isNpcCorporation is true for the NPC corporations characters start in,
// which have no assets or wallets to read.
func isNpcCorporation(id int64) bool {
	return id >= 1_000_000 && id < 2_000_000
}
