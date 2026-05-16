package handlers

import (
	"errors"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

const dateFormat = "2006-01-02"

const (
	defaultBacktestsPage  = 1
	defaultBacktestsLimit = 20
	maxBacktestsLimit     = 100
	defaultTradesPage     = 1
	defaultTradesLimit    = 50
	maxTradesLimit        = 200
)

var errInvalidBacktestSummary = errors.New("invalid completed backtest summary")

// backtestSubmitResponse is returned immediately by POST /backtests (HTTP 202).
type backtestSubmitResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// backtestStatusResponse is returned by GET /backtests/:id for all run states.
// Result is non-nil only when Status == COMPLETED.
// ErrorMessage is non-nil only when Status == FAILED.
type backtestStatusResponse struct {
	ID           string                 `json:"id"`
	Status       string                 `json:"status"`
	ErrorMessage *string                `json:"errorMessage,omitempty"`
	Result       *backtestResultPayload `json:"result,omitempty"`
}

type backtestStrategyResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// backtestResultPayload carries the full backtest result, present only on COMPLETED runs.
type backtestResultPayload struct {
	Strategy          backtestStrategyResponse `json:"strategy"`
	Underlying        string                   `json:"underlying"`
	Interval          string                   `json:"interval"`
	From              string                   `json:"from"`
	To                string                   `json:"to"`
	Lots              int                      `json:"lots"`
	CapStart          float64                  `json:"capStart"`
	CapEnd            float64                  `json:"capEnd"`
	NetPnl            float64                  `json:"netPnl"`
	GrossPnl          float64                  `json:"grossPnl"`
	TotalCharges      float64                  `json:"totalCharges"`
	ReturnPct         float64                  `json:"returnPct"`
	TradeCount        int                      `json:"tradeCount"`
	WinRate           int                      `json:"winRate"`
	MaxDrawdownPct    float64                  `json:"maxDrawdownPct"`
	AvgWin            *float64                 `json:"avgWin"`
	AvgLoss           *float64                 `json:"avgLoss"`
	BestTrade         *float64                 `json:"bestTrade"`
	WorstTrade        *float64                 `json:"worstTrade"`
	AvgPnlPerTrade    *float64                 `json:"avgPnlPerTrade"`
	AvgHoldingMinutes *float64                 `json:"avgHoldingMinutes"`
	ProfitFactor      *float64                 `json:"profitFactor"`
	RewardRisk        *float64                 `json:"rewardRisk"`
	LongestWinStreak  int                      `json:"longestWinStreak"`
	LongestLossStreak int                      `json:"longestLossStreak"`
	TradesPerWeek     float64                  `json:"tradesPerWeek"`
	Chart             backtestChartResponse    `json:"chart"`
}

type backtestChartResponse struct {
	Equity []chartPointResponse `json:"equity"`
}

type chartPointResponse struct {
	Ts    string  `json:"ts"`
	Value float64 `json:"value"`
}

// backtestTradesPageResponse is returned by GET /backtests/:id/trades.
type backtestTradesPageResponse struct {
	Trades []backtestTradeResponse `json:"trades"`
	Total  int                     `json:"total"`
	Page   int                     `json:"page"`
	Limit  int                     `json:"limit"`
}

type backtestTradeResponse struct {
	EntryTs      string  `json:"entryTs"`
	ExitTs       string  `json:"exitTs"`
	Side         string  `json:"side"`
	Quantity     int     `json:"quantity"`
	EntryPrice   float64 `json:"entryPrice"`
	ExitPrice    float64 `json:"exitPrice"`
	Pnl          float64 `json:"pnl"`
	GrossPnl     float64 `json:"grossPnl"`
	Slippage     float64 `json:"slippage"`
	Brokerage    float64 `json:"brokerage"`
	STT          float64 `json:"stt"`
	ExchangeFees float64 `json:"exchangeFees"`
	SEBIFees     float64 `json:"sebiFees"`
	GST          float64 `json:"gst"`
	StampDuty    float64 `json:"stampDuty"`
	TotalCharges float64 `json:"totalCharges"`
	Reason       string  `json:"reason"`
	ExitReason   string  `json:"exitReason"`
}

type backtestListResponse struct {
	Runs  []backtestSummaryResponse `json:"runs"`
	Page  int                       `json:"page"`
	Limit int                       `json:"limit"`
	Total int                       `json:"total"`
}

type backtestSummaryResponse struct {
	ID          string                        `json:"id"`
	Status      string                        `json:"status"`
	CompletedAt string                        `json:"completedAt"`
	Result      backtestSummaryResultResponse `json:"result"`
}

type backtestSummaryResultResponse struct {
	Strategy       backtestStrategyResponse `json:"strategy"`
	Underlying     string                   `json:"underlying"`
	Interval       string                   `json:"interval"`
	From           string                   `json:"from"`
	To             string                   `json:"to"`
	CapStart       float64                  `json:"capStart"`
	CapEnd         float64                  `json:"capEnd"`
	NetPnl         float64                  `json:"netPnl"`
	GrossPnl       float64                  `json:"grossPnl"`
	TotalCharges   float64                  `json:"totalCharges"`
	ReturnPct      float64                  `json:"returnPct"`
	TradeCount     int                      `json:"tradeCount"`
	WinRate        *int                     `json:"winRate"`
	MaxDrawdownPct *float64                 `json:"maxDrawdownPct"`
}

type backtestSubmitRequest struct {
	StrategyID string                `json:"strategyId" binding:"required"`
	Inputs     backtestInputsRequest `json:"inputs"     binding:"required"`
}

type backtestInputsRequest struct {
	Underlying string            `json:"underlying" binding:"required"`
	DateRange  backtestDateRange `json:"dateRange"  binding:"required"`
	Lots       int               `json:"lots"       binding:"required,min=1,max=50"`
	Capital    float64           `json:"capital"    binding:"required,gt=0,max=10000000"`
}

type backtestDateRange struct {
	From string `json:"from" binding:"required"`
	To   string `json:"to"   binding:"required"`
}

func toBacktestSubmitResponse(run *models.BacktestRun) backtestSubmitResponse {
	return backtestSubmitResponse{
		ID:     run.ID.String(),
		Status: run.Status,
	}
}

func toBacktestStatusResponse(run *models.BacktestRun) backtestStatusResponse {
	resp := backtestStatusResponse{
		ID:           run.ID.String(),
		Status:       run.Status,
		ErrorMessage: run.ErrorMessage,
	}
	if run.Status == models.BacktestCompleted {
		payload := toBacktestResultPayload(run)
		resp.Result = payload
	}
	return resp
}

func toBacktestResultPayload(run *models.BacktestRun) *backtestResultPayload {
	capital := derefFloat(run.Capital)
	netPnl := derefFloat(run.NetPnl)

	p := &backtestResultPayload{
		Strategy: backtestStrategyResponse{
			ID:   derefStr(run.StrategySlug),
			Name: derefStr(run.StrategyName),
		},
		Underlying:     derefStr(run.Underlying),
		Interval:       run.CandleInterval,
		From:           run.FromTs.UTC().Format(dateFormat),
		To:             run.ToTs.UTC().Format(dateFormat),
		Lots:           derefInt(run.Lots),
		CapStart:       capital,
		CapEnd:         round2(capital + netPnl),
		NetPnl:         round2(netPnl),
		GrossPnl:       round2(derefFloat(run.GrossPnl)),
		TotalCharges:   round2(derefFloat(run.TotalCharges)),
		ReturnPct:      computeReturnPct(run),
		TradeCount:     derefInt(run.TotalTrades),
		WinRate:        computeWinRate(run),
		MaxDrawdownPct: round2(derefFloat(run.MaxDrawdown) * 100),
		Chart: backtestChartResponse{
			Equity: []chartPointResponse{},
		},
	}

	if s := run.ResultStats; s != nil {
		p.AvgWin = s.AvgWin
		p.AvgLoss = s.AvgLoss
		p.BestTrade = s.BestTrade
		p.WorstTrade = s.WorstTrade
		p.AvgPnlPerTrade = s.AvgPnlPerTrade
		p.AvgHoldingMinutes = s.AvgHoldingMinutes
		p.ProfitFactor = s.ProfitFactor
		p.RewardRisk = s.RewardRisk
		p.LongestWinStreak = s.LongestWinStreak
		p.LongestLossStreak = s.LongestLossStreak
		p.TradesPerWeek = s.TradesPerWeek
	}

	if cd := run.ChartData; cd != nil {
		p.Chart = toBacktestChartResponse(cd)
	}

	return p
}

func toBacktestChartResponse(cd *models.ChartData) backtestChartResponse {
	equity := make([]chartPointResponse, len(cd.Equity))
	for i, pt := range cd.Equity {
		equity[i] = chartPointResponse{Ts: pt.Ts.UTC().Format(time.RFC3339), Value: pt.Value}
	}
	return backtestChartResponse{Equity: equity}
}

func toBacktestTradesPageResponse(trades []models.Trade, page, limit int) (backtestTradesPageResponse, error) {
	total := len(trades)

	start := (page - 1) * limit
	if start >= total {
		return backtestTradesPageResponse{
			Trades: []backtestTradeResponse{},
			Total:  total,
			Page:   page,
			Limit:  limit,
		}, nil
	}
	end := start + limit
	if end > total {
		end = total
	}
	slice := trades[start:end]

	items := make([]backtestTradeResponse, len(slice))
	for i, tr := range slice {
		items[i] = backtestTradeResponse{
			EntryTs:      tr.EntryTimestamp.UTC().Format(time.RFC3339),
			ExitTs:       tr.ExitTimestamp.UTC().Format(time.RFC3339),
			Side:         string(tr.Side),
			Quantity:     tr.Quantity,
			EntryPrice:   tr.EntryPrice,
			ExitPrice:    tr.ExitPrice,
			Pnl:          tr.NetPnL,
			GrossPnl:     tr.GrossPnL,
			Slippage:     tr.Slippage,
			Brokerage:    tr.Brokerage,
			STT:          tr.STT,
			ExchangeFees: tr.ExchangeFees,
			SEBIFees:     tr.SEBIFees,
			GST:          tr.GST,
			StampDuty:    tr.StampDuty,
			TotalCharges: tr.TotalCharges,
			Reason:       tr.Reason,
			ExitReason:   tr.ExitReason,
		}
	}

	return backtestTradesPageResponse{
		Trades: items,
		Total:  total,
		Page:   page,
		Limit:  limit,
	}, nil
}

func toBacktestListResponse(runs []models.BacktestRun, page, limit, total int) (backtestListResponse, error) {
	items := make([]backtestSummaryResponse, len(runs))
	for i := range runs {
		item, err := toBacktestSummaryResponse(&runs[i])
		if err != nil {
			return backtestListResponse{}, err
		}
		items[i] = item
	}
	return backtestListResponse{
		Runs:  items,
		Page:  page,
		Limit: limit,
		Total: total,
	}, nil
}

func toBacktestSummaryResponse(run *models.BacktestRun) (backtestSummaryResponse, error) {
	if err := validateBacktestSummary(run); err != nil {
		return backtestSummaryResponse{}, err
	}
	return backtestSummaryResponse{
		ID:          run.ID.String(),
		Status:      run.Status,
		CompletedAt: formatCompletedAt(run.CompletedAt),
		Result: backtestSummaryResultResponse{
			Strategy: backtestStrategyResponse{
				ID:   *run.StrategySlug,
				Name: *run.StrategyName,
			},
			Underlying:     *run.Underlying,
			Interval:       run.CandleInterval,
			From:           run.FromTs.UTC().Format(dateFormat),
			To:             run.ToTs.UTC().Format(dateFormat),
			CapStart:       *run.Capital,
			CapEnd:         round2(*run.Capital + *run.NetPnl),
			NetPnl:         round2(*run.NetPnl),
			GrossPnl:       round2(derefFloat(run.GrossPnl)),
			TotalCharges:   round2(derefFloat(run.TotalCharges)),
			ReturnPct:      computeReturnPct(run),
			TradeCount:     *run.TotalTrades,
			WinRate:        computeWinRatePtr(run),
			MaxDrawdownPct: computeMaxDrawdownPctPtr(run),
		},
	}, nil
}

func validateBacktestSummary(run *models.BacktestRun) error {
	if run == nil ||
		run.ID == uuid.Nil ||
		run.Status != models.BacktestCompleted ||
		run.CompletedAt == nil ||
		isBlankPtr(run.StrategySlug) ||
		isBlankPtr(run.StrategyName) ||
		isBlankPtr(run.Underlying) ||
		run.CandleInterval == "" ||
		run.FromTs.IsZero() ||
		run.ToTs.IsZero() ||
		run.Capital == nil ||
		run.NetPnl == nil ||
		run.TotalTrades == nil {
		return errInvalidBacktestSummary
	}
	return nil
}

func isBlankPtr(p *string) bool {
	return p == nil || *p == ""
}

func computeWinRatePtr(run *models.BacktestRun) *int {
	if run.TotalTrades == nil || *run.TotalTrades == 0 || run.WinCount == nil {
		return nil
	}
	winRate := *run.WinCount * 100 / *run.TotalTrades
	return &winRate
}

func computeMaxDrawdownPctPtr(run *models.BacktestRun) *float64 {
	if run.MaxDrawdown == nil {
		return nil
	}
	maxDrawdownPct := round2(*run.MaxDrawdown * 100)
	return &maxDrawdownPct
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func derefFloat(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
