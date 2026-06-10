package main

import (
	"strconv"
	"strings"
	"time"
)

type dateWindow struct {
	FromDate time.Time
	ToDate   time.Time
}

func splitDateWindows(fromDate, toDate time.Time) []dateWindow {
	if toDate.Before(fromDate) {
		return nil
	}
	var windows []dateWindow
	for start := fromDate; !start.After(toDate); {
		end := start.AddDate(0, 0, maxKiteWindowDays-1)
		if end.After(toDate) {
			end = toDate
		}
		windows = append(windows, dateWindow{FromDate: start, ToDate: end})
		start = end.AddDate(0, 0, 1)
	}
	return windows
}

func sessionRange(fromDate, toDate time.Time, loc *time.Location) (time.Time, time.Time) {
	return withClock(fromDate, marketSessionOpen, loc), withClock(toDate, marketSessionClose, loc)
}

func withClock(day time.Time, clock string, loc *time.Location) time.Time {
	parts := strings.Split(clock, ":")
	hour, _ := strconv.Atoi(parts[0])
	minute, _ := strconv.Atoi(parts[1])
	second, _ := strconv.Atoi(parts[2])
	return time.Date(day.Year(), day.Month(), day.Day(), hour, minute, second, 0, loc)
}
