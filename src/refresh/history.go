package refresh

import (
	"MeadowWorksTools/src/db"
	"MeadowWorksTools/src/history"
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

// The market history is checked once a day. EVE Ref's scan starts after
// downtime and takes most of the day, and a day's file can take days to
// appear, so the exact hour doesn't matter much: each check picks up
// whatever has appeared or grown since the last one.
const (
	historyCheckHourUTC = 13
	// giving up after this many failed downloads in a row means EVE Ref or
	// the network is down; the next check carries on
	historyMaxFailures = 5
)

// MarketHistoryDaily imports EVE Ref's market history now, then once a day.
// The first run imports the whole archive, which takes hours; it's saved day
// by day, so a restart carries on where it stopped.
func MarketHistoryDaily(client *history.Client) {
	for {
		if err := SyncMarketHistory(context.Background(), client); err != nil {
			log.Printf("market history: %v", err)
		}
		time.Sleep(time.Until(nextHistoryCheck(time.Now())))
	}
}

// nextHistoryCheck is the next historyCheckHourUTC after now.
func nextHistoryCheck(now time.Time) time.Time {
	now = now.UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), historyCheckHourUTC, 0, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// SyncMarketHistory imports every day EVE Ref has that isn't saved yet, or
// that has grown since it was saved, newest first. Days totals.json lists
// come from it; years before the first of those are found from EVE Ref's
// year folders. A day that isn't published yet is left for the next check.
func SyncMarketHistory(ctx context.Context, client *history.Client) error {
	totals, err := client.Totals(ctx)
	if err != nil {
		return fmt.Errorf("fetching totals.json: %w", err)
	}
	var listed []history.Day
	lastListedYear := time.Now().UTC().Year()
	if first, ok := history.FirstDay(totals); ok {
		lastListedYear = first.Year()
	}
	for year := history.FirstYear; year <= lastListedYear; year++ {
		days, err := client.Listed(ctx, year)
		if err != nil {
			return fmt.Errorf("listing %d: %w", year, err)
		}
		listed = append(listed, days...)
	}
	saved, err := db.LoadMarketHistoryDays(ctx)
	if err != nil {
		return fmt.Errorf("loading saved days: %w", err)
	}

	days := history.ToFetch(totals, listed, saved)
	if len(days) == 0 {
		return nil
	}
	log.Printf("market history: %d days to import, %s back to %s", len(days),
		days[0].Format(time.DateOnly), days[len(days)-1].Format(time.DateOnly))

	start := time.Now()
	var imported, notPublished, failures, totalRows int
	for _, day := range days {
		if err := ctx.Err(); err != nil {
			return err
		}
		rows, skipped, err := client.Day(ctx, day)
		if err == nil {
			err = db.SaveMarketHistoryDay(ctx, day, rows, totals[day])
		}
		switch {
		case errors.Is(err, history.ErrNotPublished):
			notPublished++
			continue
		case err != nil:
			failures++
			log.Printf("market history: %s: %v", day.Format(time.DateOnly), err)
			if failures >= historyMaxFailures {
				return fmt.Errorf("stopped after %d failures in a row; imported %d days", failures, imported)
			}
			continue
		}
		failures = 0
		imported++
		totalRows += len(rows)
		if skipped > 0 {
			log.Printf("market history: %s: skipped %d unreadable lines", day.Format(time.DateOnly), skipped)
		}
		if imported%25 == 0 {
			log.Printf("market history: imported %d of %d days (%d rows) in %s", imported, len(days), totalRows,
				time.Since(start).Round(time.Second))
		}
	}
	log.Printf("market history: imported %d days (%d rows) in %s; %d not published yet", imported, totalRows,
		time.Since(start).Round(time.Second), notPublished)
	return nil
}
