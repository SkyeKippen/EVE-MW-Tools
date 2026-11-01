package refresh

import (
	"MeadowWorksTools/src/history"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestNextHistoryCheck(t *testing.T) {
	cases := map[string]string{
		"2026-11-01T08:00:00Z": "2026-11-01T13:00:00Z",
		"2026-11-01T13:00:00Z": "2026-11-02T13:00:00Z",
		"2026-11-01T23:59:00Z": "2026-11-02T13:00:00Z",
	}
	for now, want := range cases {
		n, _ := time.Parse(time.RFC3339, now)
		if got := nextHistoryCheck(n).Format(time.RFC3339); got != want {
			t.Errorf("after %s: got %s, want %s", now, got, want)
		}
	}
}

// TestSyncMarketHistory imports from a fake EVE Ref into a real database,
// then checks again after a day's file has grown and a new day appeared. It
// needs TEST_DATABASE_URL pointing at a database the tools/db scripts have
// been run on; its market history tables are emptied.
func TestSyncMarketHistory(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	t.Setenv("DATABASE_URL", url)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `TRUNCATE lifetime_indy.market_history, lifetime_indy.market_history_days`); err != nil {
		t.Fatal(err)
	}

	// 2025-01-02 has three good rows, a bad one and one dated the day before
	totals := `{"2025-01-02": 3, "2025-01-03": 0}`
	files := map[string][]byte{
		"/2025/market-history-2025-01-02.csv.bz2": testFile(t, "market-history-2025-01-02.csv.bz2"),
		"/2003/market-history-2003-05-01.csv.bz2": testFile(t, "market-history-2003-05-01.csv.bz2"),
	}
	var downloads int
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/totals.json":
			w.Write([]byte(totals))
		case r.URL.Path == "/2003/":
			w.Write([]byte(`<a href="market-history-2003-05-01.csv.bz2">market-history-2003-05-01.csv.bz2</a>`))
		case files[r.URL.Path] != nil:
			downloads++
			w.Write(files[r.URL.Path])
		default:
			http.NotFound(w, r)
		}
	}))
	defer fake.Close()
	client := &history.Client{BaseURL: fake.URL}

	if err := SyncMarketHistory(ctx, client); err != nil {
		t.Fatal(err)
	}
	var count int
	conn.QueryRow(ctx, `SELECT count(*) FROM lifetime_indy.market_history`).Scan(&count)
	if count != 4 || downloads != 2 {
		t.Fatalf("after the first check: %d rows from %d downloads, want 4 from 2", count, downloads)
	}

	// nothing changed: nothing is downloaded
	if err := SyncMarketHistory(ctx, client); err != nil {
		t.Fatal(err)
	}
	if downloads != 2 {
		t.Errorf("a check with nothing new downloaded %d files", downloads-2)
	}

	// 2025-01-02 grows (with a changed price) and 2025-01-03 appears
	totals = `{"2025-01-02": 4, "2025-01-03": 1}`
	files["/2025/market-history-2025-01-02.csv.bz2"] = testFile(t, "grown-2025-01-02.csv.bz2")
	files["/2025/market-history-2025-01-03.csv.bz2"] = testFile(t, "market-history-2025-01-03.csv.bz2")
	if err := SyncMarketHistory(ctx, client); err != nil {
		t.Fatal(err)
	}
	if downloads != 4 {
		t.Errorf("downloaded %d files on the third check, want 2", downloads-2)
	}
	conn.QueryRow(ctx, `SELECT count(*) FROM lifetime_indy.market_history`).Scan(&count)
	if count != 6 {
		t.Errorf("%d rows after the third check, want 6", count)
	}
	var average float64
	conn.QueryRow(ctx, `SELECT average FROM lifetime_indy.market_history
		WHERE type_id = 34 AND region_id = 10000002 AND date = '2025-01-02'`).Scan(&average)
	if average != 4.6 {
		t.Errorf("updated average = %v, want 4.6", average)
	}
	var rows, expected int
	conn.QueryRow(ctx, `SELECT rows, expected FROM lifetime_indy.market_history_days WHERE date = '2025-01-02'`).Scan(&rows, &expected)
	if rows != 4 || expected != 4 {
		t.Errorf("2025-01-02 recorded %d rows of %d, want 4 of 4", rows, expected)
	}
}

// testFile reads one of the history package's test files.
func testFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("../history/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
