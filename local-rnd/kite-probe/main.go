package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/deependra191/algoedgefno-backend/local-rnd/kite"
)

const (
	dateLayout        = "2006-01-02"
	dateTimeLayout    = "2006-01-02 15:04:05"
	istLocationName   = "Asia/Kolkata"
	defaultUnderlying = "NIFTY"
	defaultIndexSpot  = "NIFTY 50"

	defaultDepthDates = "2018-01-02,2019-01-02,2020-01-02,2021-01-04,2022-01-03,2023-01-02,2024-01-02,2025-01-02,2026-01-02"
	defaultCapDays    = "30,60,61,90,120"
	defaultActiveDays = "120,90,60,30,7"

	defaultExpiredFrom = "2024-01-01"
	defaultExpiredTo   = "2024-01-05"
	defaultRollFrom    = "2024-12-20"
	defaultRollTo      = "2025-01-05"
	defaultBurstCount  = 6
	defaultTimeout     = 45 * time.Second
	sessionOpen        = "09:15:00"
	sessionClose       = "15:30:00"
)

const (
	exitOK    = 0
	exitError = 1
)

func main() {
	os.Exit(run())
}

func run() int {
	underlying := flag.String("underlying", defaultUnderlying, "underlying used to auto-resolve the nearest active futures contract")
	indexSymbol := flag.String("index-symbol", defaultIndexSpot, "NSE index spot tradingsymbol to probe")
	indexToken := flag.String("index-token", "", "override index spot instrument token")
	futureToken := flag.String("future-token", "", "override active futures instrument token")
	expiredFrom := flag.String("expired-from", defaultExpiredFrom, "expired-period probe start date in YYYY-MM-DD")
	expiredTo := flag.String("expired-to", defaultExpiredTo, "expired-period probe end date in YYYY-MM-DD")
	rollFrom := flag.String("roll-from", defaultRollFrom, "roll comparison start date in YYYY-MM-DD")
	rollTo := flag.String("roll-to", defaultRollTo, "roll comparison end date in YYYY-MM-DD")
	depthDates := flag.String("depth-dates", defaultDepthDates, "comma-separated YYYY-MM-DD dates for index spot depth probe")
	capDays := flag.String("cap-days", defaultCapDays, "comma-separated minute window sizes to test")
	activeDays := flag.String("active-days", defaultActiveDays, "comma-separated lookback days for active-futures minute probe")
	burstCount := flag.Int("burst", defaultBurstCount, "number of concurrent historical requests in the short burst")
	timeout := flag.Duration("timeout", defaultTimeout, "per-section timeout")
	flag.Parse()

	ist, err := time.LoadLocation(istLocationName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load IST location: %v\n", err)
		return exitError
	}

	creds, err := kite.LoadReadOnlyCredentials()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load Kite credentials: %v\n", err)
		fmt.Fprintln(os.Stderr, "See local-rnd/kite-probe/README.md for the interactive login flow.")
		return exitError
	}
	client := kite.NewClient(creds)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	instruments, instrumentsPreview, err := client.Instruments(ctx)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch instruments: %v\n", err)
		if instrumentsPreview != "" {
			fmt.Printf("\n/instruments raw preview:\n%s\n", instrumentsPreview)
		}
		return exitError
	}

	printSection("Kite live probe")
	fmt.Printf("instrument rows: %d\n", len(instruments))
	fmt.Printf("/instruments raw preview:\n%s\n", instrumentsPreview)

	index := resolveIndex(instruments, *indexSymbol, *indexToken)
	if index.InstrumentToken == "" {
		fmt.Fprintf(os.Stderr, "could not resolve index token for NSE:%s; pass -index-token\n", *indexSymbol)
		return exitError
	}
	fmt.Printf("\nresolved index spot: %s\n", instrumentSummary(index))

	future := resolveFuture(instruments, *underlying, *futureToken, time.Now())
	if future.InstrumentToken == "" {
		fmt.Fprintf(os.Stderr, "could not resolve active future for %s; pass -future-token\n", *underlying)
		return exitError
	}
	fmt.Printf("resolved active future: %s\n", instrumentSummary(future))

	if err := runExpiredContinuousMinuteProbe(client, future, *expiredFrom, *expiredTo, ist, *timeout); err != nil {
		fmt.Fprintf(os.Stderr, "expired continuous minute probe: %v\n", err)
	}
	if err := runIndexSpotDepthProbe(client, index, *depthDates, ist, *timeout); err != nil {
		fmt.Fprintf(os.Stderr, "index spot depth probe: %v\n", err)
	}
	if err := runActiveFutureDepthProbe(client, future, *activeDays, ist, *timeout); err != nil {
		fmt.Fprintf(os.Stderr, "active future depth probe: %v\n", err)
	}
	if err := runRollComparisonProbe(client, future, *rollFrom, *rollTo, ist, *timeout); err != nil {
		fmt.Fprintf(os.Stderr, "roll comparison probe: %v\n", err)
	}
	if err := runWindowCapProbe(client, index, *capDays, ist, *timeout); err != nil {
		fmt.Fprintf(os.Stderr, "window cap probe: %v\n", err)
	}
	if err := runBurstProbe(client, index, *burstCount, ist, *timeout); err != nil {
		fmt.Fprintf(os.Stderr, "burst probe: %v\n", err)
	}

	fmt.Println("\nProbe complete. Paste the output back into the PR task thread; do not paste any env values or tokens.")
	return exitOK
}

