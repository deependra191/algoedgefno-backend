package csvutil

import (
	"fmt"
	"strconv"
)

// ParseFloat parses a numeric string from a CSV field.
// Returns an error for empty or dash-only values — a common convention in data
// provider CSV files to indicate missing numeric data.
func ParseFloat(s string) (float64, error) {
	if s == "" || s == "-" {
		return 0, fmt.Errorf("empty or missing value")
	}
	return strconv.ParseFloat(s, 64)
}

// ParseInt64 parses an integer string from a CSV field.
// Returns zero for empty or dash-only values — a common convention in data
// provider CSV files to indicate missing numeric data.
func ParseInt64(s string) (int64, error) {
	if s == "" || s == "-" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}
