package nse

import (
	"strings"
	"testing"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

func TestPrevTradingDay(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want time.Weekday
	}{
		{"from monday", date(2026, 4, 13), time.Friday},
		{"from tuesday", date(2026, 4, 14), time.Monday},
		{"from saturday", date(2026, 4, 11), time.Friday},
		{"from sunday", date(2026, 4, 12), time.Friday},
		{"from friday", date(2026, 4, 10), time.Thursday},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prevTradingDay(tt.in)
			if got.Weekday() != tt.want {
				t.Errorf("prevTradingDay(%s) weekday = %s, want %s", tt.in.Format("Mon"), got.Weekday(), tt.want)
			}
		})
	}
}

func TestPrevTradingDayOrToday(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want time.Weekday
	}{
		{"weekday stays", date(2026, 4, 15), time.Wednesday},
		{"saturday -> friday", date(2026, 4, 11), time.Friday},
		{"sunday -> friday", date(2026, 4, 12), time.Friday},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prevTradingDayOrToday(tt.in)
			if got.Weekday() != tt.want {
				t.Errorf("got %s, want %s", got.Weekday(), tt.want)
			}
		})
	}
}

func TestParseExpiry(t *testing.T) {
	tests := []struct {
		in   string
		want string
		zero bool
	}{
		{"26-Apr-2026", "2026-04-26", false},
		{"2026-04-26", "2026-04-26", false},
		{"26-APR-2026", "2026-04-26", false},
		{"", "", true},
		{"-", "", true},
		{"garbage", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := parseExpiry(tt.in)
			if tt.zero && !got.IsZero() {
				t.Errorf("expected zero time for %q, got %s", tt.in, got)
			}
			if !tt.zero && got.Format("2006-01-02") != tt.want {
				t.Errorf("parseExpiry(%q) = %s, want %s", tt.in, got.Format("2006-01-02"), tt.want)
			}
		})
	}
}

