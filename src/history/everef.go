package history

import (
	"compress/bzip2"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultBaseURL is where EVE Ref publishes the market history archive.
const DefaultBaseURL = "https://data.everef.net/market-history"

// FirstYear is the first year folder EVE Ref's archive has.
const FirstYear = 2003

// ErrNotPublished means EVE Ref has no file for the day yet.
var ErrNotPublished = errors.New("not published yet")

// Client downloads from EVE Ref. The zero value is ready to use.
type Client struct {
	HTTPClient *http.Client
	BaseURL    string // defaults to DefaultBaseURL; tests point it at a fake server
	UserAgent  string
}

func (c *Client) base() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultBaseURL
}

func (c *Client) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return resp, nil
	case http.StatusNotFound:
		resp.Body.Close()
		return nil, fmt.Errorf("%s: %w", url, ErrNotPublished)
	default:
		resp.Body.Close()
		return nil, fmt.Errorf("%s answered %s", url, resp.Status)
	}
}

// Totals fetches totals.json: how many records each day's file has.
func (c *Client) Totals(ctx context.Context) (map[Day]int, error) {
	resp, err := c.get(ctx, c.base()+"/totals.json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var raw map[string]int
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("reading totals.json: %w", err)
	}
	return ParseTotals(raw)
}

// Listed returns the days with a file in a year's folder.
func (c *Client) Listed(ctx context.Context, year int) ([]Day, error) {
	resp, err := c.get(ctx, fmt.Sprintf("%s/%d/", c.base(), year))
	if errors.Is(err, ErrNotPublished) {
		return nil, nil // no folder for that year
	}
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	html, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	return ParseListing(string(html)), nil
}

// DayURL is where a day's file is published.
func (c *Client) DayURL(day Day) string {
	return fmt.Sprintf("%s/%d/market-history-%s.csv.bz2", c.base(), day.Year(), day.Format(time.DateOnly))
}

// Day downloads and reads one day's file. Rows dated another day are left
// out. skipped counts lines that couldn't be read.
func (c *Client) Day(ctx context.Context, day Day) (rows []Row, skipped int, err error) {
	resp, err := c.get(ctx, c.DayURL(day))
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	rows, skipped, err = ParseCSV(bzip2.NewReader(resp.Body))
	if err != nil {
		return nil, skipped, fmt.Errorf("%s: %w", c.DayURL(day), err)
	}
	kept := rows[:0]
	for _, row := range rows {
		if row.Date.Equal(day) {
			kept = append(kept, row)
		} else {
			skipped++
		}
	}
	return kept, skipped, nil
}
