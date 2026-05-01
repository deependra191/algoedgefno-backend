package nse

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

const (
	nseMainURL        = "https://www.nseindia.com"
	eodInterval       = "1d"
	ProviderName      = "nse_eod"
	maxBhavSize       = 50 << 20
	httpClientTimeout = 60 * time.Second
)

// EODProvider fetches NSE F&O end-of-day bhavcopy data and syncs it into the local store.
type EODProvider struct {
	instrumentStore models.InstrumentRepository
	candleStore     models.CandleRepository
	httpClient      *http.Client
	targetDate      time.Time
	userAgent       string
	acceptHTML      string
	// cacheMu protects all cached* fields against concurrent sync calls.
	cacheMu         sync.Mutex
	cachedRows      []bhavRow
	cachedDate      time.Time
	cachedIndexRows []indexRow
	cachedIndexDate time.Time
	cachedCMRows    []cmRow
	cachedCMDate    time.Time
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
	underlyings := make(map[string]bool, len(rows))
	// Track futures underlyings so we can create continuous instruments.
	type futuresInfo struct {
		contType string
		lotSize  int
	}
	futuresUnderlyings := make(map[string]futuresInfo)

	for _, row := range rows {
		inst, ok := bhavRowToInstrument(row)
		if !ok {
			log.Printf("%s: skipping row with unrecognised instrument type %q", ProviderName, row.InstrumentType)
			continue
		}
		instruments = append(instruments, inst)
		if row.Underlying != "" {
			underlyings[row.Underlying] = true
		}
		if contType, ok := continuousFuturesType(inst.InstrumentType); ok {
			if _, exists := futuresUnderlyings[row.Underlying]; !exists && row.Underlying != "" {
				futuresUnderlyings[row.Underlying] = futuresInfo{contType: contType, lotSize: inst.LotSize}
			}
		}
	}

	for underlying, fi := range futuresUnderlyings {
		u := underlying
		symbol := underlying + models.ContinuousFuturesSuffix
		instruments = append(instruments, models.Instrument{
			Symbol:         symbol,
			Name:           symbol,
			Exchange:       models.ExchangeNFO,
			InstrumentType: fi.contType,
			Underlying:     &u,
			LotSize:        fi.lotSize,
		})
	}

	indexRows, err := p.fetchLatestIndicesBhavcopy(ctx)
	if err != nil {
		if !p.targetDate.IsZero() {
			return 0, fmt.Errorf("fetch indices bhavcopy: %w", err)
		}
		log.Printf("%s: indices bhavcopy unavailable, skipping index instruments: %v", ProviderName, err)
	} else {
		for _, row := range indexRows {
			instruments = append(instruments, indexRowToInstrument(row))
		}
	}

	cmRows, err := p.fetchLatestCMBhavcopy(ctx)
	if err != nil {
		if !p.targetDate.IsZero() {
			return 0, fmt.Errorf("fetch CM bhavcopy: %w", err)
		}
		log.Printf("%s: CM bhavcopy unavailable, skipping equity instruments: %v", ProviderName, err)
	} else {
		for _, row := range cmRows {
			if !underlyings[row.Symbol] {
				continue
			}
			instruments = append(instruments, cmRowToInstrument(row))
		}
	}

	if err := p.instrumentStore.UpsertBatch(ctx, instruments); err != nil {
		return 0, fmt.Errorf("upsert instruments: %w", err)
	}
	return len(instruments), nil
}

func (p *EODProvider) SyncCandles(ctx context.Context) (int, error) {
	// Load instruments from both NSE and NFO: NSE for index/equity spot, NFO for F&O contracts.
	allInstr, err := p.instrumentStore.List(ctx, models.InstrumentFilter{})
	if err != nil {
		return 0, fmt.Errorf("list instruments: %w", err)
	}
	instrMap := make(map[instrumentKey]uuid.UUID, len(allInstr))
	underlyings := make(map[string]bool, len(allInstr))
	for _, inst := range allInstr {
		instrMap[instrumentKey{Symbol: inst.Symbol, Exchange: inst.Exchange}] = inst.ID
		if inst.Exchange == models.ExchangeNFO && inst.Underlying != nil {
			underlyings[*inst.Underlying] = true
		}
	}

	rows, err := p.fetchLatestBhavcopy(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch bhavcopy: %w", err)
	}

	candles := make([]models.Candle, 0, len(rows))
	type nearMonthCandidate struct {
		expiry time.Time
		candle models.Candle
	}
	nearMonth := make(map[string]*nearMonthCandidate)

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

		instrType, isFO := foVendorToInstrumentType(row.InstrumentType)
		if !isFO || row.Underlying == "" || row.Expiry.IsZero() {
			continue
		}
		if _, isFut := continuousFuturesType(instrType); !isFut {
			continue
		}
		if row.Expiry.Before(row.Date) {
			continue
		}
		existing, seen := nearMonth[row.Underlying]
		if !seen || row.Expiry.Before(existing.expiry) {
			contSymbol := row.Underlying + models.ContinuousFuturesSuffix
			contID, ok := instrMap[instrumentKey{Symbol: contSymbol, Exchange: models.ExchangeNFO}]
			if !ok {
				continue
			}
			nearMonth[row.Underlying] = &nearMonthCandidate{
				expiry: row.Expiry,
				candle: models.Candle{
					InstrumentID: contID,
					Timestamp:    row.Date,
					Interval:     eodInterval,
					Open:         row.Open,
					High:         row.High,
					Low:          row.Low,
					Close:        row.Close,
					Volume:       row.Volume,
					Provider:     p.Name(),
				},
			}
		}
	}

	for _, nm := range nearMonth {
		candles = append(candles, nm.candle)
	}

	indexRows, err := p.fetchLatestIndicesBhavcopy(ctx)
	if err != nil {
		if !p.targetDate.IsZero() {
			return 0, fmt.Errorf("fetch indices bhavcopy: %w", err)
		}
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

	cmRows, err := p.fetchLatestCMBhavcopy(ctx)
	if err != nil {
		if !p.targetDate.IsZero() {
			return 0, fmt.Errorf("fetch CM bhavcopy: %w", err)
		}
		log.Printf("%s: CM bhavcopy unavailable, skipping equity candles: %v", ProviderName, err)
	} else {
		for _, row := range cmRows {
			if !underlyings[row.Symbol] {
				continue
			}
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
