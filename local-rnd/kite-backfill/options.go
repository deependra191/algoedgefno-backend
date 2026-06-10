package main

import (
	"flag"
	"fmt"
	"strings"
	"time"
)

type options struct {
	symbols          []string
	currentFNO       []string
	symbolsExplicit  bool
	fromDate         time.Time
	toDate           time.Time
	delay            time.Duration
	timeout          time.Duration
	daily            bool
	replace          bool
	replaceReason    string
	snapshotDir      string
	replacementRoot  string
	intradayLocation *time.Location
}

func parseOptions(args []string, now time.Time) (options, error) {
	loc, err := time.LoadLocation(istLocationName)
	if err != nil {
		return options{}, fmt.Errorf("load IST location: %w", err)
	}

	fs := flag.NewFlagSet("kite-backfill", flag.ContinueOnError)
	symbolsCSV := fs.String("symbols", "", "comma-separated NSE index/equity symbols to import")
	currentFNOCSV := fs.String("current-fno", "", "comma-separated currently listed NFO futures contract symbols to import")
	fromValue := fs.String("from", defaultFromDate, "backfill start date in YYYY-MM-DD")
	toValue := fs.String("to", now.In(loc).Format(dateLayout), "backfill end date in YYYY-MM-DD")
	delay := fs.Duration("delay", defaultDelay, "delay between Kite historical requests")
	timeout := fs.Duration("timeout", defaultTimeout, "timeout for each Kite/API/DB operation")
	daily := fs.Bool("daily", false, "fetch Kite day candles and store them as repo 1d candles for spot/index/equity symbols")
	replace := fs.Bool("replace", false, "delete and reload the selected instrument/date/interval range after backing it up")
	replaceReason := fs.String("replace-reason", "", "required operator reason when -replace is set")
	snapshotDir := fs.String("snapshot-dir", defaultSnapshotDir, "directory for Kite instruments CSV snapshots")

	if err := fs.Parse(args); err != nil {
		return options{}, err
	}

	symbolsExplicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "symbols" {
			symbolsExplicit = true
		}
	})

	if !symbolsExplicit && strings.TrimSpace(*currentFNOCSV) == "" {
		*symbolsCSV = defaultSymbolsCSV
	}

	fromDate, err := parseDate(*fromValue, loc)
	if err != nil {
		return options{}, err
	}
	toDate, err := parseDate(*toValue, loc)
	if err != nil {
		return options{}, err
	}
	if toDate.Before(fromDate) {
		return options{}, fmt.Errorf("-to must be on or after -from")
	}
	if *delay < 0 {
		return options{}, fmt.Errorf("-delay must be non-negative")
	}
	if *timeout <= 0 {
		return options{}, fmt.Errorf("-timeout must be positive")
	}
	if *daily && strings.TrimSpace(*currentFNOCSV) != "" {
		return options{}, fmt.Errorf("-daily cannot be combined with -current-fno")
	}
	reason := strings.TrimSpace(*replaceReason)
	if *replace && reason == "" {
		return options{}, fmt.Errorf("-replace requires -replace-reason")
	}

	opts := options{
		symbols:          splitCSV(*symbolsCSV),
		currentFNO:       splitCSV(*currentFNOCSV),
		symbolsExplicit:  symbolsExplicit,
		fromDate:         fromDate,
		toDate:           toDate,
		delay:            *delay,
		timeout:          *timeout,
		daily:            *daily,
		replace:          *replace,
		replaceReason:    reason,
		snapshotDir:      strings.TrimSpace(*snapshotDir),
		replacementRoot:  replacementRootPath,
		intradayLocation: loc,
	}
	if len(opts.symbols) == 0 && len(opts.currentFNO) == 0 {
		return options{}, fmt.Errorf("at least one -symbols or -current-fno target is required")
	}
	if opts.snapshotDir == "" {
		return options{}, fmt.Errorf("-snapshot-dir is required")
	}
	return opts, nil
}

func parseDate(value string, loc *time.Location) (time.Time, error) {
	t, err := time.ParseInLocation(dateLayout, strings.TrimSpace(value), loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse date %q: %w", value, err)
	}
	return t, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		key := strings.ToUpper(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, item)
	}
	return items
}