func runExpiredContinuousMinuteProbe(client *kite.Client, future kite.Instrument, from, to string, loc *time.Location, timeout time.Duration) error {
	printSection("1. Expired futures continuous=1 + minute")
	start, end, err := sessionRange(from, to, loc)
	if err != nil {
		return err
	}
	req := kite.HistoricalRequest{
		InstrumentToken: future.InstrumentToken,
		Interval:        kite.IntervalMinute,
		From:            start,
		To:              end,
		Continuous:      true,
	}
	return printHistoricalResult(client, req, timeout)
}

func runIndexSpotDepthProbe(client *kite.Client, index kite.Instrument, dates string, loc *time.Location, timeout time.Duration) error {
	printSection("2. Index spot 1-minute depth")
	for _, date := range splitCSV(dates) {
		start, end, err := sessionRange(date, date, loc)
		if err != nil {
			return err
		}
		fmt.Printf("\nDepth date %s\n", date)
		req := kite.HistoricalRequest{
			InstrumentToken: index.InstrumentToken,
			Interval:        kite.IntervalMinute,
			From:            start,
			To:              end,
		}
		if err := printHistoricalResult(client, req, timeout); err != nil {
			return err
		}
	}
	return nil
}

func runActiveFutureDepthProbe(client *kite.Client, future kite.Instrument, daysCSV string, loc *time.Location, timeout time.Duration) error {
	printSection("3. Active-contract futures minute depth")
	today := time.Now().In(loc)
	for _, daysValue := range splitCSV(daysCSV) {
		days, err := strconv.Atoi(daysValue)
		if err != nil {
			return fmt.Errorf("parse active-days value %q: %w", daysValue, err)
		}
		day := today.AddDate(0, 0, -days)
		from := day.Format(dateLayout)
		start, end, err := sessionRange(from, from, loc)
		if err != nil {
			return err
		}
		fmt.Printf("\nActive future lookback %d days (%s)\n", days, from)
		req := kite.HistoricalRequest{
			InstrumentToken: future.InstrumentToken,
			Interval:        kite.IntervalMinute,
			From:            start,
			To:              end,
		}
		if err := printHistoricalResult(client, req, timeout); err != nil {
			return err
		}
	}
	return nil
}

