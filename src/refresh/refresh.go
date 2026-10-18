// Package refresh reads each signed-in character's and their corporations'
// data from ESI and saves it, so the saved history keeps growing past what
// ESI remembers (30 days for journals and mining, 90 for orders and jobs).
package refresh

import (
	"MeadowWorksTools/src/db"
	"MeadowWorksTools/src/esi"
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

// Scopes each dataset needs.
const (
	ScopeCharacterWallet   = "esi-wallet.read_character_wallet.v1"
	ScopeCharacterAssets   = "esi-assets.read_assets.v1"
	ScopeCharacterBPs      = "esi-characters.read_blueprints.v1"
	ScopeCharacterJobs     = "esi-industry.read_character_jobs.v1"
	ScopeCharacterMining   = "esi-industry.read_character_mining.v1"
	ScopeCharacterOrders   = "esi-markets.read_character_orders.v1"
	ScopeLoyalty           = "esi-characters.read_loyalty.v1"
	ScopeCorporationWallet = "esi-wallet.read_corporation_wallets.v1"
	ScopeCorporationAssets = "esi-assets.read_corporation_assets.v1"
	ScopeCorporationBPs    = "esi-corporations.read_blueprints.v1"
	ScopeCorporationJobs   = "esi-industry.read_corporation_jobs.v1"
	ScopeCorporationMining = "esi-industry.read_corporation_mining.v1"
	ScopeCorporationOrders = "esi-markets.read_corporation_orders.v1"
	ScopeStructures        = "esi-universe.read_structures.v1"
)

// corporations have seven wallet divisions
const walletDivisions = 7

// ErrMissingScope means the character didn't grant the scope a dataset needs.
var ErrMissingScope = errors.New("scope not granted")

// Refresher fetches and saves data. ESI is the client to call ESI with and
// SSO the config used to refresh access tokens.
type Refresher struct {
	ESI *esi.Client
	SSO *esi.Config
}

// Every refreshes every character with a saved token, and their
// corporations, now and then every interval. ESI caches most of these routes
// for an hour, so refreshing more often would only fetch the same answers.
func (r *Refresher) Every(interval time.Duration) {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		r.All(ctx)
		cancel()
		time.Sleep(interval)
	}
}

// All refreshes every character with a saved token, then each of their
// corporations once, then looks up the stations and structures they have
// things at, records newly delivered manufacturing and reaction jobs, saves
// the day's Jita prices (once a day), values the deliveries and takes the
// hour's net worth snapshots.
func (r *Refresher) All(ctx context.Context) {
	characters, err := db.ListTokenCharacters(ctx)
	if err != nil {
		log.Printf("refresh: listing characters: %v", err)
		return
	}

	corporations := map[int64][]db.TokenCharacter{}
	corporationOf := map[int64]int64{}
	for _, character := range characters {
		r.logErrors(character.CharacterName, r.Character(ctx, character))

		info, err := r.ESI.GetCharacter(ctx, character.CharacterID)
		if err != nil {
			log.Printf("refresh: corporation of %s: %v", character.CharacterName, err)
			continue
		}
		r.recordCorporation(ctx, character, info.CorporationId)
		corporations[info.CorporationId] = append(corporations[info.CorporationId], character)
		corporationOf[character.CharacterID] = info.CorporationId
	}
	for corporationID, members := range corporations {
		r.logErrors(fmt.Sprintf("corporation %d", corporationID), r.Corporation(ctx, corporationID, members))
	}
	for _, character := range characters {
		if corporationID, ok := corporationOf[character.CharacterID]; ok {
			if err := r.SavePlaces(ctx, character, corporationID); err != nil {
				log.Printf("refresh: places of %s: %v", character.CharacterName, err)
			}
		}
	}
	// new deliveries first, so today's prices include their items
	if err := r.recordNewDeliveries(ctx); err != nil {
		log.Printf("refresh: delivered industry jobs: %v", err)
	}
	if err := r.saveDailyPrices(ctx); err != nil {
		log.Printf("refresh: today's Jita prices: %v", err)
	}
	if err := r.priceDeliveries(ctx); err != nil {
		log.Printf("refresh: pricing delivered industry jobs: %v", err)
	}
	if err := r.TakeSnapshots(ctx, time.Now()); err != nil {
		log.Printf("refresh: net worth snapshots: %v", err)
	}
}

