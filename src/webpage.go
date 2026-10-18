package main

import (
	"MeadowWorksTools/src/auth"
	"MeadowWorksTools/src/db"
	"MeadowWorksTools/src/esi"
	"MeadowWorksTools/src/sde"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

func commaInt(n int64) string {
	return addCommas(fmt.Sprintf("%d", n))
}

// isk shows an amount, such as ISK or m3, rounded to a whole number, with
// commas.
func isk(amount float64) string {
	return commaInt(int64(math.Round(amount)))
}

// percent shows a share to one decimal place, e.g. 42.5%.
func percent(share float64) string {
	return fmt.Sprintf("%.1f%%", share)
}

func addCommas(s string) string {
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}

	var result []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, c)
	}

	out := string(result)
	if neg {
		out = "-" + out
	}
	return out
}

// assetVersion changes every time the server starts, so links to static
// files change too and browsers and proxies can't keep serving old copies.
var assetVersion = strconv.FormatInt(time.Now().Unix(), 36)

// static is the link to a file in src/static, e.g. {{static "style.css"}}.
func static(name string) string {
	return "/static/" + name + "?v=" + assetVersion
}

var tmpl = template.Must(
	template.New("").Funcs(template.FuncMap{
		"commaInt": commaInt,
		"isk":      isk,
		"m3":       isk,
		"percent":  percent,
		"static":   static,
	}).ParseGlob("src/templates/*.html"),
)

func handleLogout(w http.ResponseWriter, r *http.Request) {
	auth.ClearSessionCookie(w, r)
	http.Redirect(w, r, "/", http.StatusFound)
}

// saveTokenAndRefresh saves a character's tokens after sign-in, then saves
// their (and their corporation's) data in the background so it's ready
// without slowing sign-in.
func saveTokenAndRefresh(ctx context.Context, token esi.CharacterToken) error {
	if err := db.SaveCharacterToken(ctx, token); err != nil {
		return err
	}
	refresher.InBackground(token.CharacterID)
	return nil
}

func main() {
	err := godotenv.Load("config/.env")
	if err != nil {
		log.Fatal("Error loading .env file: ", err)
	}

	if err := auth.InitAuth(saveTokenAndRefresh, db.LinkCharacter); err != nil {
		log.Fatalf("evesso: config: %v", err)
	}

	_, thisFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(thisFile)
	staticDir := filepath.Join(baseDir, "static")

	fs := http.FileServer(http.Dir(staticDir))
	// no-cache makes the browser check for a newer file on every page load
	// (a cheap 304 when it hasn't changed), so updated scripts and styles are
	// picked up without a hard refresh
	http.Handle("/static/", http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		fs.ServeHTTP(w, r)
	})))
	// browsers ask for /favicon.ico on pages that don't name an icon, like errors
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(staticDir, "favicon.png"))
	})
	http.HandleFunc("/image", handleImage)

	http.HandleFunc("/{$}", renderBase)
	http.HandleFunc("/characters", auth.RequireAuth(renderCharacters))
	http.HandleFunc("/characters/add", auth.RequireAuth(renderAddAltCharacter))
	http.HandleFunc("/characters/deauthorize", auth.RequireAuth(handleDeauthorizeCharacter))
	http.HandleFunc("/characters/deauthorize-all", auth.RequireAuth(handleDeauthorizeAll))
	http.HandleFunc("/mining", auth.RequireAuth(renderMining))
	http.HandleFunc("/manufacturing", auth.RequireAuth(renderManufacturing))
	http.HandleFunc("/stockpiles", auth.RequireAuth(renderStockpiles))
	http.HandleFunc("/stockpiles/create", auth.RequireAuth(handleCreateStockpile))
	http.HandleFunc("/stockpiles/delete", auth.RequireAuth(handleDeleteStockpile))
	http.HandleFunc("/stockpiles/items/add", auth.RequireAuth(handleAddStockpileItem))
	http.HandleFunc("/stockpiles/items/remove", auth.RequireAuth(handleRemoveStockpileItem))
	http.HandleFunc("/stockpiles/counts", auth.RequireAuth(handleStockpileCounts))
	http.HandleFunc("/stockpiles/item-suggestions", auth.RequireAuth(handleItemSuggestions))
	http.HandleFunc("/stockpiles/structure-suggestions", auth.RequireAuth(handleStructureSuggestions))
	http.HandleFunc("/worth", auth.RequireAuth(renderMyWorth))

	http.HandleFunc("/auth/login", auth.HandleLogin)
	http.HandleFunc("/auth/callback", auth.HandleCallback)
	http.HandleFunc("/auth/logout", handleLogout)

	// the Mining page values ore by what it refines into, and stockpiles
	// search item names, which only the SDE knows
	sde.LoadInBackground()

	// ESI only remembers 30 to 90 days and caches most routes for an hour,
	// so saving everything hourly keeps the full history
	refresher.SSO = auth.Config()
	go refresher.Every(time.Hour)

	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "5050"
	}

	log.Printf("Server listening on port %s", port)
	err = http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatal(err)
	}
}

func renderBase(w http.ResponseWriter, r *http.Request) {
	sess, loggedIn := auth.CurrentSession(r)

	data := struct {
		LoggedIn      bool
		CharacterName string
		RequiredScope string
		AddAlt        bool
		ScopeChoices  []auth.ScopeChoice
	}{LoggedIn: loggedIn, RequiredScope: auth.RequiredScope, ScopeChoices: auth.ScopeChoices()}
	if loggedIn {
		data.CharacterName = sess.CharacterName
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := tmpl.ExecuteTemplate(w, "base.html", data)
	if err != nil {
		log.Printf("renderBase: %v", err)
	}
}

// handleImage serves an image from the EVE image server through the local
// cache, downloading it the first time.
// ?category=types|characters|corporations|alliances&id=<ID>&variation=<v>&size=<px>
// e.g. /image?category=types&id=34&variation=icon&size=64 for Tritanium.
// variation defaults to the category's usual one and size to 64.
func handleImage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	category := q.Get("category")
	id, err := strconv.ParseInt(q.Get("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	variation := q.Get("variation")
	if variation == "" {
		variation = defaultImageVariation[category]
	}
	size := 64
	if s := q.Get("size"); s != "" {
		if size, err = strconv.Atoi(s); err != nil {
			http.Error(w, "invalid size", http.StatusBadRequest)
			return
		}
	}

	path, err := esi.ImagePath(category, id, variation, size)
	switch {
	case errors.Is(err, esi.ErrInvalidImage):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case errors.Is(err, esi.ErrImageNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		log.Printf("image %s %d: %v", category, id, err)
		http.Error(w, "could not load image", http.StatusBadGateway)
		return
	}

	// let the browser keep images for a day; the cache refetches portraits daily
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, path)
}

var defaultImageVariation = map[string]string{
	"types":        "icon",
	"characters":   "portrait",
	"corporations": "logo",
	"alliances":    "logo",
}