func TestParseBhavCSV(t *testing.T) {
	csv := `FinInstrmNm,FinInstrmTp,TckrSymb,XpryDt,StrkPric,OptnTp,OpnPric,HghPric,LwPric,ClsPric,TtlTradgVol,TmStmp,NewBrdLotQty
NIFTY26APR22500CE,OPTIDX,NIFTY,26-Apr-2026,22500,CE,150.00,180.00,140.00,170.00,50000,17-Apr-2026,75
NIFTY26APRFUT,FUTIDX,NIFTY,26-Apr-2026,0,-,22200.00,22500.00,22100.00,22400.00,100000,17-Apr-2026,75
`

	fallback := date(2026, 4, 17)
	rows, err := parseBhavCSV(strings.NewReader(csv), fallback)
	if err != nil {
		t.Fatalf("parseBhavCSV: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	opt := rows[0]
	if opt.Symbol != "NIFTY26APR22500CE" {
		t.Errorf("symbol = %q", opt.Symbol)
	}
	if opt.Underlying != "NIFTY" {
		t.Errorf("underlying = %q", opt.Underlying)
	}
	if opt.LotSize != 75 {
		t.Errorf("lot size = %d, want 75", opt.LotSize)
	}
	if opt.InstrumentType != "OPTIDX" {
		t.Errorf("instrument type = %q", opt.InstrumentType)
	}
	if opt.OptionType != "CE" {
		t.Errorf("option type = %q", opt.OptionType)
	}
	if opt.Strike != 22500 {
		t.Errorf("strike = %f", opt.Strike)
	}
	if opt.Open != 150 || opt.High != 180 || opt.Low != 140 || opt.Close != 170 {
		t.Errorf("OHLC = %.0f/%.0f/%.0f/%.0f", opt.Open, opt.High, opt.Low, opt.Close)
	}
	if opt.Volume != 50000 {
		t.Errorf("volume = %d", opt.Volume)
	}

	fut := rows[1]
	if fut.Symbol != "NIFTY26APRFUT" {
		t.Errorf("futures symbol = %q", fut.Symbol)
	}
	if fut.OptionType != "-" {
		t.Errorf("futures option type = %q, want \"-\"", fut.OptionType)
	}
}

func TestParseBhavCSV_MissingFinInstrmNm(t *testing.T) {
	csv := `SYMBOL,INSTRUMENT,OPEN,HIGH,LOW,CLOSE
NIFTY,FUTIDX,100,110,90,105
`
	_, err := parseBhavCSV(strings.NewReader(csv), date(2026, 4, 17))
	if err == nil {
		t.Fatal("expected error when FinInstrmNm column is missing")
	}
}

func TestParseBhavCSV_SkipsMalformedRows(t *testing.T) {
	csv := `FinInstrmNm,FinInstrmTp,TckrSymb,XpryDt,StrkPric,OptnTp,OpnPric,HghPric,LwPric,ClsPric,TtlTradgVol,TmStmp
GOOD,FUTIDX,NIFTY,26-Apr-2026,0,-,100.00,110.00,90.00,105.00,1000,17-Apr-2026
BAD,FUTIDX,NIFTY,26-Apr-2026,0,-,not_a_number,110.00,90.00,105.00,1000,17-Apr-2026
`
	rows, err := parseBhavCSV(strings.NewReader(csv), date(2026, 4, 17))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("expected 1 valid row, got %d", len(rows))
	}
	if rows[0].Symbol != "GOOD" {
		t.Errorf("expected GOOD row, got %q", rows[0].Symbol)
	}
}

func TestBhavRowToInstrument(t *testing.T) {
	expiry := date(2026, 4, 26)
	row := bhavRow{
		InstrumentType: "OPTIDX",
		Symbol:         "NIFTY26APR22500CE",
		Underlying:     "NIFTY",
		Expiry:         expiry,
		Strike:         22500,
		OptionType:     "CE",
		LotSize:        75,
	}

	inst := bhavRowToInstrument(row)
	if inst.Symbol != "NIFTY26APR22500CE" {
		t.Errorf("symbol = %q", inst.Symbol)
	}
	if inst.Exchange != models.ExchangeNFO {
		t.Errorf("exchange = %q", inst.Exchange)
	}
	if inst.InstrumentType != "OPTIDX" {
		t.Errorf("instrument type = %q", inst.InstrumentType)
	}
	if inst.Underlying == nil || *inst.Underlying != "NIFTY" {
		t.Errorf("underlying = %v", inst.Underlying)
	}
	if inst.Expiry == nil || !inst.Expiry.Equal(expiry) {
		t.Errorf("expiry = %v", inst.Expiry)
	}
	if inst.Strike == nil || *inst.Strike != 22500 {
		t.Errorf("strike = %v", inst.Strike)
	}
	if inst.OptionType == nil || *inst.OptionType != "CE" {
		t.Errorf("option type = %v", inst.OptionType)
	}
	if inst.LotSize != 75 {
		t.Errorf("lot size = %d, want 75", inst.LotSize)
	}
}

func TestBhavRowToInstrument_Futures(t *testing.T) {
	row := bhavRow{
		InstrumentType: "FUTIDX",
		Symbol:         "NIFTY26APRFUT",
		Underlying:     "NIFTY",
		Expiry:         date(2026, 4, 26),
		Strike:         0,
		OptionType:     "-",
	}

	inst := bhavRowToInstrument(row)
	if inst.Strike != nil {
		t.Errorf("futures should have nil strike, got %v", inst.Strike)
	}
	if inst.OptionType != nil {
		t.Errorf("futures '-' should map to nil option type, got %v", inst.OptionType)
	}
}

func TestWithTargetDate(t *testing.T) {
	target := date(2026, 4, 15)
	p := &EODProvider{}
	WithTargetDate(target)(p)
	if !p.targetDate.Equal(target) {
		t.Errorf("targetDate = %v, want %v", p.targetDate, target)
	}
}

func TestParseIndicesCSV(t *testing.T) {
	csv := `Index Name,Index Date,Open Index Value,High Index Value,Low Index Value,Closing Index Value,Points Change,Change(%),Volume,Turnover (Rs. Cr.),P/E,P/B,Div Yield
Nifty 50,17-04-2026,22100.50,22500.75,22050.25,22400.60,150.10,0.67,500000000,25000.50,22.5,4.1,1.2
Nifty Bank,17-04-2026,48500.00,49200.00,48300.00,49100.00,300.00,0.61,200000000,12000.00,18.3,3.5,0.8
Nifty Financial Services,17-04-2026,22800.00,23100.00,22700.00,23000.00,120.00,0.52,100000000,8000.00,20.1,3.8,1.0
Nifty IT,17-04-2026,35000.00,35500.00,34800.00,35200.00,200.00,0.57,50000000,3000.00,28.5,8.2,1.5
`

	fallback := date(2026, 4, 17)
	rows, err := parseIndicesCSV(strings.NewReader(csv), fallback)
	if err != nil {
		t.Fatalf("parseIndicesCSV: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (filtered to known indices), got %d", len(rows))
	}

	nifty := rows[0]
	if nifty.Symbol != models.UnderlyingNifty {
		t.Errorf("symbol = %q, want %q", nifty.Symbol, models.UnderlyingNifty)
	}
	if nifty.Open != 22100.50 {
		t.Errorf("open = %f, want 22100.50", nifty.Open)
	}
	if nifty.High != 22500.75 {
		t.Errorf("high = %f, want 22500.75", nifty.High)
	}
	if nifty.Low != 22050.25 {
		t.Errorf("low = %f, want 22050.25", nifty.Low)
	}
	if nifty.Close != 22400.60 {
		t.Errorf("close = %f, want 22400.60", nifty.Close)
	}
	if nifty.Volume != 500000000 {
		t.Errorf("volume = %d, want 500000000", nifty.Volume)
	}

	bank := rows[1]
	if bank.Symbol != models.UnderlyingBankNifty {
		t.Errorf("bank symbol = %q, want %q", bank.Symbol, models.UnderlyingBankNifty)
	}

	fin := rows[2]
	if fin.Symbol != models.UnderlyingFinNifty {
		t.Errorf("fin symbol = %q, want %q", fin.Symbol, models.UnderlyingFinNifty)
	}
}

func TestParseIndicesCSV_MissingIndexNameCol(t *testing.T) {
	csv := `Name,Date,Open,High,Low,Close
Nifty 50,17-04-2026,22100,22500,22050,22400
`
	_, err := parseIndicesCSV(strings.NewReader(csv), date(2026, 4, 17))
	if err == nil {
		t.Fatal("expected error when Index Name column is missing")
	}
}

func TestParseIndicesCSV_SkipsMalformedRows(t *testing.T) {
	csv := `Index Name,Index Date,Open Index Value,High Index Value,Low Index Value,Closing Index Value,Volume
Nifty 50,17-04-2026,22100.50,22500.75,22050.25,22400.60,500000
Nifty Bank,17-04-2026,not_a_number,49200.00,48300.00,49100.00,200000
`
	rows, err := parseIndicesCSV(strings.NewReader(csv), date(2026, 4, 17))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("expected 1 valid row, got %d", len(rows))
	}
	if rows[0].Symbol != models.UnderlyingNifty {
		t.Errorf("expected NIFTY row, got %q", rows[0].Symbol)
	}
}