// recordCorporation saves which corporation the character is in, for the
// pages that include their corporations.
func (r *Refresher) recordCorporation(ctx context.Context, character db.TokenCharacter, corporationID int64) {
	if err := db.SetCharacterCorporation(ctx, character.CharacterID, corporationID); err != nil {
		log.Printf("refresh: saving the corporation of %s: %v", character.CharacterName, err)
	}
}

// InBackground refreshes one character and their corporation without holding
// up the caller, e.g. right after they sign in.
func (r *Refresher) InBackground(characterID int64) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		character, err := db.LoadTokenCharacter(ctx, characterID)
		if err != nil {
			log.Printf("refresh: character %d: %v", characterID, err)
			return
		}
		r.logErrors(character.CharacterName, r.Character(ctx, character))
		info, err := r.ESI.GetCharacter(ctx, characterID)
		if err != nil {
			log.Printf("refresh: corporation of %s: %v", character.CharacterName, err)
			return
		}
		r.recordCorporation(ctx, character, info.CorporationId)
		r.logErrors(fmt.Sprintf("corporation %d", info.CorporationId), r.Corporation(ctx, info.CorporationId, []db.TokenCharacter{character}))
		if err := r.SavePlaces(ctx, character, info.CorporationId); err != nil {
			log.Printf("refresh: places of %s: %v", character.CharacterName, err)
		}
		if err := r.RecordDeliveries(ctx); err != nil {
			log.Printf("refresh: delivered industry jobs: %v", err)
		}
	}()
}

func (r *Refresher) logErrors(who string, errs map[string]error) {
	for dataset, err := range errs {
		if errors.Is(err, ErrMissingScope) {
			continue // the character chose not to share it
		}
		log.Printf("refresh: %s: %s: %v", who, dataset, err)
	}
}

// Character saves everything the character granted scopes for. It carries on
// past a failed dataset and returns each failure by dataset name.
func (r *Refresher) Character(ctx context.Context, character db.TokenCharacter) map[string]error {
	errs := map[string]error{}
	run := func(dataset, scope string, save func(accessToken string) error) {
		if !character.HasScope(scope) {
			errs[dataset] = ErrMissingScope
			return
		}
		accessToken, err := db.GetAccessToken(ctx, r.SSO, character.CharacterID)
		if err == nil {
			err = save(accessToken)
		}
		if err != nil {
			errs[dataset] = err
		}
	}
	id := character.CharacterID

	run("wallet journal", ScopeCharacterWallet, func(token string) error {
		entries, err := r.ESI.GetCharacterWalletJournal(ctx, id, token)
		if err != nil {
			return err
		}
		return db.SaveCharacterWalletJournal(ctx, id, entries)
	})
	run("wallet balance", ScopeCharacterWallet, func(token string) error {
		balance, err := r.ESI.GetCharacterWalletBalance(ctx, id, token)
		if err != nil {
			return err
		}
		return db.SaveWalletBalance(ctx, id, db.OwnerCharacter, 0, balance)
	})
	run("wallet transactions", ScopeCharacterWallet, func(token string) error {
		transactions, err := r.ESI.GetCharacterWalletTransactions(ctx, id, token)
		if err != nil {
			return err
		}
		return db.SaveWalletTransactions(ctx, id, db.OwnerCharacter, 0, transactions)
	})
	run("loyalty points", ScopeLoyalty, func(token string) error {
		points, err := r.ESI.GetCharacterLoyaltyPoints(ctx, id, token)
		if err != nil {
			return err
		}
		return db.SaveLoyaltyPoints(ctx, id, today(), points)
	})
	run("market orders", ScopeCharacterOrders, func(token string) error {
		open, err := r.ESI.GetCharacterMarketOrders(ctx, id, token)
		if err != nil {
			return err
		}
		history, err := r.ESI.GetCharacterMarketOrderHistory(ctx, id, token)
		if err != nil {
			return err
		}
		return db.SaveMarketOrders(ctx, id, db.OwnerCharacter, open, history)
	})
	run("assets", ScopeCharacterAssets, func(token string) error {
		assets, err := r.ESI.GetCharacterAssets(ctx, id, token)
		if err != nil {
			return err
		}
		return db.ReplaceAssets(ctx, id, db.OwnerCharacter, assets)
	})
	// after the assets, which say where each blueprint is
	run("blueprints", ScopeCharacterBPs, func(token string) error {
		blueprints, err := r.ESI.GetCharacterBlueprints(ctx, id, token)
		if err != nil {
			return err
		}
		return db.ReplaceBlueprints(ctx, id, db.OwnerCharacter, blueprints)
	})
	run("industry jobs", ScopeCharacterJobs, func(token string) error {
		jobs, err := r.ESI.GetCharacterIndustryJobs(ctx, id, token)
		if err != nil {
			return err
		}
		return db.SaveIndustryJobs(ctx, id, db.OwnerCharacter, jobs)
	})
	run("mining ledger", ScopeCharacterMining, func(token string) error {
		return r.saveCharacterMining(ctx, character, token)
	})
	return errs
}

