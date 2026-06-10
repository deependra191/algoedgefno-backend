package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

type replacementAudit struct {
	RunID        uuid.UUID
	Target       importTarget
	From         time.Time
	To           time.Time
	Reason       string
	RowsDeleted  int64
	RowsInserted int64
	BackupPath   string
}

func replacementRunDir(root string, runID uuid.UUID) string {
	return filepath.Join(root, runID.String())
}

func writeCandleBackup(runDir string, target importTarget, candles []models.Candle, from, to time.Time) (string, error) {
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return "", fmt.Errorf("create replacement dir: %w", err)
	}
	fileName := sanitizedFilePart(target.ModelInstrument.Exchange) + "_" +
		sanitizedFilePart(target.ModelInstrument.Symbol) + "_" +
		from.Format(dateLayout) + "_" +
		to.Format(dateLayout) + jsonlFileExtension
	path := filepath.Join(runDir, fileName)

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, ownerReadWriteMode)
	if err != nil {
		return "", fmt.Errorf("open backup file: %w", err)
	}
	defer file.Close()
	if err := file.Chmod(ownerReadWriteMode); err != nil {
		return "", fmt.Errorf("chmod backup file: %w", err)
	}

	writer := bufio.NewWriter(file)
	for _, candle := range candles {
		row := map[string]any{
			jsonFieldInstrumentID:   candle.InstrumentID.String(),
			jsonFieldTimestamp:      candle.Timestamp.UTC().Format(time.RFC3339),
			jsonFieldInterval:       candle.Interval,
			jsonFieldOpen:           candle.Open,
			jsonFieldHigh:           candle.High,
			jsonFieldLow:            candle.Low,
			jsonFieldClose:          candle.Close,
			jsonFieldVolume:         candle.Volume,
			jsonFieldCandleProvider: candle.Provider,
		}
		encoded, err := json.Marshal(row)
		if err != nil {
			return "", fmt.Errorf("marshal backup row: %w", err)
		}
		if _, err := writer.Write(append(encoded, '\n')); err != nil {
			return "", fmt.Errorf("write backup row: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return "", fmt.Errorf("flush backup file: %w", err)
	}
	return path, nil
}

func appendReplacementAudit(runDir string, audit replacementAudit) error {
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return fmt.Errorf("create replacement dir: %w", err)
	}
	path := filepath.Join(runDir, auditFileName)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, ownerReadWriteMode)
	if err != nil {
		return fmt.Errorf("open audit file: %w", err)
	}
	defer file.Close()
	if err := file.Chmod(ownerReadWriteMode); err != nil {
		return fmt.Errorf("chmod audit file: %w", err)
	}

	record := map[string]any{
		jsonFieldRunID:        audit.RunID.String(),
		jsonFieldProvider:     providerZerodhaKite,
		jsonFieldSymbol:       audit.Target.ModelInstrument.Symbol,
		jsonFieldExchange:     audit.Target.ModelInstrument.Exchange,
		jsonFieldInstrumentID: audit.Target.ModelInstrument.ID.String(),
		jsonFieldInterval:     models.CandleInterval1M,
		jsonFieldFrom:         audit.From.Format(dateLayout),
		jsonFieldTo:           audit.To.Format(dateLayout),
		jsonFieldReason:       audit.Reason,
		jsonFieldRowsDeleted:  audit.RowsDeleted,
		jsonFieldRowsInserted: audit.RowsInserted,
		jsonFieldBackupPath:   audit.BackupPath,
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal audit record: %w", err)
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("write audit record: %w", err)
	}
	return nil
}
