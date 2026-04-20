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
	"net/http/cookiejar"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// bhavURL is the NSE F&O bhavcopy URL pattern.
// NOTE: NSE has changed this URL format historically. Verify before deploying.
// Current format (as of 2024-2025): BhavCopy_NSE_FO_0_0_0_YYYYMMDD_F_0000.csv.zip
const (
	bhavURL      = "https://nsearchives.nseindia.com/content/fo/BhavCopy_NSE_FO_0_0_0_%s_F_0000.csv.zip"
	nseMainURL   = "https://www.nseindia.com"
	nseExchange  = "NFO"
	eodInterval  = "1d"
	ProviderName = "nse_eod"
	maxBhavSize  = 50 << 20
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

type EODProvider struct {
	instrumentStore models.InstrumentRepository
	candleStore     models.CandleRepository
	httpClient      *http.Client
	targetDate      time.Time
	// Cache to avoid downloading the same bhavcopy twice in one sync cycle.
	// cacheMu protects cachedRows and cachedDate against concurrent sync calls.
	cacheMu    sync.Mutex
	cachedRows []bhavRow
	cachedDate time.Time
}

type EODOption func(*EODProvider)

func WithTargetDate(date time.Time) EODOption {
	return func(p *EODProvider) {
		p.targetDate = date
	}
}

func NewEODProvider(instrumentStore models.InstrumentRepository, candleStore models.CandleRepository, opts ...EODOption) *EODProvider {
	jar, _ := cookiejar.New(nil)
	p := &EODProvider{
		instrumentStore: instrumentStore,
		candleStore:     candleStore,
		httpClient:      &http.Client{Timeout: 60 * time.Second, Jar: jar},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// warmSession visits the NSE homepage to acquire the session cookies that
// nsearchives.nseindia.com requires before serving archive files.
func (p *EODProvider) warmSession(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, nseMainURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		log.Printf("%s: session warm-up failed: %v", ProviderName, err)
		return
	}
	resp.Body.Close()
}

func (p *EODProvider) Name() string { return ProviderName }

func (p *EODProvider) Capabilities() []models.Capability {
	return []models.Capability{models.CapEODHistory}
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
	exchange := nseExchange
	allInstr, err := p.instrumentStore.List(ctx, models.InstrumentFilter{Exchange: &exchange})
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
			Interval:     eodInterval,
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
	count, err := p.candleStore.InsertBatchIgnoreDuplicates(ctx, candles)
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
	symbolIdx := col(idx, "FinInstrmNm")
	if symbolIdx < 0 {
		return nil, fmt.Errorf("required column FinInstrmNm not found in CSV header: %v", header)
	}

	cols := colMap{
		instrType:  col(idx, "FinInstrmTp", "INSTRUMENT"),
		symbol:     symbolIdx,
		underlying: col(idx, "TckrSymb"),
		expiry:     col(idx, "XpryDt", "EXPIRY_DT"),
		strike:     col(idx, "StrkPric", "STRIKE_PR"),
		optType:    col(idx, "OptnTp", "OPTION_TYP"),
		open:       col(idx, "OpnPric", "OPEN"),
		high:       col(idx, "HghPric", "HIGH"),
		low:        col(idx, "LwPric", "LOW"),
		close_:     col(idx, "ClsPric", "CLOSE"),
		vol:        col(idx, "TtlTradgVol", "CONTRACTS"),
		ts:         col(idx, "TmStmp", "TIMESTAMP"),
		lotSize:    col(idx, "NewBrdLotQty"),
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

func bhavRowToInstrument(row bhavRow) models.Instrument {
	lotSize := row.LotSize
	if lotSize <= 0 {
		lotSize = 1
	}
	inst := models.Instrument{
		Symbol:         row.Symbol,
		Name:           row.Symbol,
		Exchange:       nseExchange,
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
