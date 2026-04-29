package nse

import (
	"archive/zip"
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

// bhavURL is the NSE F&O bhavcopy URL pattern.
// NOTE: NSE has changed this URL format historically. Verify before deploying.
// Current format (as of 2024-2025): BhavCopy_NSE_FO_0_0_0_YYYYMMDD_F_0000.csv.zip
const (
	bhavURL = "https://nsearchives.nseindia.com/content/fo/BhavCopy_NSE_FO_0_0_0_%s_F_0000.csv.zip"

	// bhavDateFormat is the date layout used to format the F&O bhavcopy URL.
	// Kept separate from the date formats in parseDate — those parse incoming CSV values,
	// this one is for generating outbound URLs.
	bhavDateFormat = "20060102"

	// NSE F&O bhavcopy CSV column names. NSE uses cryptic abbreviations; constants
	// bridge the gap and make updates easy if NSE changes their column names.
	csvColInstrumentType = "FinInstrmTp"
	csvColInstrumentName = "FinInstrmNm"
	csvColUnderlying     = "TckrSymb"
	csvColExpiry         = "XpryDt"
	csvColStrike         = "StrkPric"
	csvColOptionType     = "OptnTp"
	csvColOpen           = "OpnPric"
	csvColHigh           = "HghPric"
	csvColLow            = "LwPric"
	csvColClose          = "ClsPric"
	csvColVolume         = "TtlTradgVol"
	csvColTimestamp      = "TmStmp"
	csvColLotSize        = "NewBrdLotQty"

	// Fallback column names from the older NSE bhavcopy format.
	csvColInstrumentTypeLegacy = "INSTRUMENT"
	csvColExpiryLegacy         = "EXPIRY_DT"
	csvColStrikeLegacy         = "STRIKE_PR"
	csvColOptionTypeLegacy     = "OPTION_TYP"
	csvColOpenLegacy           = "OPEN"
	csvColHighLegacy           = "HIGH"
	csvColLowLegacy            = "LOW"
	csvColCloseLegacy          = "CLOSE"
	csvColVolumeLegacy         = "CONTRACTS"
	csvColTimestampLegacy      = "TIMESTAMP"
)

// bhavRow holds one parsed row from the F&O bhavcopy CSV.
type bhavRow struct {
	InstrumentType string
	Symbol         string
	Underlying     string
	Expiry         time.Time
	Strike         float64
	OptionType     string
	Open           float64
	High           float64
	Low            float64
	Close          float64
	Volume         int64
	LotSize        int
	Date           time.Time
}

// colMap holds resolved column indices for the CSV.
type colMap struct {
	instrType, symbol, underlying, expiry, strike, optType int
	open, high, low, close_, vol, ts, lotSize              int
}

// fetchLatestBhavcopy tries the last few trading days until it finds an available file.
// Results are cached so that SyncInstruments + SyncCandles in the same cycle don't
// download the same file twice.
func (p *EODProvider) fetchLatestBhavcopy(ctx context.Context) ([]bhavRow, error) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()

	anchor := latestTradingDate()
	if !p.targetDate.IsZero() {
		anchor = p.targetDate
	}
	if p.cachedRows != nil && p.cachedDate.Equal(anchor) {
		return p.cachedRows, nil
	}

	p.warmSession(ctx)

	if !p.targetDate.IsZero() {
		rows, err := p.downloadBhavcopy(ctx, p.targetDate)
		if err != nil {
			return nil, fmt.Errorf("bhavcopy for %s: %w", p.targetDate.Format("2006-01-02"), err)
		}
		p.cachedRows = rows
		p.cachedDate = anchor
		return rows, nil
	}

	date := anchor
	for range 7 {
		rows, err := p.downloadBhavcopy(ctx, date)
		if err == nil {
			p.cachedRows = rows
			p.cachedDate = anchor
			return rows, nil
		}
		log.Printf("%s: bhavcopy not available for %s: %v", ProviderName, date.Format("2006-01-02"), err)
		date = prevTradingDay(date)
	}
	return nil, fmt.Errorf("no bhavcopy available in the last 7 trading days")
}

// downloadBhavcopy fetches and unzips the bhavcopy archive for the given date,
// returning the parsed rows. Returns an error if the file is unavailable (HTTP non-200)
// or the zip contains no CSV — callers use this to step back to the previous trading day.
func (p *EODProvider) downloadBhavcopy(ctx context.Context, date time.Time) ([]bhavRow, error) {
	url := fmt.Sprintf(bhavURL, date.Format(bhavDateFormat))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// NSE blocks requests without proper headers.
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

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("unzip: %w", err)
	}

	for _, f := range zr.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".csv") {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			rows, err := parseBhavCSV(rc, date)
			rc.Close()
			return rows, err
		}
	}
	return nil, fmt.Errorf("no CSV found in zip")
}