func runRollComparisonProbe(client *kite.Client, future kite.Instrument, from, to string, loc *time.Location, timeout time.Duration) error {
	printSection("4. Continuous day bars around roll")
	start, end, err := sessionRange(from, to, loc)
	if err != nil {
		return err
	}

	fmt.Println("\ncontinuous=1 day")
	continuousReq := kite.HistoricalRequest{
		InstrumentToken: future.InstrumentToken,
		Interval:        kite.IntervalDay,
		From:            start,
		To:              end,
		Continuous:      true,
	}
	if err := printHistoricalResult(client, continuousReq, timeout); err != nil {
		return err
	}

	fmt.Println("\ncontinuous=0 day")
	plainReq := kite.HistoricalRequest{
		InstrumentToken: future.InstrumentToken,
		Interval:        kite.IntervalDay,
		From:            start,
		To:              end,
	}
	return printHistoricalResult(client, plainReq, timeout)
}

func runWindowCapProbe(client *kite.Client, index kite.Instrument, daysCSV string, loc *time.Location, timeout time.Duration) error {
	printSection("5. Minute per-request window cap")
	end := time.Now().In(loc)
	for _, daysValue := range splitCSV(daysCSV) {
		days, err := strconv.Atoi(daysValue)
		if err != nil {
			return fmt.Errorf("parse cap-days value %q: %w", daysValue, err)
		}
		start := end.AddDate(0, 0, -days)
		fmt.Printf("\nWindow %d days (%s -> %s)\n", days, start.Format(dateLayout), end.Format(dateLayout))
		req := kite.HistoricalRequest{
			InstrumentToken: index.InstrumentToken,
			Interval:        kite.IntervalMinute,
			From:            withClock(start, sessionOpen, loc),
			To:              withClock(end, sessionClose, loc),
		}
		if err := printHistoricalResult(client, req, timeout); err != nil {
			return err
		}
	}
	return nil
}

func runBurstProbe(client *kite.Client, index kite.Instrument, count int, loc *time.Location, timeout time.Duration) error {
	printSection("6. Historical endpoint short burst")
	if count <= 0 {
		fmt.Println("burst disabled")
		return nil
	}
	date := time.Now().In(loc).AddDate(0, 0, -7).Format(dateLayout)
	start, end, err := sessionRange(date, date, loc)
	if err != nil {
		return err
	}

	type burstResult struct {
		Index      int
		HTTPStatus int
		APIStatus  string
		ErrorType  string
		Message    string
		Elapsed    time.Duration
		Err        error
	}

	results := make(chan burstResult, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			started := time.Now()
			res, err := client.FetchHistorical(ctx, kite.HistoricalRequest{
				InstrumentToken: index.InstrumentToken,
				Interval:        kite.IntervalMinute,
				From:            start,
				To:              end,
			})
			item := burstResult{Index: i + 1, Elapsed: time.Since(started), Err: err}
			if res != nil {
				item.HTTPStatus = res.HTTPStatus
				item.APIStatus = res.APIStatus
				item.ErrorType = res.ErrorType
				item.Message = res.Message
			}
			results <- item
		}(i)
	}
	wg.Wait()
	close(results)

	for item := range results {
		fmt.Printf("request=%d http=%d api_status=%s error_type=%s elapsed=%s message=%q err=%v\n",
			item.Index, item.HTTPStatus, item.APIStatus, item.ErrorType, item.Elapsed.Round(time.Millisecond), item.Message, item.Err)
	}
	return nil
}

func printHistoricalResult(client *kite.Client, req kite.HistoricalRequest, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	result, err := client.FetchHistorical(ctx, req)
	if err != nil {
		return err
	}
	fmt.Printf("request: token=%s interval=%s from=%s to=%s continuous=%t oi=%t\n",
		req.InstrumentToken, req.Interval, req.From.Format(dateTimeLayout), req.To.Format(dateTimeLayout), req.Continuous, req.OI)
	fmt.Printf("response: http=%d api_status=%s error_type=%s message=%q candles=%d\n",
		result.HTTPStatus, result.APIStatus, result.ErrorType, result.Message, len(result.Candles))
	fmt.Printf("raw preview:\n%s\n", result.RawPreview)
	printCandleSummary(result.Candles)
	return nil
}

