package kite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Credentials contains the env-sourced Kite credentials used by local R&D tools.
type Credentials struct {
	APIKey      string
	APISecret   string
	AccessToken string
}

// LoadReadOnlyCredentials loads the credentials needed for authenticated read-only Kite API calls.
func LoadReadOnlyCredentials() (Credentials, error) {
	creds := Credentials{
		APIKey:      strings.TrimSpace(os.Getenv(EnvAPIKey)),
		APISecret:   strings.TrimSpace(os.Getenv(EnvAPISecret)),
		AccessToken: strings.TrimSpace(os.Getenv(EnvAccessToken)),
	}
	if creds.APIKey == "" {
		return Credentials{}, fmt.Errorf("%s is required", EnvAPIKey)
	}
	if creds.AccessToken == "" {
		return Credentials{}, fmt.Errorf("%s is required", EnvAccessToken)
	}
	return creds, nil
}

// LoadLoginCredentials loads the credentials needed for interactive token exchange.
func LoadLoginCredentials() (Credentials, error) {
	creds := Credentials{
		APIKey:    strings.TrimSpace(os.Getenv(EnvAPIKey)),
		APISecret: strings.TrimSpace(os.Getenv(EnvAPISecret)),
	}
	if creds.APIKey == "" {
		return Credentials{}, fmt.Errorf("%s is required", EnvAPIKey)
	}
	if creds.APISecret == "" {
		return Credentials{}, fmt.Errorf("%s is required", EnvAPISecret)
	}
	return creds, nil
}

// LoginURL returns the interactive Kite login URL for the configured API key.
func LoginURL(apiKey string) string {
	u, _ := url.Parse(loginBaseURL)
	q := u.Query()
	q.Set(QueryParamVersion, KiteLoginVersion)
	q.Set(QueryParamAPIKey, apiKey)
	u.RawQuery = q.Encode()
	return u.String()
}

// AccessTokenChecksum returns Kite's SHA-256 checksum for request-token exchange.
func AccessTokenChecksum(apiKey, requestToken, apiSecret string) string {
	sum := sha256.Sum256([]byte(apiKey + requestToken + apiSecret))
	return hex.EncodeToString(sum[:])
}

// ExchangeRequestToken exchanges an interactive request token for an access token.
func ExchangeRequestToken(ctx context.Context, creds Credentials, requestToken string) (string, error) {
	requestToken = strings.TrimSpace(requestToken)
	if requestToken == "" {
		return "", errors.New("request token is required")
	}
	checksum := AccessTokenChecksum(creds.APIKey, requestToken, creds.APISecret)
	form := url.Values{}
	form.Set(QueryParamAPIKey, creds.APIKey)
	form.Set(QueryParamRequestToken, requestToken)
	form.Set(QueryParamChecksum, checksum)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, defaultAPIBaseURL+endpointSession, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set(headerKiteVersion, kiteVersion)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := (&http.Client{Timeout: defaultHTTPTimeout}).Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read token response: %w", err)
	}

	var decoded tokenResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || decoded.Status != "success" {
		return "", fmt.Errorf("token exchange failed: HTTP %d status=%s error_type=%s message=%s",
			resp.StatusCode, decoded.Status, decoded.ErrorType, decoded.Message)
	}
	if decoded.Data.AccessToken == "" {
		return "", errors.New("token response did not contain an access token")
	}
	return decoded.Data.AccessToken, nil
}

// Client is a minimal read-only Kite Connect HTTP client.
type Client struct {
	apiKey      string
	accessToken string
	baseURL     string
	httpClient  *http.Client
}

// NewClient creates a Kite client using the given credentials.
func NewClient(creds Credentials) *Client {
	return &Client{
		apiKey:      creds.APIKey,
		accessToken: creds.AccessToken,
		baseURL:     defaultAPIBaseURL,
		httpClient:  &http.Client{Timeout: defaultHTTPTimeout},
	}
}

// HistoricalRequest describes one Kite historical candle request.
type HistoricalRequest struct {
	InstrumentToken string
	Interval        string
	From            time.Time
	To              time.Time
	Continuous      bool
	OI              bool
}

// HistoricalResult keeps the raw HTTP shape plus parsed candles for probe output.
type HistoricalResult struct {
	HTTPStatus int
	APIStatus  string
	ErrorType  string
	Message    string
	RawPreview string
	Candles    []Candle
}

// Candle is one Kite historical candle row.
type Candle struct {
	RawTimestamp string
	Timestamp    time.Time
	Open         float64
	High         float64
	Low          float64
	Close        float64
	Volume       int64
	OI           *int64
}