// Corporation saves the corporation's data using whichever of members has
// the scope and the in-game role for each dataset; a member without the role
// gets a 403, and the next member is tried.
func (r *Refresher) Corporation(ctx context.Context, corporationID int64, members []db.TokenCharacter) map[string]error {
	errs := map[string]error{}
	run := func(dataset, scope string, save func(member db.TokenCharacter, accessToken string) error) {
		err := error(ErrMissingScope)
		for _, member := range members {
			if !member.HasScope(scope) {
				continue
			}
			var accessToken string
			accessToken, err = db.GetAccessToken(ctx, r.SSO, member.CharacterID)
			if err == nil {
				err = save(member, accessToken)
			}
			if !errors.Is(err, esi.ErrForbidden) {
				break
			}
		}
		if err != nil {
			errs[dataset] = err
		}
	}
	id := corporationID

	run("wallets", ScopeCorporationWallet, func(_ db.TokenCharacter, token string) error {
		balances, err := r.ESI.GetCorporationWalletBalances(ctx, id, token)
		if err != nil {
			return err
		}
		for _, balance := range balances {
			if err := db.SaveWalletBalance(ctx, id, db.OwnerCorporation, balance.Division, balance.Balance); err != nil {
				return err
			}
		}
		for division := 1; division <= walletDivisions; division++ {
			entries, err := r.ESI.GetCorporationWalletJournal(ctx, id, division, token)
			if err != nil {
				return fmt.Errorf("division %d: %w", division, err)
			}
			if err := db.SaveCorporationWalletJournal(ctx, id, division, entries); err != nil {
				return err
			}
			transactions, err := r.ESI.GetCorporationWalletTransactions(ctx, id, division, token)
			if err != nil {
				return fmt.Errorf("division %d transactions: %w", division, err)
			}
			if err := db.SaveWalletTransactions(ctx, id, db.OwnerCorporation, division, transactions); err != nil {
				return err
			}
		}
		return nil
	})
	run("market orders", ScopeCorporationOrders, func(_ db.TokenCharacter, token string) error {
		open, err := r.ESI.GetCorporationMarketOrders(ctx, id, token)
		if err != nil {
			return err
		}
		history, err := r.ESI.GetCorporationMarketOrderHistory(ctx, id, token)
		if err != nil {
			return err
		}
		return db.SaveMarketOrders(ctx, id, db.OwnerCorporation, open, history)
	})
	run("assets", ScopeCorporationAssets, func(_ db.TokenCharacter, token string) error {
		assets, err := r.ESI.GetCorporationAssets(ctx, id, token)
		if err != nil {
			return err
		}
		return db.ReplaceAssets(ctx, id, db.OwnerCorporation, assets)
	})
	run("blueprints", ScopeCorporationBPs, func(_ db.TokenCharacter, token string) error {
		blueprints, err := r.ESI.GetCorporationBlueprints(ctx, id, token)
		if err != nil {
			return err
		}
		return db.ReplaceBlueprints(ctx, id, db.OwnerCorporation, blueprints)
	})
	run("industry jobs", ScopeCorporationJobs, func(_ db.TokenCharacter, token string) error {
		jobs, err := r.ESI.GetCorporationIndustryJobs(ctx, id, token)
		if err != nil {
			return err
		}
		return db.SaveIndustryJobs(ctx, id, db.OwnerCorporation, jobs)
	})
	run("mining ledger", ScopeCorporationMining, func(member db.TokenCharacter, token string) error {
		return r.saveCorporationMining(ctx, id, member, token)
	})
	return errs
}

func today() time.Time {
	return time.Now().UTC().Truncate(24 * time.Hour)
}
