package main

import "time"

const (
	providerZerodhaKite = "zerodha_kite"

	dateLayout          = "2006-01-02"
	dateTimeLayout      = "2006-01-02 15:04:05"
	istLocationName     = "Asia/Kolkata"
	marketSessionOpen   = "09:15:00"
	marketSessionClose  = "15:30:00"
	maxKiteWindowDays   = 60
	defaultFromDate     = "2025-01-01"
	defaultSymbolsCSV   = "NIFTY 50,NIFTY BANK,RELIANCE,HDFCBANK,INFY,TCS,ICICIBANK"
	defaultDelay        = 350 * time.Millisecond
	defaultTimeout      = 45 * time.Second
	defaultSnapshotDir  = "/private/tmp/kite-backfill-instruments"
	replacementRootPath = "/private/tmp/kite-backfill-replacements"
	auditFileName       = "audit.jsonl"
	csvFileExtension    = ".csv"
	jsonlFileExtension  = ".jsonl"
)

const (
	exitOK    = 0
	exitError = 1
)

const (
	localHostName     = "localhost"
	localIPv4Loopback = "127.0.0.1"
	localIPv6Loopback = "::1"
	pgCodeNoSuchTable = "42P01"
)

const (
	unsafeMarkerProduction = "production"
	unsafeMarkerProd       = "prod"
	unsafeMarkerStaging    = "staging"
	unsafeMarkerStage      = "stage"
	unsafeMarkerVPS        = "vps"
)

const (
	jsonFieldRunID          = "run_id"
	jsonFieldProvider       = "provider"
	jsonFieldSymbol         = "symbol"
	jsonFieldExchange       = "exchange"
	jsonFieldInstrumentID   = "instrument_id"
	jsonFieldInterval       = "interval"
	jsonFieldFrom           = "from"
	jsonFieldTo             = "to"
	jsonFieldReason         = "reason"
	jsonFieldRowsDeleted    = "rows_deleted"
	jsonFieldRowsInserted   = "rows_inserted"
	jsonFieldBackupPath     = "backup_path"
	jsonFieldTimestamp      = "timestamp"
	jsonFieldOpen           = "open"
	jsonFieldHigh           = "high"
	jsonFieldLow            = "low"
	jsonFieldClose          = "close"
	jsonFieldVolume         = "volume"
	jsonFieldCandleProvider = "candle_provider"
)
