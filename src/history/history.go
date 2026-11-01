// Package history reads EVE Ref's archive of CCP's daily market history:
// for every region and item type, each day's average, highest and lowest
// price, units traded and number of orders. EVE Ref publishes one
// bzip2-compressed CSV per day at data.everef.net/market-history/, plus
// totals.json, which says how many records each day's file has.
//
// EVE Ref builds the files from ESI's history route, which it scans region by
// region over most of a day, so a day's file can appear days late and keep
// growing for a while after it first appears. Comparing totals.json against
// what's saved tells which days are new or have grown.
package history

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Row is one region's history for one item type on one day.
type Row struct {
	Date       time.Time
	RegionID   int64
	TypeID     int64
	Average    float64
	Highest    float64
	Lowest     float64
	Volume     int64 // units traded
	OrderCount int64
}

// columns are the CSV columns a day's file must have. Others (such as
// http_last_modified) are ignored.
var columns = []string{"date", "region_id", "type_id", "average", "highest", "lowest", "volume", "order_count"}

// ParseCSV reads one day's CSV, finding the columns by their header names.
// A line that can't be read is skipped and counted in skipped, so one bad
// line doesn't lose the day.
func ParseCSV(r io.Reader) (rows []Row, skipped int, err error) {
	reader := csv.NewReader(r)
	reader.ReuseRecord = true
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return nil, 0, fmt.Errorf("reading the header: %w", err)
	}
	index := map[string]int{}
	for i, name := range header {
		index[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "\ufeff")))] = i
	}
	at := make([]int, len(columns))
	for i, name := range columns {
		column, ok := index[name]
		if !ok {
			return nil, 0, fmt.Errorf("no %q column in %v", name, header)
		}
		at[i] = column
	}

	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return rows, skipped, nil
		}
		var parseErr *csv.ParseError
		if errors.As(err, &parseErr) {
			skipped++
			continue
		}
		if err != nil {
			return nil, skipped, err
		}
		row, ok := parseRecord(record, at)
		if !ok {
			skipped++
			continue
		}
		rows = append(rows, row)
	}
}

func parseRecord(record []string, at []int) (Row, bool) {
	field := func(i int) string {
		if at[i] >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[at[i]])
	}
	var row Row
	var err error
	var errs []error
	date := field(0)
	if len(date) > len(time.DateOnly) {
		date = date[:len(time.DateOnly)] // "2025-01-02T00:00:00Z" or "2025-01-02"
	}
	row.Date, err = time.Parse(time.DateOnly, date)
	errs = append(errs, err)
	row.RegionID, err = strconv.ParseInt(field(1), 10, 64)
	errs = append(errs, err)
	row.TypeID, err = strconv.ParseInt(field(2), 10, 64)
	errs = append(errs, err)
	row.Average, err = strconv.ParseFloat(field(3), 64)
	errs = append(errs, err)
	row.Highest, err = strconv.ParseFloat(field(4), 64)
	errs = append(errs, err)
	row.Lowest, err = strconv.ParseFloat(field(5), 64)
	errs = append(errs, err)
	row.Volume, err = parseCount(field(6))
	errs = append(errs, err)
	row.OrderCount, err = parseCount(field(7))
	errs = append(errs, err)
	return row, errors.Join(errs...) == nil
}

// parseCount reads a whole number, also when it's written as "12.0".
func parseCount(s string) (int64, error) {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return int64(f), nil
}

// Day is a UTC calendar day.
type Day = time.Time

// ParseDay reads "2006-01-02".
func ParseDay(s string) (Day, error) {
	return time.Parse(time.DateOnly, s)
}

// Saved is what's already saved for a day: how many rows were saved, and how
// many records totals.json listed for the day when it was saved (0 when it
// listed none).
type Saved struct {
	Rows     int
	Expected int
}

// ToFetch returns the days worth downloading, newest first: every day
// totals.json lists with records that either hasn't been saved or has grown
// since it was saved, plus the listed days (days totals.json doesn't cover)
// that haven't been saved at all. Days listed with 0 records aren't
// published yet.
func ToFetch(totals map[Day]int, listed []Day, saved map[Day]Saved) []Day {
	var days []Day
	for day, count := range totals {
		if count <= 0 {
			continue
		}
		have, ok := saved[day]
		if !ok || count > have.Expected {
			days = append(days, day)
		}
	}
	for _, day := range listed {
		if _, inTotals := totals[day]; inTotals {
			continue
		}
		if _, ok := saved[day]; !ok {
			days = append(days, day)
		}
	}
	slices.SortFunc(days, func(a, b Day) int { return b.Compare(a) })
	return slices.Compact(days)
}

// ParseTotals reads totals.json: {"2022-02-22": 46321, ...}.
func ParseTotals(raw map[string]int) (map[Day]int, error) {
	totals := make(map[Day]int, len(raw))
	for key, count := range raw {
		day, err := ParseDay(key)
		if err != nil {
			return nil, fmt.Errorf("totals.json has a key %q that isn't a date", key)
		}
		totals[day] = count
	}
	return totals, nil
}

var listedFile = regexp.MustCompile(`market-history-(\d{4}-\d{2}-\d{2})\.csv\.bz2`)

// ParseListing finds the days with a file in a year folder's HTML listing.
func ParseListing(html string) []Day {
	var days []Day
	for _, match := range listedFile.FindAllStringSubmatch(html, -1) {
		if day, err := ParseDay(match[1]); err == nil {
			days = append(days, day)
		}
	}
	slices.SortFunc(days, func(a, b Day) int { return a.Compare(b) })
	return slices.Compact(days)
}

// FirstDay returns the earliest day in totals, and false when it's empty.
func FirstDay(totals map[Day]int) (Day, bool) {
	var first Day
	for day := range totals {
		if first.IsZero() || day.Before(first) {
			first = day
		}
	}
	return first, !first.IsZero()
}
