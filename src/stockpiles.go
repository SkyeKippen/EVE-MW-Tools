package main

import (
	"MeadowWorksTools/src/auth"
	"MeadowWorksTools/src/db"
	"MeadowWorksTools/src/places"
	"MeadowWorksTools/src/sde"
	"MeadowWorksTools/src/stockpile"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Each signed-in user has their own stockpiles. A stockpile is counted
// against everything the user's characters and their corporations have at
// its station or structure, as saved by the hourly refresh.

const (
	suggestionMinChars = 3
	suggestionLimit    = 10
	maxNameLength      = 100
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: %v", err)
	}
}

// userOwners returns the IDs of the user's characters and of their player
// corporations, whose saved assets the user's pages count.
func userOwners(ctx context.Context, userID int64) ([]int64, error) {
	linked, err := db.ListUserCharacters(ctx, userID)
	if err != nil {
		return nil, err
	}
	owners := make([]int64, 0, len(linked))
	for _, character := range linked {
		owners = append(owners, character.CharacterID)
	}
	corporationIDs, err := db.LoadUserCorporations(ctx, userID)
	if err != nil {
		return nil, err
	}
	return append(owners, corporationIDs...), nil
}

func renderStockpiles(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.CurrentSession(r)

	list, err := db.ListStockpiles(r.Context(), sess.UserID)
	if err != nil {
		log.Printf("renderStockpiles: %v", err)
		http.Error(w, "could not load stockpiles", http.StatusInternalServerError)
		return
	}
	open, _ := strconv.ParseInt(r.URL.Query().Get("open"), 10, 64)

	data := struct {
		CharacterName string
		Stockpiles    []stockpile.Stockpile
		Open          int64 // the stockpile to show expanded
		Message       string
		Error         string
		KindItem      string
		KindRuns      string
	}{
		CharacterName: sess.CharacterName,
		Stockpiles:    list,
		Open:          open,
		Message:       r.URL.Query().Get("message"),
		Error:         r.URL.Query().Get("error"),
		KindItem:      stockpile.KindItem,
		KindRuns:      stockpile.KindBlueprintRuns,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "stockpiles.html", data); err != nil {
		log.Printf("renderStockpiles: %v", err)
	}
}

// backToStockpiles redirects to the Stockpiles page, showing a message or an
// error, with stockpile open expanded (0 for none).
func backToStockpiles(w http.ResponseWriter, r *http.Request, open int64, message, errMessage string) {
	q := url.Values{}
	if open != 0 {
		q.Set("open", strconv.FormatInt(open, 10))
	}
	if message != "" {
		q.Set("message", message)
	}
	if errMessage != "" {
		q.Set("error", errMessage)
	}
	target := "/stockpiles"
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	if open != 0 {
		target += fmt.Sprintf("#stockpile-%d", open)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func handleCreateStockpile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, _ := auth.CurrentSession(r)

	name := strings.TrimSpace(r.FormValue("name"))
	placeID, _ := strconv.ParseInt(r.FormValue("structure_id"), 10, 64)
	switch {
	case name == "":
		backToStockpiles(w, r, 0, "", "Give the stockpile a name.")
		return
	case len([]rune(name)) > maxNameLength:
		backToStockpiles(w, r, 0, "", fmt.Sprintf("Stockpile names can be up to %d characters.", maxNameLength))
		return
	case placeID == 0:
		backToStockpiles(w, r, 0, "", "Pick a station or structure from the suggestions.")
		return
	}

	id, err := db.CreateStockpile(r.Context(), sess.UserID, name, placeID, sess.CharacterID, sess.CharacterName)
	if errors.Is(err, stockpile.ErrNotFound) {
		backToStockpiles(w, r, 0, "", "That station or structure isn't one we know about. Pick one from the suggestions.")
		return
	}
	if err != nil {
		log.Printf("handleCreateStockpile: %v", err)
		http.Error(w, "could not create the stockpile", http.StatusInternalServerError)
		return
	}
	backToStockpiles(w, r, id, "Created "+name+".", "")
}

func handleDeleteStockpile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, _ := auth.CurrentSession(r)
	id, _ := strconv.ParseInt(r.FormValue("stockpile_id"), 10, 64)
	if err := db.DeleteStockpile(r.Context(), sess.UserID, id); err != nil {
		log.Printf("handleDeleteStockpile: %v", err)
		http.Error(w, "could not delete the stockpile", http.StatusInternalServerError)
		return
	}
	backToStockpiles(w, r, 0, "Deleted the stockpile.", "")
}

func handleAddStockpileItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, _ := auth.CurrentSession(r)
	stockpileID, _ := strconv.ParseInt(r.FormValue("stockpile_id"), 10, 64)
	typeID, _ := strconv.ParseInt(r.FormValue("type_id"), 10, 64)
	kind := r.FormValue("kind")
	if kind != stockpile.KindBlueprintRuns {
		kind = stockpile.KindItem
	}

	t, ok := sde.Get(typeID)
	if !ok {
		backToStockpiles(w, r, stockpileID, "", "Pick an item from the suggestions.")
		return
	}
	if kind == stockpile.KindBlueprintRuns && !t.IsBlueprint {
		backToStockpiles(w, r, stockpileID, "", t.Name+" isn't a blueprint, so it can't be added as blueprint runs.")
		return
	}
	quantity, ok := stockpile.ParseQuantity(r.FormValue("quantity"))
	if !ok {
		backToStockpiles(w, r, stockpileID, "", "Enter a whole number above 0, like 1200, 1,200 or 1.2k.")
		return
	}

	err := db.SetStockpileItem(r.Context(), sess.UserID, stockpileID,
		stockpile.Item{TypeId: t.ID, Name: t.Name, Kind: kind, Quantity: quantity})
	if errors.Is(err, stockpile.ErrNotFound) {
		backToStockpiles(w, r, 0, "", "That stockpile no longer exists.")
		return
	}
	if err != nil {
		log.Printf("handleAddStockpileItem: %v", err)
		http.Error(w, "could not add the item", http.StatusInternalServerError)
		return
	}
	backToStockpiles(w, r, stockpileID, "", "")
}

func handleRemoveStockpileItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, _ := auth.CurrentSession(r)
	stockpileID, _ := strconv.ParseInt(r.FormValue("stockpile_id"), 10, 64)
	itemID, _ := strconv.ParseInt(r.FormValue("stockpile_item_id"), 10, 64)
	if err := db.RemoveStockpileItem(r.Context(), sess.UserID, stockpileID, itemID); err != nil {
		log.Printf("handleRemoveStockpileItem: %v", err)
		http.Error(w, "could not remove the item", http.StatusInternalServerError)
		return
	}
	backToStockpiles(w, r, stockpileID, "", "")
}

// handleStockpileCounts returns, as JSON, how many of each stockpile item the
// user's characters and corporations have at the stockpile's station or
// structure, keyed "<stockpileId>-<typeId>", from the assets the hourly
// refresh saved. Blueprint runs are the runs left on every copy there, or BPO
// when there's an original.
//
// The response also says whose assets and blueprints are saved, so the page
// can explain a missing count.
func handleStockpileCounts(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.CurrentSession(r)
	counts, err := loadStockpileCounts(r.Context(), sess.UserID)
	if err != nil {
		if r.Context().Err() != nil {
			return // the page was closed or reloaded
		}
		log.Printf("handleStockpileCounts: user %d: %v", sess.UserID, err)
		http.Error(w, "could not load saved assets", http.StatusInternalServerError)
		return
	}
	writeJSON(w, counts)
}

type stockpileCountsResponse struct {
	Counts              map[string]int64            `json:"counts"`
	Runs                map[string]db.BlueprintRuns `json:"runs"`
	CharacterAssets     bool                        `json:"characterAssets"`
	CorpAssets          bool                        `json:"corpAssets"`
	CharacterBlueprints bool                        `json:"characterBlueprints"`
	CorpBlueprints      bool                        `json:"corpBlueprints"`
	Prices              map[int64]float64           `json:"prices"` // Jita sell price of items, by type ID
}

