package main

import (
	"LifetimeIndustryTracker/src/auth"
	"LifetimeIndustryTracker/src/db"
	"LifetimeIndustryTracker/src/esi"
	"LifetimeIndustryTracker/src/sde"
	"context"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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

var tmpl = template.Must(
	template.New("").Funcs(template.FuncMap{
		"commaInt": commaInt,
		"isk":      isk,
		"m3":       isk,
		"percent":  percent,
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
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	http.HandleFunc("/{$}", renderBase)
	http.HandleFunc("/characters", auth.RequireAuth(renderCharacters))
	http.HandleFunc("/characters/add", auth.RequireAuth(renderAddAltCharacter))
	http.HandleFunc("/mining", auth.RequireAuth(renderMining))
	http.HandleFunc("/manufacturing", auth.RequireAuth(renderManufacturing))
	http.HandleFunc("/worth", auth.RequireAuth(renderMyWorth))

	http.HandleFunc("/auth/login", auth.HandleLogin)
	http.HandleFunc("/auth/callback", auth.HandleCallback)
	http.HandleFunc("/auth/logout", handleLogout)

	// the Mining page values ore by what it refines into, which only the
	// SDE knows
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