func printCandleSummary(candles []kite.Candle) {
	if len(candles) == 0 {
		return
	}
	first := candles[0]
	last := candles[len(candles)-1]
	_, firstOffset := first.Timestamp.Zone()
	_, lastOffset := last.Timestamp.Zone()
	fmt.Printf("first timestamp raw=%q parsed=%s offset_seconds=%d\n", first.RawTimestamp, first.Timestamp.Format(time.RFC3339), firstOffset)
	fmt.Printf("last timestamp raw=%q parsed=%s offset_seconds=%d\n", last.RawTimestamp, last.Timestamp.Format(time.RFC3339), lastOffset)
	fmt.Println("sample candles:")
	for _, candle := range sampleCandles(candles) {
		fmt.Printf("  %s O=%.2f H=%.2f L=%.2f C=%.2f V=%d\n",
			candle.RawTimestamp, candle.Open, candle.High, candle.Low, candle.Close, candle.Volume)
	}
}

func sampleCandles(candles []kite.Candle) []kite.Candle {
	if len(candles) <= 6 {
		return candles
	}
	sampled := make([]kite.Candle, 0, 6)
	sampled = append(sampled, candles[:3]...)
	sampled = append(sampled, candles[len(candles)-3:]...)
	return sampled
}

func resolveIndex(instruments []kite.Instrument, symbol, token string) kite.Instrument {
	if token != "" {
		return kite.Instrument{InstrumentToken: token, TradingSymbol: symbol, Exchange: kite.ExchangeNSE, InstrumentType: kite.InstrumentTypeIndex}
	}
	inst, ok := kite.FindByExchangeSymbol(instruments, kite.ExchangeNSE, symbol)
	if !ok {
		return kite.Instrument{}
	}
	return inst
}

func resolveFuture(instruments []kite.Instrument, underlying, token string, asOf time.Time) kite.Instrument {
	if token != "" {
		return kite.Instrument{InstrumentToken: token, TradingSymbol: underlying + " FUT override", Exchange: kite.ExchangeNFO, InstrumentType: kite.InstrumentTypeFuture}
	}
	inst, ok := kite.FindNearestFuture(instruments, underlying, asOf)
	if !ok {
		return kite.Instrument{}
	}
	return inst
}

func instrumentSummary(inst kite.Instrument) string {
	return fmt.Sprintf("token=%s exchange=%s symbol=%s name=%s type=%s segment=%s expiry=%s lot_size=%s",
		inst.InstrumentToken, inst.Exchange, inst.TradingSymbol, inst.Name, inst.InstrumentType, inst.Segment, inst.Expiry, inst.LotSize)
}

func sessionRange(fromDate, toDate string, loc *time.Location) (time.Time, time.Time, error) {
	from, err := parseDate(fromDate, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	to, err := parseDate(toDate, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return withClock(from, sessionOpen, loc), withClock(to, sessionClose, loc), nil
}

func parseDate(value string, loc *time.Location) (time.Time, error) {
	t, err := time.ParseInLocation(dateLayout, strings.TrimSpace(value), loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse date %q: %w", value, err)
	}
	return t, nil
}

func withClock(day time.Time, clock string, loc *time.Location) time.Time {
	parts := strings.Split(clock, ":")
	hour, _ := strconv.Atoi(parts[0])
	minute, _ := strconv.Atoi(parts[1])
	second, _ := strconv.Atoi(parts[2])
	return time.Date(day.Year(), day.Month(), day.Day(), hour, minute, second, 0, loc)
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
}

func printSection(title string) {
	fmt.Printf("\n\n=== %s ===\n", title)
}