func loadStockpileCounts(ctx context.Context, userID int64) (stockpileCountsResponse, error) {
	var res stockpileCountsResponse
	list, err := db.ListStockpiles(ctx, userID)
	if err != nil {
		return res, fmt.Errorf("stockpiles: %w", err)
	}
	owners, err := userOwners(ctx, userID)
	if err != nil {
		return res, fmt.Errorf("characters: %w", err)
	}

	var placeIDs, typeIDs []int64
	for _, s := range list {
		placeIDs = append(placeIDs, s.Place.LocationID)
		for _, item := range s.Items {
			if item.Kind == stockpile.KindItem {
				typeIDs = append(typeIDs, item.TypeId)
			}
		}
	}

	stock, err := db.LoadStockAtPlaces(ctx, owners, placeIDs)
	if err != nil {
		return res, fmt.Errorf("assets: %w", err)
	}
	runs, err := db.LoadBlueprintRunsAtPlaces(ctx, owners, placeIDs)
	if err != nil {
		return res, fmt.Errorf("blueprints: %w", err)
	}
	savedAssets, savedBlueprints, err := db.SavedOwnerData(ctx, owners)
	if err != nil {
		return res, fmt.Errorf("saved owners: %w", err)
	}
	if res.Prices, err = db.LoadLatestJitaPrices(ctx, typeIDs); err != nil {
		return res, fmt.Errorf("prices: %w", err)
	}
	res.Counts, res.Runs = stockpileCounts(list, stock, runs)
	res.CharacterAssets, res.CorpAssets = savedAssets[db.OwnerCharacter], savedAssets[db.OwnerCorporation]
	res.CharacterBlueprints, res.CorpBlueprints = savedBlueprints[db.OwnerCharacter], savedBlueprints[db.OwnerCorporation]
	return res, nil
}

// stockpileCounts picks out, for each item a stockpile wants, what's at its
// place, keyed "<stockpileId>-<typeId>".
func stockpileCounts(list []stockpile.Stockpile, stock map[db.PlaceType]int64, runs map[db.PlaceType]db.BlueprintRuns) (map[string]int64, map[string]db.BlueprintRuns) {
	counts := map[string]int64{}
	runsByKey := map[string]db.BlueprintRuns{}
	for _, s := range list {
		for _, item := range s.Items {
			key := fmt.Sprintf("%d-%d", s.StockpileId, item.TypeId)
			at := db.PlaceType{PlaceID: s.Place.LocationID, TypeID: item.TypeId}
			if item.Kind == stockpile.KindBlueprintRuns {
				runsByKey[key] = runs[at]
			} else {
				counts[key] = stock[at]
			}
		}
	}
	return counts, runsByKey
}

// handleItemSuggestions returns item types matching ?q= as JSON, for the Add
// Item dialog. ?kind=blueprint_runs suggests only blueprints.
func handleItemSuggestions(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	items := []sde.Item{}
	if len([]rune(q)) >= suggestionMinChars {
		var err error
		items, err = sde.Search(q, r.URL.Query().Get("kind") == stockpile.KindBlueprintRuns, suggestionLimit)
		if err != nil {
			log.Printf("handleItemSuggestions: %v", err)
			http.Error(w, "item lookup failed", http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, items)
}

// handleStructureSuggestions returns, as JSON, the stations and structures
// matching ?q= where the user's characters or corporations have something,
// for the New Stockpile dialog.
func handleStructureSuggestions(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.CurrentSession(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	found := []places.Place{}
	if len([]rune(q)) >= suggestionMinChars {
		owners, err := userOwners(r.Context(), sess.UserID)
		if err == nil {
			found, err = db.SearchOwnerPlaces(r.Context(), owners, q, suggestionLimit)
		}
		if err != nil {
			log.Printf("handleStructureSuggestions: %v", err)
			http.Error(w, "structure lookup failed", http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, found)
}