// FetchHistorical retrieves historical candles without mutating any local state.
func (c *Client) FetchHistorical(ctx context.Context, r HistoricalRequest) (*HistoricalResult, error) {
	if r.InstrumentToken == "" {
		return nil, errors.New("instrument token is required")
	}
	if r.Interval == "" {
		return nil, errors.New("interval is required")
	}
	if r.From.IsZero() || r.To.IsZero() {
		return nil, errors.New("from and to are required")
	}

	endpoint := c.baseURL + endpointHistorical + url.PathEscape(r.InstrumentToken) + "/" + url.PathEscape(r.Interval)
	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse historical URL: %w", err)
	}
	q := reqURL.Query()
	q.Set(QueryParamFrom, r.From.Format(KiteHistoricalTimestamp))
	q.Set(QueryParamTo, r.To.Format(KiteHistoricalTimestamp))
	q.Set(QueryParamContinuous, boolQueryValue(r.Continuous))
	q.Set(QueryParamOI, boolQueryValue(r.OI))
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build historical request: %w", err)
	}
	c.setAuthHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("historical request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read historical response: %w", err)
	}

	result := &HistoricalResult{
		HTTPStatus: resp.StatusCode,
		RawPreview: previewBody(body),
	}

	var decoded historicalResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		result.Message = "response was not valid Kite historical JSON"
		return result, nil
	}
	result.APIStatus = decoded.Status
	result.ErrorType = decoded.ErrorType
	result.Message = decoded.Message
	result.Candles = decoded.Data.Candles
	return result, nil
}

// Instrument is one row from Kite's instruments CSV dump.
type Instrument struct {
	InstrumentToken string
	ExchangeToken   string
	TradingSymbol   string
	Name            string
	LastPrice       string
	Expiry          string
	Strike          string
	TickSize        string
	LotSize         string
	InstrumentType  string
	Segment         string
	Exchange        string
}

// Instruments retrieves the current Kite instruments CSV dump.
func (c *Client) Instruments(ctx context.Context) ([]Instrument, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+endpointInstruments, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build instruments request: %w", err)
	}
	c.setAuthHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("instruments request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read instruments response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, previewBody(body), fmt.Errorf("instruments request returned HTTP %d", resp.StatusCode)
	}

	rows, err := csv.NewReader(bytes.NewReader(body)).ReadAll()
	if err != nil {
		return nil, previewBody(body), fmt.Errorf("parse instruments CSV: %w", err)
	}
	if len(rows) == 0 {
		return nil, previewBody(body), errors.New("instruments CSV was empty")
	}

	header := mapCSVHeader(rows[0])
	instruments := make([]Instrument, 0, len(rows)-1)
	for _, row := range rows[1:] {
		instruments = append(instruments, Instrument{
			InstrumentToken: csvValue(header, row, CSVColumnInstrumentToken),
			ExchangeToken:   csvValue(header, row, CSVColumnExchangeToken),
			TradingSymbol:   csvValue(header, row, CSVColumnTradingSymbol),
			Name:            csvValue(header, row, CSVColumnName),
			LastPrice:       csvValue(header, row, CSVColumnLastPrice),
			Expiry:          csvValue(header, row, CSVColumnExpiry),
			Strike:          csvValue(header, row, CSVColumnStrike),
			TickSize:        csvValue(header, row, CSVColumnTickSize),
			LotSize:         csvValue(header, row, CSVColumnLotSize),
			InstrumentType:  csvValue(header, row, CSVColumnInstrumentType),
			Segment:         csvValue(header, row, CSVColumnSegment),
			Exchange:        csvValue(header, row, CSVColumnExchange),
		})
	}
	return instruments, previewBody(body), nil
}

// FindByExchangeSymbol returns the current instrument row for an exchange/tradingsymbol pair.
func FindByExchangeSymbol(instruments []Instrument, exchange, symbol string) (Instrument, bool) {
	for _, inst := range instruments {
		if strings.EqualFold(inst.Exchange, exchange) && strings.EqualFold(inst.TradingSymbol, symbol) {
			return inst, true
		}
	}
	return Instrument{}, false
}

