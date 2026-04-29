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
	indicesURL = "https://nsearchives.nseindia.com/content/indices/ind_close_all_%s.csv"

	// indicesDateFormat is the date layout for the indices bhavcopy URL (DDMMYYYY).
	indicesDateFormat = "02012006"

	indexLotSize = 1

	// NSE indices bhavcopy CSV column names.
	// NOTE: Verify column names against the live NSE indices bhavcopy before first production run.
	csvColIndexName  = "Index Name"
	csvColIndexDate  = "Index Date"
	csvColIndexOpen  = "Open Index Value"
	csvColIndexHigh  = "High Index Value"
	csvColIndexLow   = "Low Index Value"
	csvColIndexClose = "Closing Index Value"
	csvColIndexVol   = "Volume"

	// "Index Name" column values for the indices this provider syncs.
	// Kept as constants — NSE has renamed indices before; updates are a one-line fix here.
	nseIndexNameNifty50     = "Nifty 50"
	nseIndexNameNiftyBank   = "Nifty Bank"
	nseIndexNameFinServices = "Nifty Financial Services"
)

// indexRow holds one parsed row from the NSE indices bhavcopy CSV.
type indexRow struct {
	Symbol string
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume int64
	Date   time.Time
}

// nseIndexNameMap maps the "Index Name" values in the NSE indices bhavcopy to our
// underlying constants. Only indices we actively use for backtesting are included.
// NSE publishes ~100 indices; we filter to these three.
var nseIndexNameMap = map[string]string{
	nseIndexNameNifty50:     models.UnderlyingNifty,
	nseIndexNameNiftyBank:   models.UnderlyingBankNifty,
	nseIndexNameFinServices: models.UnderlyingFinNifty,
}

// fetchLatestIndicesBhavcopy fetches the NSE indices bhavcopy, trying recent trading days.
// Only rows matching nseIndexNameMap are returned. Results are cached within a sync cycle.
// On daily runs, a fetch failure is non-fatal — callers log and skip index data.
func (p *EODProvider) fetchLatestIndicesBhavcopy(ctx context.Context) ([]indexRow, error) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()

	anchor := latestTradingDate()
	if !p.targetDate.IsZero() {
		anchor = p.targetDate
	}
	if p.cachedIndexRows != nil && p.cachedIndexDate.Equal(anchor) {
		return p.cachedIndexRows, nil
	}

	// Warm the session in case this is called without a prior fetchLatestBhavcopy
	// (which also warms), so the NSE cookie jar has valid session cookies.
	p.warmSession(ctx)

	if !p.targetDate.IsZero() {
		rows, err := p.downloadIndicesBhavcopy(ctx, p.targetDate)
		if err != nil {
			return nil, fmt.Errorf("indices bhavcopy for %s: %w", p.targetDate.Format("2006-01-02"), err)
		}
		p.cachedIndexRows = rows
		p.cachedIndexDate = anchor
		return rows, nil
	}

	date := anchor
	for range 7 {
		rows, err := p.downloadIndicesBhavcopy(ctx, date)
		if err == nil {
			p.cachedIndexRows = rows
			p.cachedIndexDate = anchor
			return rows, nil
		}
		log.Printf("%s: indices bhavcopy not available for %s: %v", ProviderName, date.Format("2006-01-02"), err)
		date = prevTradingDay(date)
	}
	return nil, fmt.Errorf("no indices bhavcopy available in the last 7 trading days")
}

// downloadIndicesBhavcopy fetches the plain-CSV indices bhavcopy for the given date.
// Unlike the F&O bhavcopy, the indices file is not zipped.
func (p *EODProvider) downloadIndicesBhavcopy(ctx context.Context, date time.Time) ([]indexRow, error) {
	url := fmt.Sprintf(indicesURL, date.Format(indicesDateFormat))

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

	return parseIndicesCSV(bytes.NewReader(body), date)
}

// parseIndicesCSV parses the NSE indices bhavcopy CSV and returns rows for
// the indices listed in nseIndexNameMap. Rows for other indices are skipped silently.
// Rows with unparseable OHLC values are also skipped — partial price data is worse
// than no data for backtesting.
func parseIndicesCSV(r io.Reader, fallbackDate time.Time) ([]indexRow, error) {
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

	nameIdx := col(idx, csvColIndexName)
	if nameIdx < 0 {
		return nil, fmt.Errorf("required column %q not found in indices CSV header: %v", csvColIndexName, header)
	}

	dateIdx := col(idx, csvColIndexDate)
	openIdx := col(idx, csvColIndexOpen)
	highIdx := col(idx, csvColIndexHigh)
	lowIdx := col(idx, csvColIndexLow)
	closeIdx := col(idx, csvColIndexClose)
	volIdx := col(idx, csvColIndexVol)

	get := func(rec []string, i int) string {
		if i < 0 || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}

	rows := make([]indexRow, 0)
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		indexName := get(rec, nameIdx)
		symbol, ok := nseIndexNameMap[indexName]
		if !ok {
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

		rows = append(rows, indexRow{
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

// indexRowToInstrument maps a parsed index row to a domain Instrument.
func indexRowToInstrument(row indexRow) models.Instrument {
	underlying := row.Symbol
	return models.Instrument{
		Symbol:         row.Symbol,
		Name:           row.Symbol,
		Exchange:       models.ExchangeNSE,
		InstrumentType: models.InstrumentTypeIndex,
		Underlying:     &underlying,
		LotSize:        indexLotSize,
	}
}
