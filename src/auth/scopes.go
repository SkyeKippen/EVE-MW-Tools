package auth

import (
	"net/http"
	"slices"
)

// RequiredScope is always requested at sign-in, whatever the user picks.
const RequiredScope = "publicData"

// ScopeChoice is a scope the user may leave out at sign-in.
type ScopeChoice struct {
	Scope       string
	Label       string
	Description string
	Note        string // a caveat shown under the description, if any
}

// optionalScopes are the scopes shown as switches on the sign-in form, in
// the order shown. A scope only shows once it is also listed in ESI_SCOPES.
var optionalScopes = []ScopeChoice{
	{Scope: "esi-wallet.read_character_wallet.v1", Label: "Character wallet"},
	{Scope: "esi-assets.read_assets.v1", Label: "Character assets"},
	{Scope: "esi-characters.read_blueprints.v1", Label: "Character blueprints"},
	{Scope: "esi-industry.read_character_jobs.v1", Label: "Character industry jobs"},
	{Scope: "esi-industry.read_character_mining.v1", Label: "Character mining ledger"},
	{Scope: "esi-markets.read_character_orders.v1", Label: "Character market orders"},
	{Scope: "esi-characters.read_loyalty.v1", Label: "Loyalty points"},
	{Scope: "esi-wallet.read_corporation_wallet.v1", Label: "Corporation wallet"},
	{Scope: "esi-wallet.read_corporation_wallets.v1", Label: "Corporation wallets"},
	{Scope: "esi-assets.read_corporation_assets.v1", Label: "Corporation assets"},
	{Scope: "esi-corporations.read_blueprints.v1", Label: "Corporation blueprints"},
	{Scope: "esi-industry.read_corporation_jobs.v1", Label: "Corporation industry jobs"},
	{Scope: "esi-industry.read_corporation_mining.v1", Label: "Corporation mining"},
	{Scope: "esi-markets.read_corporation_orders.v1", Label: "Corporation market orders"},
	{Scope: "esi-corporations.read_divisions.v1", Label: "Corporation divisions"},
	{Scope: "esi-universe.read_structures.v1", Label: "Structures"},
	{Scope: "esi-markets.structure_markets.v1", Label: "Structure markets",
		Description: "Reads the C-J6MT Keepstar's market once a day for the Import and Prices pages.",
		Note:        "The character needs docking access there."},
}

// chosenParam marks a sign-in that came from the scope picker, so an empty
// selection means "none of the optional scopes" rather than "not asked".
const chosenParam = "scopes_chosen"

// ScopeChoices returns the optional scopes that ESI_SCOPES requests, for the
// sign-in form.
func ScopeChoices() []ScopeChoice {
	var choices []ScopeChoice
	for _, choice := range optionalScopes {
		if slices.Contains(cfg.Scopes, choice.Scope) {
			choices = append(choices, choice)
		}
	}
	return choices
}

// requestedScopes returns the scopes to send to EVE SSO for this sign-in.
// Optional scopes are dropped when the user unticked them on the sign-in form;
// a plain /auth/login asks for every configured scope. RequiredScope is always
// included, and nothing outside ESI_SCOPES can be added from the request.
func requestedScopes(r *http.Request) []string {
	q := r.URL.Query()
	chosen := q.Has(chosenParam)
	picked := q["scope"]

	scopes := []string{RequiredScope}
	for _, scope := range cfg.Scopes {
		if scope == RequiredScope {
			continue
		}
		if chosen && isOptional(scope) && !slices.Contains(picked, scope) {
			continue
		}
		scopes = append(scopes, scope)
	}
	return scopes
}

func isOptional(scope string) bool {
	return slices.ContainsFunc(optionalScopes, func(choice ScopeChoice) bool {
		return choice.Scope == scope
	})
}
