package nse

import (
	"strings"
	"testing"
	"time"
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

func TestParseFloat(t *testing.T) {
	tests := []struct {
		in      string
		want    float64
		wantErr bool
	}{
		{"100.50", 100.50, false},
		{"0", 0, false},
		{"", 0, true},
		{"-", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseFloat(tt.in)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFloat(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseFloat(%q) = %f, want %f", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseInt64(t *testing.T) {
	got, err := parseInt64("12345")
	if err != nil || got != 12345 {
		t.Errorf("parseInt64(\"12345\") = %d, %v", got, err)
	}
	got, _ = parseInt64("")
	if got != 0 {
		t.Errorf("parseInt64(\"\") = %d, want 0", got)
	}
	got, _ = parseInt64("-")
	if got != 0 {
		t.Errorf("parseInt64(\"-\") = %d, want 0", got)
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
	if inst.Exchange != nseExchange {
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

func TestSetTargetDate(t *testing.T) {
	p := &EODProvider{}
	target := date(2026, 4, 15)
	p.SetTargetDate(target)
	if !p.targetDate.Equal(target) {
		t.Errorf("targetDate = %v, want %v", p.targetDate, target)
	}
}

func date(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}
