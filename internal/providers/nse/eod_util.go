package nse

import (
	"fmt"
	"time"
)

const (
	istOffsetSeconds = 5*60*60 + 30*60

	// NSE CSV date format strings. Go's reference time is Mon Jan 2 15:04:05 MST 2006.
	nseDateFmtDMY      = "02-Jan-2006" // e.g. 15-Jan-2025 — F&O expiry, bhavcopy timestamp
	nseDateFmtISO      = "2006-01-02"  // e.g. 2025-01-15 — standard ISO date
	nseDateFmtDMYUpper = "02-JAN-2006" // e.g. 15-JAN-2025 — NSE uppercase variant
	nseDateFmtSlashMDY = "01/02/2006"  // e.g. 01/15/2025 — slash-separated
	nseDateFmtCompact  = "20060102"    // e.g. 20250115 — compact no-separator
	nseDateFmtDDMMYYYY = "02-01-2006"  // e.g. 15-01-2025 — NSE indices bhavcopy "Index Date"
)

// ist is the Indian Standard Time location (UTC+5:30). NSE operates on IST.
var ist = time.FixedZone("IST", istOffsetSeconds)

// latestTradingDate returns the most recent weekday (Mon–Fri) in IST.
func latestTradingDate() time.Time {
	t := time.Now().In(ist).Truncate(24 * time.Hour)
	return prevTradingDayOrToday(t)
}

// prevTradingDayOrToday returns t itself unless it falls on a weekend,
// in which case it rolls back to the preceding Friday.
func prevTradingDayOrToday(t time.Time) time.Time {
	switch t.Weekday() {
	case time.Sunday:
		return t.AddDate(0, 0, -2)
	case time.Saturday:
		return t.AddDate(0, 0, -1)
	}
	return t
}

// prevTradingDay returns the trading day immediately before t, skipping weekends.
func prevTradingDay(t time.Time) time.Time {
	t = t.AddDate(0, 0, -1)
	return prevTradingDayOrToday(t)
}

// col returns the index of the first matching column name, or -1 if none found.
func col(idx map[string]int, names ...string) int {
	for _, name := range names {
		if i, ok := idx[name]; ok {
			return i
		}
	}
	return -1
}

// parseExpiry parses a contract expiry date string, trying multiple NSE-observed formats.
// Returns a zero time.Time for empty, missing, or unrecognised values — callers treat zero as "no expiry".
func parseExpiry(s string) time.Time {
	if s == "" || s == "-" {
		return time.Time{}
	}
	for _, f := range []string{nseDateFmtDMY, nseDateFmtISO, nseDateFmtDMYUpper, nseDateFmtSlashMDY} {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// parseDate parses a date string from a bhavcopy timestamp column,
// trying multiple NSE-observed formats. Returns an error if none match.
func parseDate(s string) (time.Time, error) {
	for _, f := range []string{nseDateFmtDMY, nseDateFmtISO, nseDateFmtCompact, nseDateFmtSlashMDY, nseDateFmtDDMMYYYY} {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date: %s", s)
}

// instrumentKey is used to build a (symbol, exchange) lookup map for candle FK resolution.
// Both fields are needed because the same symbol can exist in NSE and NFO
// (e.g. NIFTY index on NSE vs NIFTY futures on NFO).
type instrumentKey struct {
	Symbol   string
	Exchange string
}
