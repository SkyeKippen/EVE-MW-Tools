package main

import (
	"MeadowWorksTools/src/auth"
	"MeadowWorksTools/src/db"
	"context"
	"log"
	"net/http"
	"slices"
	"strconv"
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

// wantsDataDeleted is true when both delete boxes of the de-authorize
// prompts were ticked: "Delete this character's data", then "I understand the
// deletion of my data is irreversible". Otherwise characters are only
// de-authorized.
func wantsDataDeleted(r *http.Request) bool {
	return r.PostFormValue("delete-data") == "1" && r.PostFormValue("confirm-delete") == "1"
}

// handleDeauthorizeCharacter de-authorizes one of the signed-in user's
// characters: its token is revoked and deleted, so nothing more is read for
// it, and its saved data stays. With both delete boxes ticked its data is
// deleted too and it's removed from the user; removing the signed-in
// character switches the session to the user's next one, or signs out when it
// was the last.
func handleDeauthorizeCharacter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, _ := auth.CurrentSession(r)

	characterID, err := strconv.ParseInt(r.PostFormValue("character-id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid character id", http.StatusBadRequest)
		return
	}
	characters, err := db.ListUserCharacters(r.Context(), sess.UserID)
	if err != nil {
		log.Printf("deauthorize: listing characters of user %d: %v", sess.UserID, err)
		http.Error(w, "failed to de-authorize the character", http.StatusInternalServerError)
		return
	}
	i := slices.IndexFunc(characters, func(c db.UserCharacter) bool { return c.CharacterID == characterID })
	if i < 0 {
		http.Error(w, db.ErrNotYourCharacter.Error(), http.StatusForbidden)
		return
	}

	deleteData := wantsDataDeleted(r)
	if err := deauthorizeCharacters(r.Context(), characters[i:i+1], deleteData); err != nil {
		http.Error(w, "failed to de-authorize the character", http.StatusInternalServerError)
		return
	}

	if deleteData && characterID == sess.CharacterID {
		remaining := slices.Delete(characters, i, i+1)
		if len(remaining) == 0 {
			auth.ClearSessionCookie(w, r)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		auth.SwitchSessionCharacter(r, remaining[0].CharacterID, remaining[0].CharacterName)
	}
	http.Redirect(w, r, "/characters", http.StatusSeeOther)
}

// handleDeauthorizeAll de-authorizes every character of the signed-in user,
// deleting their data too when both delete boxes were ticked, and signs out.
func handleDeauthorizeAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, _ := auth.CurrentSession(r)

	characters, err := db.ListUserCharacters(r.Context(), sess.UserID)
	if err != nil {
		log.Printf("deauthorize: listing characters of user %d: %v", sess.UserID, err)
		http.Error(w, "failed to de-authorize your characters", http.StatusInternalServerError)
		return
	}
	if err := deauthorizeCharacters(r.Context(), characters, wantsDataDeleted(r)); err != nil {
		http.Error(w, "failed to de-authorize your characters", http.StatusInternalServerError)
		return
	}

	auth.ClearSessionCookie(w, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// deauthorizeCharacters deletes each character's token and revokes it with
// EVE SSO, then, with deleteData, deletes their data.
func deauthorizeCharacters(ctx context.Context, characters []db.UserCharacter, deleteData bool) error {
	ids := make([]int64, len(characters))
	for i, character := range characters {
		ids[i] = character.CharacterID
		refreshToken, err := db.DeleteCharacterToken(ctx, character.CharacterID)
		if err != nil {
			log.Printf("deauthorize: deleting token of character %d (%s): %v", character.CharacterID, character.CharacterName, err)
			return err
		}
		// the token is already gone from the database, so a failed SSO revoke only gets logged
		if cfg := auth.Config(); cfg != nil && refreshToken != "" {
			if err := cfg.RevokeRefreshToken(ctx, refreshToken); err != nil {
				log.Printf("deauthorize: EVE SSO revoke for character %d: %v", character.CharacterID, err)
			}
		}
		log.Printf("deauthorize: de-authorized character %d (%s)", character.CharacterID, character.CharacterName)
	}
	if !deleteData || len(ids) == 0 {
		return nil
	}
	if err := db.DeleteCharacterData(ctx, ids); err != nil {
		log.Printf("deauthorize: deleting data of characters %v: %v", ids, err)
		return err
	}
	log.Printf("deauthorize: deleted the data of characters %v", ids)
	return nil
}
