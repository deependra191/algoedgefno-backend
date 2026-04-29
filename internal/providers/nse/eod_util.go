package nse

import (
	"fmt"
	"strconv"
	"time"
)

// ist is the Indian Standard Time location (UTC+5:30). NSE operates on IST.
var ist = time.FixedZone("IST", 5*60*60+30*60)

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

func parseFloat(s string) (float64, error) {
	if s == "" || s == "-" {
		return 0, fmt.Errorf("empty or missing value")
	}
	return strconv.ParseFloat(s, 64)
}

func parseInt64(s string) (int64, error) {
	if s == "" || s == "-" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

// parseExpiry parses a contract expiry date string, trying multiple NSE-observed formats.
// Returns a zero time.Time for empty, missing, or unrecognised values — callers treat zero as "no expiry".
func parseExpiry(s string) time.Time {
	if s == "" || s == "-" {
		return time.Time{}
	}
	// Try common NSE date formats.
	formats := []string{"02-Jan-2006", "2006-01-02", "02-JAN-2006", "01/02/2006"}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// parseDate parses a date string from a bhavcopy timestamp column,
// trying multiple NSE-observed formats. Returns an error if none match.
func parseDate(s string) (time.Time, error) {
	formats := []string{
		"02-Jan-2006",
		"2006-01-02",
		"20060102",
		"01/02/2006",
		"02-01-2006", // DD-MM-YYYY with dashes — used by NSE indices bhavcopy "Index Date" column
	}
	for _, f := range formats {
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