func TestParseIndicesCSV_UsesDateFromCSV(t *testing.T) {
	csv := `Index Name,Index Date,Open Index Value,High Index Value,Low Index Value,Closing Index Value,Volume
Nifty 50,15-04-2026,22100.50,22500.75,22050.25,22400.60,500000
`
	rows, err := parseIndicesCSV(strings.NewReader(csv), date(2026, 4, 17))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	want := date(2026, 4, 15)
	if !rows[0].Date.Equal(want) {
		t.Errorf("date = %v, want %v", rows[0].Date, want)
	}
}

func TestIndexRowToInstrument(t *testing.T) {
	row := indexRow{
		Symbol: models.UnderlyingNifty,
		Open:   22100,
		High:   22500,
		Low:    22050,
		Close:  22400,
		Volume: 500000,
		Date:   date(2026, 4, 17),
	}

	inst := indexRowToInstrument(row)
	if inst.Symbol != models.UnderlyingNifty {
		t.Errorf("symbol = %q", inst.Symbol)
	}
	if inst.Exchange != models.ExchangeNSE {
		t.Errorf("exchange = %q, want %q", inst.Exchange, models.ExchangeNSE)
	}
	if inst.InstrumentType != models.InstrumentTypeIndex {
		t.Errorf("instrument type = %q, want %q", inst.InstrumentType, models.InstrumentTypeIndex)
	}
	if inst.Underlying == nil || *inst.Underlying != models.UnderlyingNifty {
		t.Errorf("underlying = %v", inst.Underlying)
	}
	if inst.LotSize != indexLotSize {
		t.Errorf("lot size = %d, want %d", inst.LotSize, indexLotSize)
	}
	if inst.Expiry != nil {
		t.Errorf("index should have nil expiry, got %v", inst.Expiry)
	}
	if inst.Strike != nil {
		t.Errorf("index should have nil strike, got %v", inst.Strike)
	}
	if inst.OptionType != nil {
		t.Errorf("index should have nil option type, got %v", inst.OptionType)
	}
}

func date(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}
