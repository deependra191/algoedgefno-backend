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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/internal/providers"
	"github.com/deependra191/algoedgefno-backend/internal/storage"
)

// bhavURL is the NSE F&O bhavcopy URL pattern.
// NOTE: NSE has changed this URL format historically. Verify before deploying.
// Current format (as of 2024-2025): BhavCopy_NSE_FO_0_0_0_YYYYMMDD_F_0000.csv.zip
const bhavURL = "https://nsearchives.nseindia.com/content/fo/BhavCopy_NSE_FO_0_0_0_%s_F_0000.csv.zip"

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
	Date           time.Time
}

// colMap holds resolved column indices for the CSV.
type colMap struct {
	instrType, symbol, underlying, expiry, strike, optType int
	open, high, low, close_, vol, ts                       int
}

type EODProvider struct {
	instrumentStore *storage.InstrumentStore
	candleStore     *storage.CandleStore
	httpClient      *http.Client
	// Cache to avoid downloading the same bhavcopy twice in one sync cycle.
	// cacheMu protects cachedRows and cachedDate against concurrent sync calls.
	cacheMu    sync.Mutex
	cachedRows []bhavRow
	cachedDate time.Time
}

func NewEODProvider(instrumentStore *storage.InstrumentStore, candleStore *storage.CandleStore) *EODProvider {
	return &EODProvider{
		instrumentStore: instrumentStore,
		candleStore:     candleStore,
		httpClient:      &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *EODProvider) Name() string { return "nse_eod" }

func (p *EODProvider) Capabilities() []providers.Capability {
	return []providers.Capability{providers.CapEODHistory}
}

func (p *EODProvider) Healthy(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, "https://nsearchives.nseindia.com", nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

func (p *EODProvider) SyncInstruments(ctx context.Context) (int, error) {
	rows, err := p.fetchLatestBhavcopy(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch bhavcopy: %w", err)
	}

	instruments := make([]models.Instrument, 0, len(rows))
	for _, row := range rows {
		instruments = append(instruments, bhavRowToInstrument(row))
	}

	if err := p.instrumentStore.UpsertBatch(ctx, instruments); err != nil {
		return 0, fmt.Errorf("upsert instruments: %w", err)
	}
	return len(instruments), nil
}

func (p *EODProvider) SyncCandles(ctx context.Context) (int, error) {
	rows, err := p.fetchLatestBhavcopy(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch bhavcopy: %w", err)
	}

	// Load all NFO instruments into a symbol→ID map for FK lookup.
	exchange := "NFO"
	allInstr, err := p.instrumentStore.List(ctx, storage.InstrumentFilter{Exchange: &exchange})
	if err != nil {
		return 0, fmt.Errorf("list instruments: %w", err)
	}
	instrMap := make(map[string]uuid.UUID, len(allInstr))
	for _, inst := range allInstr {
		instrMap[inst.Symbol] = inst.ID
	}

	candles := make([]models.Candle, 0, len(rows))
	for _, row := range rows {
		id, ok := instrMap[row.Symbol]
		if !ok {
			continue
		}
		candles = append(candles, models.Candle{
			InstrumentID: id,
			Timestamp:    row.Date,
			Interval:     "1d",
			Open:         row.Open,
			High:         row.High,
			Low:          row.Low,
			Close:        row.Close,
			Volume:       row.Volume,
			Provider:     p.Name(),
		})
	}

	if len(candles) == 0 {
		return 0, nil
	}
	count, err := p.candleStore.InsertBatch(ctx, candles)
	if err != nil {
		return 0, fmt.Errorf("insert candles: %w", err)
	}
	return int(count), nil
}

// fetchLatestBhavcopy tries the last few trading days until it finds an available file.
// Results are cached so that SyncInstruments + SyncCandles in the same cycle don't
// download the same file twice.
func (p *EODProvider) fetchLatestBhavcopy(ctx context.Context) ([]bhavRow, error) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()

	today := latestTradingDate()
	if p.cachedRows != nil && p.cachedDate.Equal(today) {
		return p.cachedRows, nil
	}

	date := today
	for i := 0; i < 7; i++ {
		rows, err := p.downloadBhavcopy(ctx, date)
		if err == nil {
			p.cachedRows = rows
			p.cachedDate = today
			return rows, nil
		}
		log.Printf("nse_eod: bhavcopy not available for %s: %v", date.Format("2006-01-02"), err)
		date = prevTradingDay(date)
	}
	return nil, fmt.Errorf("no bhavcopy available in the last 7 trading days")
}

func (p *EODProvider) downloadBhavcopy(ctx context.Context, date time.Time) ([]bhavRow, error) {
	url := fmt.Sprintf(bhavURL, date.Format("20060102"))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// NSE blocks requests without proper headers.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", "https://www.nseindia.com/")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
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
			defer rc.Close()
			return parseBhavCSV(rc, date)
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

	// NOTE: Column names must be verified against a real NSE bhavcopy file
	// before first production sync. TckrSymb is required — it provides the
	// unique per-contract ticker. Without it, instruments cannot be uniquely
	// identified (the old "SYMBOL" column holds only the underlying name).
	symbolIdx := col(idx, "TckrSymb")
	if symbolIdx < 0 {
		return nil, fmt.Errorf("required column TckrSymb not found in CSV header: %v", header)
	}

	cols := colMap{
		instrType:  col(idx, "FinInstrmTp", "INSTRUMENT"),
		symbol:     symbolIdx,
		underlying: col(idx, "UndrlygAsst"),
		expiry:     col(idx, "XpryDt", "EXPIRY_DT"),
		strike:     col(idx, "StrkPric", "STRIKE_PR"),
		optType:    col(idx, "OptnTp", "OPTION_TYP"),
		open:       col(idx, "OpnPric", "OPEN"),
		high:       col(idx, "HghPric", "HIGH"),
		low:        col(idx, "LwPric", "LOW"),
		close_:     col(idx, "ClsPric", "CLOSE"),
		vol:        col(idx, "TtlTradgVol", "CONTRACTS"),
		ts:         col(idx, "TmStmp", "TIMESTAMP"),
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
		Date:           rowDate,
	}, nil
}

func bhavRowToInstrument(row bhavRow) models.Instrument {
	inst := models.Instrument{
		ID:             uuid.New(),
		Symbol:         row.Symbol,
		Name:           row.Symbol,
		Exchange:       "NFO",
		InstrumentType: row.InstrumentType,
		LotSize:        1, // lot sizes require NSE contract specs; default to 1
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

// ist is the Indian Standard Time location (UTC+5:30). NSE operates on IST.
var ist = time.FixedZone("IST", 5*60*60+30*60)

// latestTradingDate returns the most recent weekday (Mon–Fri) in IST.
func latestTradingDate() time.Time {
	t := time.Now().In(ist).Truncate(24 * time.Hour)
	return prevTradingDayOrToday(t)
}

func prevTradingDayOrToday(t time.Time) time.Time {
	switch t.Weekday() {
	case time.Sunday:
		return t.AddDate(0, 0, -2)
	case time.Saturday:
		return t.AddDate(0, 0, -1)
	}
	return t
}

func prevTradingDay(t time.Time) time.Time {
	t = t.AddDate(0, 0, -1)
	return prevTradingDayOrToday(t)
}

// col returns the index of the first matching column name, or -1 if none found.
func col(idx map[string]int, names ...string) int {
	for _, name := range names {
		if i, ok := idx[name]; ok {
			return i
		}
	}
	return -1
}

func parseFloat(s string) (float64, error) {
	if s == "" || s == "-" {
		return 0, fmt.Errorf("empty or missing value")
	}
	return strconv.ParseFloat(s, 64)
}

func parseInt64(s string) (int64, error) {
	if s == "" || s == "-" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

func parseExpiry(s string) time.Time {
	if s == "" || s == "-" {
		return time.Time{}
	}
	// Try common NSE date formats.
	formats := []string{"02-Jan-2006", "2006-01-02", "02-JAN-2006", "01/02/2006"}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func parseDate(s string) (time.Time, error) {
	formats := []string{"02-Jan-2006", "2006-01-02", "20060102", "01/02/2006"}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date: %s", s)
}
