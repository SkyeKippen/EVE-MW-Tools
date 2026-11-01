package history

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func day(s string) Day {
	d, err := ParseDay(s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestParseCSVFindsColumnsByName(t *testing.T) {
	// columns in another order, with an extra one, a BOM and a bad line
	input := "\ufefftype_id,region_id,date,order_count,volume,lowest,highest,average,extra\n" +
		"34,10000002,2025-01-02,1500,123456789,4.1,4.9,4.5,x\n" +
		"35,10000002,2025-01-02,12.0,1000,9.8,10,9.9,x\n" +
		"oops,10000002,2025-01-02,1,1,1,1,1,x\n"
	rows, skipped, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 1 {
		t.Errorf("skipped %d lines, want 1", skipped)
	}
	want := []Row{
		{Date: day("2025-01-02"), RegionID: 10000002, TypeID: 34, Average: 4.5, Highest: 4.9, Lowest: 4.1, Volume: 123456789, OrderCount: 1500},
		{Date: day("2025-01-02"), RegionID: 10000002, TypeID: 35, Average: 9.9, Highest: 10, Lowest: 9.8, Volume: 1000, OrderCount: 12},
	}
	if !slices.Equal(rows, want) {
		t.Errorf("got %+v\nwant %+v", rows, want)
	}
}

func TestParseCSVNeedsEveryColumn(t *testing.T) {
	_, _, err := ParseCSV(strings.NewReader("date,region_id,type_id,average,highest,lowest,volume\n"))
	if err == nil || !strings.Contains(err.Error(), "order_count") {
		t.Errorf("got %v, want an error naming order_count", err)
	}
}

func TestToFetch(t *testing.T) {
	totals := map[Day]int{
		day("2025-01-01"): 50000, // saved with the same count: skip
		day("2025-01-02"): 51000, // grew since it was saved: again
		day("2025-01-03"): 20000, // new
		day("2025-01-04"): 0,     // not published yet
	}
	listed := []Day{
		day("2003-05-01"), // not in totals, not saved: fetch
		day("2003-05-02"), // not in totals, saved: skip
		day("2025-01-01"), // in totals: totals decides
	}
	saved := map[Day]Saved{
		day("2025-01-01"): {Rows: 50000, Expected: 50000},
		day("2025-01-02"): {Rows: 49000, Expected: 49000},
		day("2003-05-02"): {Rows: 100},
	}
	got := ToFetch(totals, listed, saved)
	want := []Day{day("2025-01-03"), day("2025-01-02"), day("2003-05-01")}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseListing(t *testing.T) {
	html := `<a href="market-history-2003-05-02.csv.bz2">market-history-2003-05-02.csv.bz2</a>
		<a href="market-history-2003-05-01.csv.bz2">x</a><a href="totals.json">totals.json</a>`
	got := ParseListing(html)
	want := []Day{day("2003-05-01"), day("2003-05-02")}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestClient(t *testing.T) {
	fixture, err := os.ReadFile("testdata/market-history-2025-01-02.csv.bz2")
	if err != nil {
		t.Fatal(err)
	}
	var userAgent string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.Header.Get("User-Agent")
		switch r.URL.Path {
		case "/totals.json":
			w.Write([]byte(`{"2025-01-02": 4, "2025-01-03": 0}`))
		case "/2025/market-history-2025-01-02.csv.bz2":
			w.Write(fixture)
		case "/2003/":
			w.Write([]byte(`<a href="market-history-2003-05-01.csv.bz2">`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer fake.Close()
	client := &Client{BaseURL: fake.URL, UserAgent: "test-agent"}
	ctx := context.Background()

	totals, err := client.Totals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if totals[day("2025-01-02")] != 4 || len(totals) != 2 {
		t.Errorf("totals = %v", totals)
	}
	if userAgent != "test-agent" {
		t.Errorf("User-Agent = %q", userAgent)
	}

	rows, skipped, err := client.Day(ctx, day("2025-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	// the bad region and the row dated the day before are skipped
	if len(rows) != 3 || skipped != 2 {
		t.Errorf("got %d rows and %d skipped, want 3 and 2: %+v", len(rows), skipped, rows)
	}
	if rows[1].TypeID != 44992 || rows[1].Average != 2401234.56 || rows[1].Volume != 25000 {
		t.Errorf("PLEX row = %+v", rows[1])
	}

	if _, _, err := client.Day(ctx, day("2025-01-03")); !errors.Is(err, ErrNotPublished) {
		t.Errorf("missing day: got %v, want ErrNotPublished", err)
	}
	listed, err := client.Listed(ctx, 2003)
	if err != nil || !slices.Equal(listed, []Day{day("2003-05-01")}) {
		t.Errorf("listed 2003 = %v, %v", listed, err)
	}
	if listed, err := client.Listed(ctx, 2004); err != nil || len(listed) != 0 {
		t.Errorf("a year with no folder: %v, %v", listed, err)
	}
}

func TestDayURL(t *testing.T) {
	got := (&Client{}).DayURL(time.Date(2024, 3, 9, 0, 0, 0, 0, time.UTC))
	want := "https://data.everef.net/market-history/2024/market-history-2024-03-09.csv.bz2"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
