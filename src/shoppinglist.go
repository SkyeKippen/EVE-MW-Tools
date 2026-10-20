package main

import (
	"MeadowWorksTools/src/auth"
	"MeadowWorksTools/src/db"
	"MeadowWorksTools/src/sde"
	"MeadowWorksTools/src/shopping"
	"MeadowWorksTools/src/stockpile"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// The Shopping List page lists what the user's checked stockpiles are
// missing, and checks a list pasted in EVE's Multibuy format against the
// SDE. Both tables copy back out in Multibuy format. Each user can save one
// pasted list, kept in the database until they clear it.

// maxShoppingListLength caps a pasted or saved list, in bytes.
const maxShoppingListLength = 200_000

func renderShoppingList(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.CurrentSession(r)

	list, err := db.ListStockpiles(r.Context(), sess.UserID)
	if err != nil {
		log.Printf("renderShoppingList: %v", err)
		http.Error(w, "could not load stockpiles", http.StatusInternalServerError)
		return
	}
	saved, err := db.LoadShoppingList(r.Context(), sess.UserID)
	if err != nil {
		log.Printf("renderShoppingList: %v", err)
		http.Error(w, "could not load the saved list", http.StatusInternalServerError)
		return
	}

	data := struct {
		CharacterName string
		Stockpiles    []stockpile.Stockpile
		SavedList     string
		Message       string
		Error         string
	}{
		CharacterName: sess.CharacterName,
		Stockpiles:    list,
		SavedList:     saved,
		Message:       r.URL.Query().Get("message"),
		Error:         r.URL.Query().Get("error"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "shopping.html", data); err != nil {
		log.Printf("renderShoppingList: %v", err)
	}
}

func backToShoppingList(w http.ResponseWriter, r *http.Request, message, errMessage string) {
	q := url.Values{}
	if message != "" {
		q.Set("message", message)
	}
	if errMessage != "" {
		q.Set("error", errMessage)
	}
	target := "/shopping"
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// handleSaveShoppingList saves the pasted list, replacing the one saved
// before.
func handleSaveShoppingList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, _ := auth.CurrentSession(r)
	list := strings.TrimSpace(strings.ReplaceAll(r.FormValue("list"), "\r\n", "\n"))
	switch {
	case list == "":
		backToShoppingList(w, r, "", "There's nothing in the box to save. Use Clear Cached List to remove the saved list.")
		return
	case len(list) > maxShoppingListLength:
		backToShoppingList(w, r, "", "That list is too long to save.")
		return
	}
	if err := db.SaveShoppingList(r.Context(), sess.UserID, list); err != nil {
		log.Printf("handleSaveShoppingList: %v", err)
		http.Error(w, "could not save the list", http.StatusInternalServerError)
		return
	}
	backToShoppingList(w, r, "Saved your list.", "")
}

func handleClearShoppingList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, _ := auth.CurrentSession(r)
	if err := db.ClearShoppingList(r.Context(), sess.UserID); err != nil {
		log.Printf("handleClearShoppingList: %v", err)
		http.Error(w, "could not clear the list", http.StatusInternalServerError)
		return
	}
	backToShoppingList(w, r, "Cleared the saved list.", "")
}

// handleShoppingMissing returns, as JSON rows, what the stockpiles in
// ?ids=1,2,3 are missing, added up by item, from the assets the hourly
// refresh saved, priced at Jita sell as the Stockpiles page prices them.
func handleShoppingMissing(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.CurrentSession(r)
	selected := map[int64]bool{}
	for _, id := range strings.Split(r.URL.Query().Get("ids"), ",") {
		if n, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64); err == nil {
			selected[n] = true
		}
	}
	lines := []shopping.Line{}
	if len(selected) > 0 {
		list, err := db.ListStockpiles(r.Context(), sess.UserID)
		var counts stockpileCountsResponse
		if err == nil {
			counts, err = loadStockpileCounts(r.Context(), sess.UserID)
		}
		if err != nil {
			if r.Context().Err() != nil {
				return // the page was closed or reloaded
			}
			log.Printf("handleShoppingMissing: user %d: %v", sess.UserID, err)
			http.Error(w, "could not load saved assets", http.StatusInternalServerError)
			return
		}
		names := map[int64]string{}
		for _, s := range list {
			for _, item := range s.Items {
				names[item.TypeId] = item.Name
			}
		}
		for typeID, quantity := range shopping.Missing(list, selected, counts.Counts) {
			t := describeType(typeID)
			if t.Name == "" {
				t.Name = names[typeID]
			}
			lines = append(lines, shopping.Line{TypeID: typeID, Name: t.Name, Quantity: quantity, Category: t.Category, Group: t.Group, Blueprint: t.Blueprint})
		}
		shopping.SetPrices(lines, counts.Prices)
		shopping.Sort(lines)
	}
	writeJSON(w, lines)
}

// handleShoppingParse returns, as JSON rows, the Multibuy list posted as
// "list", with lines that name no item marked bad, priced at Jita sell as
// the Stockpiles page prices them.
func handleShoppingParse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	text := r.FormValue("list")
	if len(text) > maxShoppingListLength {
		http.Error(w, "list too long", http.StatusRequestEntityTooLarge)
		return
	}
	lines := shopping.Parse(text, findType)
	if lines == nil {
		lines = []shopping.Line{}
	}
	prices, err := db.LoadLatestJitaPrices(r.Context(), shopping.TypeIDs(lines))
	if err != nil {
		if r.Context().Err() != nil {
			return // the page was closed or the list changed again
		}
		log.Printf("handleShoppingParse: %v", err)
		http.Error(w, "could not load prices", http.StatusInternalServerError)
		return
	}
	shopping.SetPrices(lines, prices)
	writeJSON(w, lines)
}

// findType looks up a published item by name, ignoring case.
func findType(name string) (shopping.Type, bool) {
	item, ok := sde.Find(name)
	if !ok {
		return shopping.Type{}, false
	}
	return describeType(item.ID), true
}

// describeType returns an item's name, category and group from the SDE.
func describeType(typeID int64) shopping.Type {
	item, _ := sde.Get(typeID)
	category, group, _ := sde.ItemType(typeID)
	return shopping.Type{ID: typeID, Name: item.Name, Category: category, Group: group, Blueprint: item.IsBlueprint}
}
