package refresh

import (
	"MeadowWorksTools/src/db"
	"MeadowWorksTools/src/esi"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// TestRefreshSavesEverything runs a full refresh against a fake ESI and a
// real database. It needs TEST_DATABASE_URL pointing at a database the
// tools/db scripts have been run on; its lifetime_indy tables are emptied.
func TestRefreshSavesEverything(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	t.Setenv("DATABASE_URL", url)
	t.Setenv("TOKEN_ENCRYPTION_KEY", "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE=")
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, `TRUNCATE lifetime_indy.esi_tokens, lifetime_indy.character_wallet_journal,
		lifetime_indy.corp_wallet_journal, lifetime_indy.wallet_balances, lifetime_indy.loyalty_points,
		lifetime_indy.market_orders, lifetime_indy.assets, lifetime_indy.industry_jobs,
		lifetime_indy.mining_ledger, lifetime_indy.corp_mining_ledger, lifetime_indy.wallet_transactions,
		lifetime_indy.blueprints, lifetime_indy.networth_snapshots, lifetime_indy.places CASCADE`)
	if err != nil {
		t.Fatal(err)
	}

	routes := map[string]string{
		"/characters/7":                                        `{"name":"Skye","corporation_id":98000001}`,
		"/characters/7/wallet":                                 `336717024.87`,
		"/characters/7/wallet/journal":                         `[{"id":1,"date":"2026-10-13T22:20:25Z","ref_type":"market_escrow","description":"Market escrow release","amount":-256000000,"balance":336717024.8701,"context_id":6893305979,"context_id_type":"market_transaction_id","first_party_id":7,"second_party_id":7,"reason":""}]`,
		"/characters/7/loyalty/points":                         `[{"corporation_id":1000125,"loyalty_points":5000}]`,
		"/characters/7/orders":                                 `[{"order_id":100,"type_id":34,"is_buy_order":false,"price":5.5,"volume_total":1000,"volume_remain":400,"range":"region","duration":90,"issued":"2026-10-01T00:00:00Z","location_id":60003760,"region_id":10000002,"is_corporation":false}]`,
		"/characters/7/orders/history":                         `[{"order_id":99,"type_id":35,"is_buy_order":true,"price":10,"volume_total":10,"volume_remain":0,"min_volume":1,"escrow":0,"range":"station","duration":30,"issued":"2026-09-01T00:00:00Z","location_id":60003760,"region_id":10000002,"is_corporation":false,"state":"expired"}]`,
		"/characters/7/assets":                                 `[{"item_id":1,"type_id":34,"quantity":500,"location_id":60003760,"location_flag":"Hangar","location_type":"station","is_singleton":false},{"item_id":2,"type_id":587,"quantity":1,"location_id":60003760,"location_flag":"Hangar","location_type":"station","is_singleton":true}]`,
		"/characters/7/industry/jobs":                          `[{"job_id":5,"activity_id":1,"status":"delivered","installer_id":7,"blueprint_id":3,"blueprint_type_id":690,"blueprint_location_id":60003760,"output_location_id":60003760,"facility_id":60003760,"station_id":60003760,"product_type_id":587,"runs":1,"licensed_runs":10,"successful_runs":1,"cost":1500.5,"duration":3600,"start_date":"2026-10-01T00:00:00Z","end_date":"2026-10-01T01:00:00Z","completed_date":"2026-10-01T02:00:00Z","completed_character_id":7}]`,
		"/characters/7/mining":                                 `[{"date":"2026-10-13","solar_system_id":30000142,"type_id":17470,"quantity":3000}]`,
		"/corporations/98000001/wallets":                       `[{"division":1,"balance":1000.25},{"division":2,"balance":0}]`,
		"/corporations/98000001/wallets/1/journal":             `[{"id":2,"date":"2026-10-13T00:00:00Z","ref_type":"player_donation","description":"Donation","amount":1000,"balance":1000.25}]`,
		"/corporations/98000001/orders":                        `[]`,
		"/corporations/98000001/orders/history":                `[]`,
		"/corporations/98000001/assets":                        "403",
		"/corporations/98000001/industry/jobs":                 `[{"job_id":6,"activity_id":8,"status":"active","installer_id":7,"blueprint_id":4,"blueprint_type_id":691,"blueprint_location_id":1,"output_location_id":1,"facility_id":1035000000001,"location_id":1035000000001,"runs":1,"probability":0.4,"duration":7200,"start_date":"2026-10-13T00:00:00Z","end_date":"2026-10-13T02:00:00Z"}]`,
		"/corporation/98000001/mining/observers":               `[{"observer_id":1035000000001,"observer_type":"structure","last_updated":"2026-10-13"}]`,
		"/corporation/98000001/mining/observers/1035000000001": `[{"character_id":8,"recorded_corporation_id":98000001,"type_id":45490,"quantity":2000,"last_updated":"2026-10-13"}]`,
		"/universe/structures/1035000000001":                   `{"name":"Moon Drill","owner_id":98000001,"solar_system_id":30002187,"type_id":35835}`,
		"/characters/7/blueprints":                             `[{"item_id":3,"type_id":690,"location_id":60003760,"location_flag":"Hangar","quantity":-2,"runs":10,"material_efficiency":10,"time_efficiency":20}]`,
		"/corporations/98000001/blueprints":                    "403",
		"/universe/stations/60003760":                          `{"name":"Jita IV - Moon 4 - Caldari Navy Assembly Plant","system_id":30000142}`,
		"/universe/systems/30000142":                           `{"name":"Jita","constellation_id":20000020}`,
		"/universe/systems/30002187":                           `{"name":"Amarr","constellation_id":20000322}`,
		"/universe/constellations/20000020":                    `{"name":"Kimotoro","region_id":10000002}`,
		"/universe/constellations/20000322":                    `{"name":"Throne Worlds","region_id":10000043}`,
		"/universe/names":                                      `[{"id":17470,"name":"Veldspar II-Grade"},{"id":45490,"name":"Zeolites III-Grade"},{"id":30000142,"name":"Jita"},{"id":30002187,"name":"Amarr"},{"id":8,"name":"Miner Alt"},{"id":98000001,"name":"Meadow Works"},{"id":10000002,"name":"The Forge"},{"id":10000043,"name":"Domain"}]`,
	}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := routes[r.URL.Path]
		switch {
		case strings.HasPrefix(r.URL.Path, "/corporations/98000001/wallets/") && !ok:
			fmt.Fprint(w, `[]`)
		case !ok:
			http.NotFound(w, r)
		case body == "403":
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		default:
			fmt.Fprint(w, body)
		}
	}))
	defer fake.Close()

	scopes := []string{ScopeCharacterWallet, ScopeCharacterAssets, ScopeCharacterJobs, ScopeCharacterMining,
		ScopeCharacterOrders, ScopeLoyalty, ScopeCorporationWallet, ScopeCorporationAssets, ScopeCorporationJobs,
		ScopeCorporationMining, ScopeCorporationOrders, ScopeStructures, ScopeCharacterBPs, ScopeCorporationBPs}
	err = db.SaveCharacterToken(ctx, esi.CharacterToken{CharacterID: 7, CharacterName: "Skye", Scopes: scopes,
		AccessToken: "access", RefreshToken: "refresh", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}

	r := &Refresher{ESI: &esi.Client{BaseURL: fake.URL}}
	r.All(ctx)
	r.All(ctx) // a second run updates rows instead of duplicating them

	counts := map[string]int{
		"character_wallet_journal": 1, "corp_wallet_journal": 1, "wallet_balances": 3, "loyalty_points": 1,
		"market_orders": 2, "assets": 2, "industry_jobs": 2, "mining_ledger": 1, "corp_mining_ledger": 1,
		"blueprints": 1, "places": 2, "networth_snapshots": 2,
	}
	for table, want := range counts {
		var got int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM lifetime_indy."+table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s has %d rows, want %d", table, got, want)
		}
	}

	var state string
	conn.QueryRow(ctx, `SELECT state FROM lifetime_indy.market_orders WHERE order_id = 99`).Scan(&state)
	if state != "expired" {
		t.Errorf("order 99 state %q", state)
	}
	var observer, system, miner, baseOre string
	conn.QueryRow(ctx, `SELECT observer_name, solar_system_name, character_name, base_ore FROM lifetime_indy.corp_mining_ledger`).Scan(&observer, &system, &miner, &baseOre)
	if observer != "Moon Drill" || system != "Amarr" || miner != "Miner Alt" || baseOre != "Zeolites" {
		t.Errorf("corp mining %q %q %q %q", observer, system, miner, baseOre)
	}
	var stationName, structureRegion string
	conn.QueryRow(ctx, `SELECT name FROM lifetime_indy.places WHERE location_id = 60003760`).Scan(&stationName)
	conn.QueryRow(ctx, `SELECT region FROM lifetime_indy.places WHERE location_id = 1035000000001`).Scan(&structureRegion)
	if stationName != "Jita IV - Moon 4 - Caldari Navy Assembly Plant" || structureRegion != "Domain" {
		t.Errorf("places %q, %q", stationName, structureRegion)
	}
	var assetPlace, blueprintPlace int64
	conn.QueryRow(ctx, `SELECT place_id FROM lifetime_indy.assets WHERE item_id = 1`).Scan(&assetPlace)
	conn.QueryRow(ctx, `SELECT place_id FROM lifetime_indy.blueprints WHERE item_id = 3`).Scan(&blueprintPlace)
	if assetPlace != 60003760 || blueprintPlace != 60003760 {
		t.Errorf("asset at %d, blueprint at %d", assetPlace, blueprintPlace)
	}
	var liquid float64
	conn.QueryRow(ctx, `SELECT liquid FROM lifetime_indy.networth_snapshots WHERE owner_id = 7`).Scan(&liquid)
	if liquid < 336717024 {
		t.Errorf("character snapshot liquid %v", liquid)
	}
	var encrypted []byte
	conn.QueryRow(ctx, `SELECT access_token FROM lifetime_indy.esi_tokens WHERE character_id = 7`).Scan(&encrypted)
	if strings.Contains(string(encrypted), "access") {
		t.Errorf("token stored in plain text")
	}
}
