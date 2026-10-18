package main

import (
	"MeadowWorksTools/src/auth"
	"MeadowWorksTools/src/db"
	"MeadowWorksTools/src/production"
	"MeadowWorksTools/src/sde"
	"context"
	"log"
	"net/http"
	"time"
)

// ManufacturingColumn is one sortable heading of the Manufacturing table.
type ManufacturingColumn struct {
	Label   string
	Unit    string // shown after the label, e.g. ISK
	Numeric bool
	Href    string // the link that sorts by this column
	Arrow   string // ▲ or ▼ on the column the list is sorted by
}

func renderManufacturing(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.CurrentSession(r)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// jobs are fetched and saved by the hourly refresh; the page only reads
	// what's saved
	linked, err := db.ListUserCharacters(ctx, sess.UserID)
	if err != nil {
		log.Printf("renderManufacturing: characters of user %d: %v", sess.UserID, err)
		http.Error(w, "could not load your characters", http.StatusInternalServerError)
		return
	}
	characterIDs := make([]int64, 0, len(linked))
	for _, character := range linked {
		characterIDs = append(characterIDs, character.CharacterID)
	}

	// reactions count too, each product (intermediates included) as its own
	// item; only jobs the user's own characters started count
	totals, err := db.LoadLifetimeProduction(ctx, characterIDs,
		[]int{db.ActivityManufacturing, db.ActivityOldReaction, db.ActivityReaction})
	if err != nil {
		log.Printf("renderManufacturing: loading lifetime production: %v", err)
		http.Error(w, "could not load saved manufacturing: "+err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]production.Item, 0, len(totals))
	var grandQuantity, unpriced int64
	var grandValue float64
	for _, total := range totals {
		category, group, _ := sde.ItemType(total.ProductTypeID)
		items = append(items, production.Item{
			Name: total.ProductName, Category: category, Group: group,
			Quantity: total.Quantity, Value: total.Value, UnpricedQuantity: total.UnpricedQuantity,
		})
		grandQuantity += total.Quantity
		grandValue += total.Value
		unpriced += total.UnpricedQuantity
	}
	sort := production.ParseSort(r.URL.Query().Get("sort"), r.URL.Query().Get("dir"))
	production.SortItems(items, sort)

	data := struct {
		CharacterName string
		Columns       []ManufacturingColumn
		Items         []production.Item
		GrandQuantity int64
		GrandValue    float64
		Unpriced      int64
	}{CharacterName: sess.CharacterName,
		Columns: manufacturingColumns(sort), Items: items,
		GrandQuantity: grandQuantity, GrandValue: grandValue, Unpriced: unpriced}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "manufacturing.html", data); err != nil {
		log.Printf("renderManufacturing: %v", err)
	}
}

// manufacturingColumns gives each heading the link that sorts by it.
func manufacturingColumns(current production.Sort) []ManufacturingColumn {
	columns := []ManufacturingColumn{
		{Label: "Item", Href: production.SortName},
		{Label: "Item Type", Href: production.SortType},
		{Label: "Quantity Made", Unit: "units", Numeric: true, Href: production.SortQuantity},
		{Label: "Total Value", Unit: "ISK", Numeric: true, Href: production.SortValue},
	}
	for i, column := range columns {
		next := current.Toggle(column.Href)
		if current.Column == column.Href {
			columns[i].Arrow = "▲"
			if current.Desc {
				columns[i].Arrow = "▼"
			}
		}
		columns[i].Href = "/manufacturing?sort=" + next.Column + "&dir=" + next.Dir()
	}
	return columns
}
