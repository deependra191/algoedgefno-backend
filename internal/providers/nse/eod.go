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
	indicesURL   = "https://nsearchives.nseindia.com/content/indices/ind_close_all_%s.csv"
	nseMainURL   = "https://www.nseindia.com"
	eodInterval  = "1d"
	ProviderName = "nse_eod"
	maxBhavSize  = 50 << 20

	// bhavDateFormat is the date layout used to format the F&O bhavcopy URL.
	// Kept separate from the date formats in parseDate — those parse incoming CSV values,
	// this one is for generating outbound URLs.
	bhavDateFormat = "20060102"
	// indicesDateFormat is the date layout for the indices bhavcopy URL (DDMMYYYY).
	indicesDateFormat = "02012006"

	httpClientTimeout = 60 * time.Second

	indexLotSize = 1

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

	// NSE indices bhavcopy CSV column names.
	csvColIndexName  = "Index Name"
	csvColIndexDate  = "Index Date"
	csvColIndexOpen  = "Open Index Value"
	csvColIndexHigh  = "High Index Value"
	csvColIndexLow   = "Low Index Value"
	csvColIndexClose = "Closing Index Value"
	csvColIndexVol   = "Volume"
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
	"Nifty 50":          models.UnderlyingNifty,
	"Nifty Bank":        models.UnderlyingBankNifty,
	"Nifty Fin Service": models.UnderlyingFinNifty,
}

// EODProvider fetches NSE F&O end-of-day bhavcopy data and syncs it into the local store.
type EODProvider struct {
	instrumentStore models.InstrumentRepository
	candleStore     models.CandleRepository
	httpClient      *http.Client
	targetDate      time.Time
	userAgent       string
	acceptHTML      string
	// Cache to avoid downloading the same bhavcopy twice in one sync cycle.
	// cacheMu protects all cached* fields against concurrent sync calls.
	cacheMu         sync.Mutex
	cachedRows      []bhavRow
	cachedDate      time.Time
	cachedIndexRows []indexRow
	cachedIndexDate time.Time
}

// EODOption is a functional option for configuring an EODProvider.
type EODOption func(*EODProvider)

// WithTargetDate overrides the default "latest trading day" anchor so the provider
// syncs a specific date. Primarily used in tests and backfill runs.
func WithTargetDate(date time.Time) EODOption {
	return func(p *EODProvider) {
		p.targetDate = date
	}
}

// WithUserAgent overrides the User-Agent header sent to NSE.
// Use when NSE tightens their bot-detection and the default string starts getting blocked.
func WithUserAgent(ua string) EODOption {
	return func(p *EODProvider) { p.userAgent = ua }
}

// WithAcceptHTML overrides the Accept header sent during the session warm-up request.
func WithAcceptHTML(accept string) EODOption {
	return func(p *EODProvider) { p.acceptHTML = accept }
}

// NewEODProvider creates an EODProvider with a cookie-jar-enabled HTTP client.
// The jar is required to pass NSE's session-cookie gate on nsearchives.nseindia.com.
// Use WithUserAgent / WithAcceptHTML to override the default browser-fingerprinting
// headers when NSE tightens bot detection — no recompile needed.
func NewEODProvider(instrumentStore models.InstrumentRepository, candleStore models.CandleRepository, opts ...EODOption) *EODProvider {
	jar, _ := cookiejar.New(nil)
	p := &EODProvider{
		instrumentStore: instrumentStore,
		candleStore:     candleStore,
		httpClient:      &http.Client{Timeout: httpClientTimeout, Jar: jar},
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
	req.Header.Set("User-Agent", p.userAgent)
	req.Header.Set("Accept", p.acceptHTML)
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
	req.Header.Set("User-Agent", p.userAgent)
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

	indexRows, err := p.fetchLatestIndicesBhavcopy(ctx)
	if err != nil {
		log.Printf("%s: indices bhavcopy unavailable, skipping index instruments: %v", ProviderName, err)
	} else {
		for _, row := range indexRows {
			instruments = append(instruments, indexRowToInstrument(row))
		}
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

	allInstr, err := p.instrumentStore.List(ctx, models.InstrumentFilter{})
	if err != nil {
		return 0, fmt.Errorf("list instruments: %w", err)
	}
	instrMap := make(map[instrumentKey]uuid.UUID, len(allInstr))
	for _, inst := range allInstr {
		instrMap[instrumentKey{Symbol: inst.Symbol, Exchange: inst.Exchange}] = inst.ID
	}

	candles := make([]models.Candle, 0, len(rows))
	for _, row := range rows {
		id, ok := instrMap[instrumentKey{Symbol: row.Symbol, Exchange: models.ExchangeNFO}]
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

	indexRows, err := p.fetchLatestIndicesBhavcopy(ctx)
	if err != nil {
		log.Printf("%s: indices bhavcopy unavailable, skipping index candles: %v", ProviderName, err)
	} else {
		for _, row := range indexRows {
			id, ok := instrMap[instrumentKey{Symbol: row.Symbol, Exchange: models.ExchangeNSE}]
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

// ist is the Indian Standard Time location (UTC+5:30). NSE operates on IST.
var ist = time.FixedZone("IST", 5*60*60+30*60)

// latestTradingDate returns the most recent weekday (Mon–Fri) in IST.
func latestTradingDate() time.Time {
	t := time.Now().In(ist).Truncate(24 * time.Hour)
	return prevTradingDayOrToday(t)
}

// prevTradingDayOrToday returns t itself unless it falls on a weekend,
// in which case it rolls back to the preceding Friday.
func prevTradingDayOrToday(t time.Time) time.Time {
	switch t.Weekday() {
	case time.Sunday:
		return t.AddDate(0, 0, -2)
	case time.Saturday:
		return t.AddDate(0, 0, -1)
	}
	return t
}

// prevTradingDay returns the trading day immediately before t, skipping weekends.
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

// parseExpiry parses a contract expiry date string, trying multiple NSE-observed formats.
// Returns a zero time.Time for empty, missing, or unrecognised values — callers treat zero as "no expiry".
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

// parseDate parses a date string from a bhavcopy timestamp column,
// trying multiple NSE-observed formats. Returns an error if none match.
func parseDate(s string) (time.Time, error) {
	formats := []string{"02-Jan-2006", "2006-01-02", "20060102", "01/02/2006", "02-01-2006"}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date: %s", s)
}

// instrumentKey is used to build a (symbol, exchange) lookup map for candle FK resolution.
type instrumentKey struct {
	Symbol   string
	Exchange string
}

// fetchLatestIndicesBhavcopy fetches the NSE indices bhavcopy, trying recent trading days.
// Only rows matching nseIndexNameMap are returned. Results are cached within a sync cycle.
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
// the indices listed in nseIndexNameMap. Other indices are skipped.
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

	var rows []indexRow
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
