package main

import (
	"MeadowWorksTools/src/auth"
	"MeadowWorksTools/src/db"
	"MeadowWorksTools/src/esi"
	"MeadowWorksTools/src/mining"
	"MeadowWorksTools/src/refresh"
	"MeadowWorksTools/src/sde"
	"context"
	"errors"
	"log"
	"net/http"
	"time"
)

// esiClient makes every ESI call the pages need.
var esiClient = &esi.Client{}

// refresher saves ESI data to the database, hourly and on demand.
var refresher = &refresh.Refresher{ESI: esiClient}

// MiningCharacter is a character whose mining ledger the page tried to read.
type MiningCharacter struct {
	CharacterName string
	Problem       string // why their latest mining couldn't be saved, or ""
}

// userCharacters returns every character of the signed-in user, main and
// alts, with the scopes each granted. A character whose token is gone is
// still included so the page can say it needs to sign in again.
func userCharacters(ctx context.Context, sess *auth.Session) ([]db.TokenCharacter, error) {
	linked, err := db.ListUserCharacters(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}
	characters := make([]db.TokenCharacter, 0, len(linked))
	for _, character := range linked {
		withToken, err := db.LoadTokenCharacter(ctx, character.CharacterID)
		if err != nil {
			withToken = db.TokenCharacter{CharacterID: character.CharacterID, CharacterName: character.CharacterName}
		}
		characters = append(characters, withToken)
	}
	return characters, nil
}

func renderMining(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.CurrentSession(r)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// mining is fetched and saved by the hourly refresh; the page only reads
	// what's saved
	var characterIDs []int64
	var characters []MiningCharacter
	userChars, err := userCharacters(ctx, sess)
	if err != nil {
		log.Printf("renderMining: characters of user %d: %v", sess.UserID, err)
		http.Error(w, "could not load your characters", http.StatusInternalServerError)
		return
	}
	for _, character := range userChars {
		characterIDs = append(characterIDs, character.CharacterID)
		problem := ""
		if !character.HasScope(refresh.ScopeCharacterMining) {
			problem = "they haven't shared their mining ledger, so sign in with them again with Character mining ledger ticked"
		}
		characters = append(characters, MiningCharacter{CharacterName: character.CharacterName, Problem: problem})
	}

	ores, err := db.LoadOreTotals(ctx, characterIDs)
	if err != nil {
		log.Printf("renderMining: loading ore totals: %v", err)
		http.Error(w, "could not load saved mining: "+err.Error(), http.StatusInternalServerError)
		return
	}
	valueProblem, volumesErr := addOreValues(ctx, ores, characterIDs)

	var grandTotal, unpriced int64
	var grandValue, grandVolume float64
	for _, ore := range ores {
		grandTotal += ore.Quantity
		grandValue += ore.Value
		unpriced += ore.UnpricedQuantity
		grandVolume += ore.Volume
	}

	data := struct {
		CharacterName string
		Characters    []MiningCharacter
		Ores          []mining.OreTotal
		GrandTotal    int64
		GrandValue    float64
		GrandVolume   float64
		Unpriced      int64
		ValueProblem  string
		HasVolumes    bool
		TrackingDays  int64
	}{CharacterName: sess.CharacterName, Characters: characters, Ores: ores, GrandTotal: grandTotal,
		GrandValue: grandValue, GrandVolume: grandVolume, Unpriced: unpriced, ValueProblem: valueProblem,
		HasVolumes: volumesErr == nil, TrackingDays: trackingDays(ctx, sess.UserID, miningBackfillDays)}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err = tmpl.ExecuteTemplate(w, "mining.html", data)
	if err != nil {
		log.Printf("renderMining: %v", err)
	}
}

// addOreValues values each ore at the saved Jita sell prices of what it
// refines into on the days it was mined, and adds up the m3 mined. It reads
// only the database and the SDE. It returns why the values
// are missing, or "", and why the volumes are, or nil.
func addOreValues(ctx context.Context, ores []mining.OreTotal, characterIDs []int64) (string, error) {
	days, err := db.LoadMinedDays(ctx, characterIDs)
	if err != nil {
		log.Printf("renderMining: loading mining by day: %v", err)
		return "the saved mining couldn't be read by day", err
	}

	volumes, volumesErr := sde.Volumes()
	if volumesErr == nil {
		mining.AddVolumes(ores, days, volumes)
	}

	oreTypes := map[int64]string{}
	for _, day := range days {
		oreTypes[day.TypeID] = day.OreType
	}
	recipes, err := refresh.OreRecipes(oreTypes)
	if errors.Is(err, sde.ErrNotLoaded) {
		return "the refine materials are still loading from EVE's static data, so check back in a few minutes", volumesErr
	}
	if err != nil {
		log.Printf("renderMining: refine recipes: %v", err)
		return "the refine materials couldn't be loaded", volumesErr
	}
	materialIDs := refresh.MaterialIDs(recipes)
	book, err := db.LoadJitaSellPrices(ctx, materialIDs)
	if err != nil {
		log.Printf("renderMining: loading Jita prices: %v", err)
		return "the saved Jita prices couldn't be read", volumesErr
	}
	mining.AddValues(ores, days, recipes, book)
	return "", volumesErr
}
