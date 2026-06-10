package kite

import "time"

const (
	defaultAPIBaseURL = "https://api.kite.trade"
	loginBaseURL      = "https://kite.zerodha.com/connect/login"

	defaultHTTPTimeout = 30 * time.Second
	maxResponsePreview = 4096
)

const (
	EnvAPIKey      = "KITE_API_KEY"
	EnvAPISecret   = "KITE_API_SECRET"
	EnvAccessToken = "KITE_ACCESS_TOKEN"
)

const (
	headerAuthorization = "Authorization"
	headerKiteVersion   = "X-Kite-Version"
	kiteVersion         = "3"
)

const (
	authSchemeToken = "token"
)

const (
	endpointHistorical  = "/instruments/historical/"
	endpointInstruments = "/instruments"
	endpointSession     = "/session/token"
)

const (
	QueryParamAPIKey        = "api_key"
	QueryParamRequestToken  = "request_token"
	QueryParamChecksum      = "checksum"
	QueryParamVersion       = "v"
	QueryParamFrom          = "from"
	QueryParamTo            = "to"
	QueryParamContinuous    = "continuous"
	QueryParamOI            = "oi"
	QueryValueEnabled       = "1"
	QueryValueDisabled      = "0"
	KiteHistoricalTimestamp = "2006-01-02 15:04:05"
	KiteLoginVersion        = "3"
)

const (
	IntervalMinute = "minute"
	IntervalDay    = "day"
)

const (
	APIStatusSuccess = "success"
)

const (
	JSONFieldStatus      = "status"
	JSONFieldData        = "data"
	JSONFieldCandles     = "candles"
	JSONFieldMessage     = "message"
	JSONFieldErrorType   = "error_type"
	JSONFieldAccessToken = "access_token"
)

const (
	CSVColumnInstrumentToken = "instrument_token"
	CSVColumnExchangeToken   = "exchange_token"
	CSVColumnTradingSymbol   = "tradingsymbol"
	CSVColumnName            = "name"
	CSVColumnLastPrice       = "last_price"
	CSVColumnExpiry          = "expiry"
	CSVColumnStrike          = "strike"
	CSVColumnTickSize        = "tick_size"
	CSVColumnLotSize         = "lot_size"
	CSVColumnInstrumentType  = "instrument_type"
	CSVColumnSegment         = "segment"
	CSVColumnExchange        = "exchange"
)

const (
	ExchangeNSE = "NSE"
	ExchangeNFO = "NFO"

	InstrumentTypeFuture = "FUT"
	InstrumentTypeIndex  = "INDEX"
	InstrumentTypeEquity = "EQ"

	SegmentNFOFutures = "NFO-FUT"
)
