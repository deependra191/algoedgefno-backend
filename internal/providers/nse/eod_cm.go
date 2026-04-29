package nse

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

const (
	// cmURL is the NSE CM (equity) bhavcopy URL pattern (plain CSV, not zipped).
	// NOTE: Verify column names against the live NSE CM bhavcopy before first production run.
	cmURL = "https://nsearchives.nseindia.com/products/content/sec_bhavdata_full_%s.csv"

	// cmDateFormat is the date layout for the CM bhavcopy URL (DDMMYYYY).
	// Same layout as indicesDateFormat — kept separate so either can change independently.
	cmDateFormat = "02012006"

	equityLotSize = 1

	// NSE CM bhavcopy CSV column names.
	csvColCMSymbol = "SYMBOL"
	csvColCMSeries = "SERIES"
	csvColCMDate   = "DATE1"
	csvColCMOpen   = "OPEN_PRICE"
	csvColCMHigh   = "HIGH_PRICE"
	csvColCMLow    = "LOW_PRICE"
	csvColCMClose  = "CLOSE_PRICE"
	csvColCMVolume = "TTL_TRD_QNTY"

	// cmSeriesEQ is the SERIES value for equity (cash market) instruments in the CM bhavcopy.
	cmSeriesEQ = "EQ"
)

// cmRow holds one parsed row from the NSE CM (equity) bhavcopy CSV.
type cmRow struct {
	Symbol string
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume int64
	Date   time.Time
}

// fetchLatestCMBhavcopy fetches the NSE CM (equity) bhavcopy, trying recent trading days.
// Returns all EQ-series rows; callers filter to their relevant subset.
// Results are cached within a sync cycle.
// On daily runs, a fetch failure is non-fatal — callers log and skip CM data.
func (p *EODProvider) fetchLatestCMBhavcopy(ctx context.Context) ([]cmRow, error) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()

	anchor := latestTradingDate()
	if !p.targetDate.IsZero() {
		anchor = p.targetDate
	}
	if p.cachedCMRows != nil && p.cachedCMDate.Equal(anchor) {
		return p.cachedCMRows, nil
	}

	p.warmSession(ctx)

	if !p.targetDate.IsZero() {
		rows, err := p.downloadCMBhavcopy(ctx, p.targetDate)
		if err != nil {
			return nil, fmt.Errorf("CM bhavcopy for %s: %w", p.targetDate.Format("2006-01-02"), err)
		}
		p.cachedCMRows = rows
		p.cachedCMDate = anchor
		return rows, nil
	}

	date := anchor
	for range 7 {
		rows, err := p.downloadCMBhavcopy(ctx, date)
		if err == nil {
			p.cachedCMRows = rows
			p.cachedCMDate = anchor
			return rows, nil
		}
		log.Printf("%s: CM bhavcopy not available for %s: %v", ProviderName, date.Format("2006-01-02"), err)
		date = prevTradingDay(date)
	}
	return nil, fmt.Errorf("no CM bhavcopy available in the last 7 trading days")
}

// downloadCMBhavcopy fetches the plain-CSV CM bhavcopy for the given date.
func (p *EODProvider) downloadCMBhavcopy(ctx context.Context, date time.Time) ([]cmRow, error) {
	url := fmt.Sprintf(cmURL, date.Format(cmDateFormat))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", p.userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", nseMainURL+"/")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBhavSize))
	if err != nil {
		return nil, err
	}

	return parseCMCSV(bytes.NewReader(body), date)
}

// parseCMCSV parses the NSE CM bhavcopy CSV and returns all EQ-series rows.
// Non-EQ series (BE, N1, etc.) are skipped. Rows with unparseable OHLC values are
// also skipped — partial price data is worse than no data for backtesting.
// NOTE: Verify column names against the live NSE CM bhavcopy before first production run.
func parseCMCSV(r io.Reader, fallbackDate time.Time) ([]cmRow, error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true

	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	idx := make(map[string]int, len(header))
	for i, h := range header {
		idx[strings.TrimSpace(h)] = i
	}

	symbolIdx := col(idx, csvColCMSymbol)
	if symbolIdx < 0 {
		return nil, fmt.Errorf("required column %q not found in CM CSV header: %v", csvColCMSymbol, header)
	}

	seriesIdx := col(idx, csvColCMSeries)
	dateIdx := col(idx, csvColCMDate)
	openIdx := col(idx, csvColCMOpen)
	highIdx := col(idx, csvColCMHigh)
	lowIdx := col(idx, csvColCMLow)
	closeIdx := col(idx, csvColCMClose)
	volIdx := col(idx, csvColCMVolume)

	get := func(rec []string, i int) string {
		if i < 0 || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}

	rows := make([]cmRow, 0)
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		if get(rec, seriesIdx) != cmSeriesEQ {
			continue
		}

		symbol := get(rec, symbolIdx)
		if symbol == "" {
			continue
		}

		open, err := parseFloat(get(rec, openIdx))
		if err != nil {
			continue
		}
		high, err := parseFloat(get(rec, highIdx))
		if err != nil {
			continue
		}
		low, err := parseFloat(get(rec, lowIdx))
		if err != nil {
			continue
		}
		close_, err := parseFloat(get(rec, closeIdx))
		if err != nil {
			continue
		}
		vol, _ := parseInt64(get(rec, volIdx))

		rowDate := fallbackDate
		if ds := get(rec, dateIdx); ds != "" {
			if t, err := parseDate(ds); err == nil {
				rowDate = t
			}
		}

		rows = append(rows, cmRow{
			Symbol: symbol,
			Open:   open,
			High:   high,
			Low:    low,
			Close:  close_,
			Volume: vol,
			Date:   rowDate,
		})
	}
	return rows, nil
}

// cmRowToInstrument maps a parsed CM equity row to a domain Instrument.
func cmRowToInstrument(row cmRow) models.Instrument {
	underlying := row.Symbol
	return models.Instrument{
		Symbol:         row.Symbol,
		Name:           row.Symbol,
		Exchange:       models.ExchangeNSE,
		InstrumentType: models.InstrumentTypeEquity,
		Underlying:     &underlying,
		LotSize:        equityLotSize,
	}
}