// parseBhavCSV parses the bhavcopy CSV.
// Column names are mapped with fallbacks to handle both old and new NSE formats.
// NOTE: Verify column names against the live NSE bhavcopy before first production run.
func parseBhavCSV(r io.Reader, date time.Time) ([]bhavRow, error) {
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

	// FinInstrmNm is the unique per-contract name (e.g. "NIFTY26APR22500CE").
	// TckrSymb holds only the underlying (e.g. "NIFTY"), not unique per contract.
	symbolIdx := col(idx, csvColInstrumentName)
	if symbolIdx < 0 {
		return nil, fmt.Errorf("required column %s not found in CSV header: %v", csvColInstrumentName, header)
	}

	cols := colMap{
		instrType:  col(idx, csvColInstrumentType, csvColInstrumentTypeLegacy),
		symbol:     symbolIdx,
		underlying: col(idx, csvColUnderlying),
		expiry:     col(idx, csvColExpiry, csvColExpiryLegacy),
		strike:     col(idx, csvColStrike, csvColStrikeLegacy),
		optType:    col(idx, csvColOptionType, csvColOptionTypeLegacy),
		open:       col(idx, csvColOpen, csvColOpenLegacy),
		high:       col(idx, csvColHigh, csvColHighLegacy),
		low:        col(idx, csvColLow, csvColLowLegacy),
		close_:     col(idx, csvColClose, csvColCloseLegacy),
		vol:        col(idx, csvColVolume, csvColVolumeLegacy),
		ts:         col(idx, csvColTimestamp, csvColTimestampLegacy),
		lotSize:    col(idx, csvColLotSize),
	}

	var rows []bhavRow
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		row, err := parseRow(rec, cols, date)
		if err != nil {
			continue // skip malformed rows
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// parseRow converts a single CSV record into a bhavRow using pre-resolved column indices.
// fallbackDate is used when the record's timestamp column is absent or unparseable.
// Returns an error only for fatal fields (missing symbol, unparseable OHLC); non-fatal
// fields (volume, lot size, strike) default to zero silently.
func parseRow(rec []string, cols colMap, fallbackDate time.Time) (bhavRow, error) {
	get := func(i int) string {
		if i < 0 || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}

	symbol := get(cols.symbol)
	if symbol == "" {
		return bhavRow{}, fmt.Errorf("empty symbol")
	}

	open, err := parseFloat(get(cols.open))
	if err != nil {
		return bhavRow{}, fmt.Errorf("open: %w", err)
	}
	high, err := parseFloat(get(cols.high))
	if err != nil {
		return bhavRow{}, fmt.Errorf("high: %w", err)
	}
	low, err := parseFloat(get(cols.low))
	if err != nil {
		return bhavRow{}, fmt.Errorf("low: %w", err)
	}
	close_, err := parseFloat(get(cols.close_))
	if err != nil {
		return bhavRow{}, fmt.Errorf("close: %w", err)
	}

	vol, _ := parseInt64(get(cols.vol)) // non-fatal if missing
	lotSize, _ := parseInt64(get(cols.lotSize))

	expiry := parseExpiry(get(cols.expiry))
	strike, _ := parseFloat(get(cols.strike))

	// Use the file's timestamp if available, else the date we fetched for.
	rowDate := fallbackDate
	if ts := get(cols.ts); ts != "" {
		if t, err := parseDate(ts); err == nil {
			rowDate = t
		}
	}

	return bhavRow{
		InstrumentType: get(cols.instrType),
		Symbol:         symbol,
		Underlying:     get(cols.underlying),
		Expiry:         expiry,
		Strike:         strike,
		OptionType:     get(cols.optType),
		Open:           open,
		High:           high,
		Low:            low,
		Close:          close_,
		Volume:         vol,
		LotSize:        int(lotSize),
		Date:           rowDate,
	}, nil
}

// bhavRowToInstrument maps a parsed bhavcopy row to a domain Instrument.
// LotSize defaults to 1 when the CSV value is absent or zero.
func bhavRowToInstrument(row bhavRow) models.Instrument {
	lotSize := row.LotSize
	if lotSize <= 0 {
		lotSize = 1
	}
	inst := models.Instrument{
		Symbol:         row.Symbol,
		Name:           row.Symbol,
		Exchange:       models.ExchangeNFO,
		InstrumentType: row.InstrumentType,
		LotSize:        lotSize,
	}
	if row.Underlying != "" {
		inst.Underlying = &row.Underlying
	}
	if !row.Expiry.IsZero() {
		inst.Expiry = &row.Expiry
	}
	if row.Strike > 0 {
		inst.Strike = &row.Strike
	}
	if row.OptionType != "" && row.OptionType != "-" {
		inst.OptionType = &row.OptionType
	}
	return inst
}
