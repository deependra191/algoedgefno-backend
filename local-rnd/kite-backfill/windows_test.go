package main

import (
	"testing"
	"time"
)

func TestSplitDateWindows_MaxSixtyCalendarDays(t *testing.T) {
	loc := time.UTC
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, loc)
	to := time.Date(2025, 3, 2, 0, 0, 0, 0, loc)

	windows := splitDateWindows(from, to)
	if len(windows) != 2 {
		t.Fatalf("expected 2 windows, got %d", len(windows))
	}
	if !windows[0].FromDate.Equal(from) || !windows[0].ToDate.Equal(time.Date(2025, 3, 1, 0, 0, 0, 0, loc)) {
		t.Fatalf("first window = %#v", windows[0])
	}
	if !windows[1].FromDate.Equal(time.Date(2025, 3, 2, 0, 0, 0, 0, loc)) || !windows[1].ToDate.Equal(to) {
		t.Fatalf("second window = %#v", windows[1])
	}
}

func TestSplitDateWindows_ExactlySixtyDays(t *testing.T) {
	loc := time.UTC
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, loc)
	to := time.Date(2025, 3, 1, 0, 0, 0, 0, loc)

	windows := splitDateWindows(from, to)
	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}
	if !windows[0].FromDate.Equal(from) || !windows[0].ToDate.Equal(to) {
		t.Fatalf("window = %#v", windows[0])
	}
}
