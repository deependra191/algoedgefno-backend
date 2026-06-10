package main

import (
	"testing"
	"time"
)

func TestParseOptions_CurrentFNODoesNotImplicitlyAddDefaultSpotSymbols(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	opts, err := parseOptions([]string{"-current-fno", "NIFTY26JUNFUT"}, now)
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}
	if len(opts.symbols) != 0 {
		t.Fatalf("expected no implicit spot symbols for current-fno mode, got %v", opts.symbols)
	}
	if len(opts.currentFNO) != 1 || opts.currentFNO[0] != "NIFTY26JUNFUT" {
		t.Fatalf("current fno = %v", opts.currentFNO)
	}
}

func TestParseOptions_ReplaceRequiresReason(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	if _, err := parseOptions([]string{"-symbols", "RELIANCE", "-replace"}, now); err == nil {
		t.Fatal("expected -replace without reason to fail")
	}
}

func TestParseOptions_DailyRejectsCurrentFNO(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	if _, err := parseOptions([]string{"-daily", "-current-fno", "NIFTY26JUNFUT"}, now); err == nil {
		t.Fatal("expected -daily with -current-fno to fail")
	}
}
