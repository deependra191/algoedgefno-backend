package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ownerReadWriteMode = 0o600

func writeInstrumentSnapshot(rawCSV []byte, dir string, now time.Time) (string, error) {
	if len(rawCSV) == 0 {
		return "", fmt.Errorf("Kite instruments CSV snapshot is empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create snapshot dir: %w", err)
	}
	path := filepath.Join(dir, "kite-instruments-"+now.UTC().Format("20060102T150405Z")+csvFileExtension)
	if err := os.WriteFile(path, rawCSV, ownerReadWriteMode); err != nil {
		return "", fmt.Errorf("write instruments snapshot: %w", err)
	}
	return path, nil
}

func sanitizedFilePart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	return b.String()
}
