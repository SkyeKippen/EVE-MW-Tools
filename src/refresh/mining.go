package refresh

import (
	"MeadowWorksTools/src/db"
	"MeadowWorksTools/src/esi"
	"MeadowWorksTools/src/mining"
	"context"
	"errors"
	"fmt"
	"time"
)

func (r *Refresher) saveCharacterMining(ctx context.Context, character db.TokenCharacter, accessToken string) error {
	entries, err := r.ESI.GetCharacterMiningLedger(ctx, character.CharacterID, accessToken)
	if err != nil {
		return err
	}

	ids := make([]int64, 0, 2*len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.TypeId, entry.SolarSystemId)
	}
	names, err := r.ESI.GetNamesCached(ctx, ids)
	if err != nil {
		return fmt.Errorf("naming ores and systems: %w", err)
	}

	rows := make([]db.MiningRow, 0, len(entries))
	for _, entry := range entries {
		date, err := time.Parse(time.DateOnly, entry.Date)
		if err != nil {
			return fmt.Errorf("mining ledger date %q: %w", entry.Date, err)
		}
		oreType := typeName(names, entry.TypeId)
		rows = append(rows, db.MiningRow{
			CharacterID:     character.CharacterID,
			CharacterName:   character.CharacterName,
			Date:            date,
			SolarSystemID:   entry.SolarSystemId,
			SolarSystemName: names[entry.SolarSystemId],
			TypeID:          entry.TypeId,
			OreType:         oreType,
			BaseOre:         mining.BaseOreName(oreType),
			Quantity:        entry.Quantity,
		})
	}
	return db.SaveMiningLedger(ctx, rows)
}

func (r *Refresher) saveCorporationMining(ctx context.Context, corporationID int64, member db.TokenCharacter, accessToken string) error {
	ledgers, err := r.ESI.GetCorporationMiningLedger(ctx, corporationID, accessToken)
	if err != nil {
		return err
	}

	// structures can be looked up when the member granted the scope and is
	// on the structure's access list; otherwise the name stays blank
	structures := map[int64]esi.Structure{}
	if member.HasScope(ScopeStructures) {
		for _, ledger := range ledgers {
			structure, err := r.ESI.GetStructure(ctx, ledger.Observer.ObserverId, accessToken)
			if errors.Is(err, esi.ErrForbidden) || errors.Is(err, esi.ErrNotFound) {
				continue
			}
			if err != nil {
				return fmt.Errorf("observer %d: %w", ledger.Observer.ObserverId, err)
			}
			structures[ledger.Observer.ObserverId] = structure
		}
	}

	var typeAndSystemIds, characterIds []int64
	for _, structure := range structures {
		typeAndSystemIds = append(typeAndSystemIds, structure.SolarSystemId)
	}
	for _, ledger := range ledgers {
		for _, entry := range ledger.Entries {
			typeAndSystemIds = append(typeAndSystemIds, entry.TypeId)
			characterIds = append(characterIds, entry.CharacterId)
		}
	}
	names, err := r.ESI.GetNamesCached(ctx, typeAndSystemIds)
	if err != nil {
		return fmt.Errorf("naming ores and systems: %w", err)
	}
	// characters can be renamed, so their names aren't cached
	characterNames, err := r.ESI.GetNames(ctx, characterIds)
	if err != nil {
		return fmt.Errorf("naming miners: %w", err)
	}

	var rows []db.CorpMiningRow
	for _, ledger := range ledgers {
		structure := structures[ledger.Observer.ObserverId]
		for _, entry := range ledger.Entries {
			date, err := time.Parse(time.DateOnly, entry.LastUpdated)
			if err != nil {
				return fmt.Errorf("observed mining date %q: %w", entry.LastUpdated, err)
			}
			oreType := typeName(names, entry.TypeId)
			rows = append(rows, db.CorpMiningRow{
				CorporationID:         corporationID,
				ObserverID:            ledger.Observer.ObserverId,
				ObserverName:          structure.Name,
				SolarSystemID:         structure.SolarSystemId,
				SolarSystemName:       names[structure.SolarSystemId],
				Date:                  date,
				CharacterID:           entry.CharacterId,
				CharacterName:         characterNames[entry.CharacterId],
				RecordedCorporationID: entry.RecordedCorporationId,
				TypeID:                entry.TypeId,
				OreType:               oreType,
				BaseOre:               mining.BaseOreName(oreType),
				Quantity:              entry.Quantity,
			})
		}
	}
	return db.SaveCorporationMiningLedger(ctx, rows)
}

func typeName(names map[int64]string, typeId int64) string {
	if name, ok := names[typeId]; ok {
		return name
	}
	return fmt.Sprintf("Unknown type %d", typeId)
}