// FindNearestFuture returns the earliest live futures contract for the underlying.
func FindNearestFuture(instruments []Instrument, underlying string, asOf time.Time) (Instrument, bool) {
	underlying = strings.ToUpper(strings.TrimSpace(underlying))
	candidates := make([]Instrument, 0)
	for _, inst := range instruments {
		if !strings.EqualFold(inst.Exchange, ExchangeNFO) {
			continue
		}
		if !strings.EqualFold(inst.InstrumentType, InstrumentTypeFuture) {
			continue
		}
		if inst.Segment != "" && !strings.EqualFold(inst.Segment, SegmentNFOFutures) {
			continue
		}
		if !strings.EqualFold(inst.Name, underlying) && !strings.HasPrefix(strings.ToUpper(inst.TradingSymbol), underlying) {
			continue
		}
		expiry, ok := ParseKiteDate(inst.Expiry)
		if !ok || expiry.Before(asOf) {
			continue
		}
		candidates = append(candidates, inst)
	}
	sort.Slice(candidates, func(i, j int) bool {
		left, _ := ParseKiteDate(candidates[i].Expiry)
		right, _ := ParseKiteDate(candidates[j].Expiry)
		return left.Before(right)
	})
	if len(candidates) == 0 {
		return Instrument{}, false
	}
	return candidates[0], true
}

// ParseKiteDate parses Kite's yyyy-mm-dd date fields.
func ParseKiteDate(value string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	return t, err == nil
}

func (c *Client) setAuthHeaders(req *http.Request) {
	req.Header.Set(headerKiteVersion, kiteVersion)
	req.Header.Set(headerAuthorization, authSchemeToken+" "+c.apiKey+":"+c.accessToken)
}

func boolQueryValue(enabled bool) string {
	if enabled {
		return QueryValueEnabled
	}
	return QueryValueDisabled
}

func previewBody(body []byte) string {
	if len(body) > maxResponsePreview {
		return string(body[:maxResponsePreview]) + "\n... <truncated>"
	}
	return string(body)
}

func mapCSVHeader(header []string) map[string]int {
	mapped := make(map[string]int, len(header))
	for i, col := range header {
		mapped[strings.TrimSpace(col)] = i
	}
	return mapped
}

func csvValue(header map[string]int, row []string, column string) string {
	i, ok := header[column]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

type historicalResponse struct {
	Status    string         `json:"status"`
	Data      historicalData `json:"data"`
	Message   string         `json:"message"`
	ErrorType string         `json:"error_type"`
}

type historicalData struct {
	Candles []Candle `json:"candles"`
}

type tokenResponse struct {
	Status    string    `json:"status"`
	Data      tokenData `json:"data"`
	Message   string    `json:"message"`
	ErrorType string    `json:"error_type"`
}

type tokenData struct {
	AccessToken string `json:"access_token"`
}

func (c *Candle) UnmarshalJSON(data []byte) error {
	var row []json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil {
		return err
	}
	if len(row) < 6 {
		return fmt.Errorf("historical candle row has %d fields, want at least 6", len(row))
	}

	if err := json.Unmarshal(row[0], &c.RawTimestamp); err != nil {
		return fmt.Errorf("parse candle timestamp: %w", err)
	}
	parsed, parseErr := parseKiteTimestamp(c.RawTimestamp)
	if parseErr != nil {
		return parseErr
	}
	c.Timestamp = parsed

	var err error
	if c.Open, err = rawFloat(row[1]); err != nil {
		return fmt.Errorf("parse open: %w", err)
	}
	if c.High, err = rawFloat(row[2]); err != nil {
		return fmt.Errorf("parse high: %w", err)
	}
	if c.Low, err = rawFloat(row[3]); err != nil {
		return fmt.Errorf("parse low: %w", err)
	}
	if c.Close, err = rawFloat(row[4]); err != nil {
		return fmt.Errorf("parse close: %w", err)
	}
	if c.Volume, err = rawInt64(row[5]); err != nil {
		return fmt.Errorf("parse volume: %w", err)
	}
	if len(row) > 6 {
		oi, err := rawInt64(row[6])
		if err != nil {
			return fmt.Errorf("parse oi: %w", err)
		}
		c.OI = &oi
	}
	return nil
}

func parseKiteTimestamp(value string) (time.Time, error) {
	layouts := []string{
		"2006-01-02T15:04:05-0700",
		time.RFC3339,
		KiteHistoricalTimestamp,
	}
	var lastErr error
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
		lastErr = err
	}
	return time.Time{}, fmt.Errorf("parse Kite timestamp %q: %w", value, lastErr)
}

func rawFloat(raw json.RawMessage) (float64, error) {
	var n float64
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, err
	}
	return n, nil
}

func rawInt64(raw json.RawMessage) (int64, error) {
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, nil
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0, err
	}
	if f != float64(int64(f)) {
		return 0, fmt.Errorf("value %s is not an integer", strconv.FormatFloat(f, 'f', -1, 64))
	}
	return int64(f), nil
}
