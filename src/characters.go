package main

import (
	"MeadowWorksTools/src/auth"
	"MeadowWorksTools/src/db"
	"log"
	"net/http"
)

func renderCharacters(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.CurrentSession(r)

	characters, err := db.ListUserCharacters(r.Context(), sess.UserID)
	if err != nil {
		log.Printf("renderCharacters: user %d: %v", sess.UserID, err)
		http.Error(w, "could not load your characters", http.StatusInternalServerError)
		return
	}

	// CharacterID marks the signed-in character's row.
	data := struct {
		CharacterID int64
		Characters  []db.UserCharacter
	}{CharacterID: sess.CharacterID, Characters: characters}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "characters.html", data); err != nil {
		log.Printf("renderCharacters: %v", err)
	}
}

func renderAddAltCharacter(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.CurrentSession(r)

	data := struct {
		CharacterName string
		AddAlt        bool
		ScopeChoices  []auth.ScopeChoice
	}{CharacterName: sess.CharacterName, AddAlt: true, ScopeChoices: auth.ScopeChoices()}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "characters_add.html", data); err != nil {
		log.Printf("renderAddAltCharacter: %v", err)
	}
}
