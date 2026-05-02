package handlers

import (
	"encoding/json"
	"math"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

const dateFormat = "2006-01-02"

const (
	defaultTradesPage  = 1
	defaultTradesLimit = 50
	maxTradesLimit     = 200
)

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

// backtestResultPayload carries the full backtest result, present only on COMPLETED runs.
type backtestResultPayload struct {
	StrategySlug      string  `json:"strategySlug"`
	Underlying        string  `json:"underlying"`
	Interval          string  `json:"interval"`
	From              string  `json:"from"`
	To                string  `json:"to"`
	Lots              int     `json:"lots"`
	CapStart          float64 `json:"capStart"`
	CapEnd            float64 `json:"capEnd"`
	NetPnl            float64 `json:"netPnl"`
	ReturnPct         float64 `json:"returnPct"`
	TradeCount        int     `json:"tradeCount"`
	WinRate           int     `json:"winRate"`
	MaxDrawdownPct    float64 `json:"maxDrawdownPct"`
	AvgWin            *float64 `json:"avgWin"`
	AvgLoss           *float64 `json:"avgLoss"`
	BestTrade         *float64 `json:"bestTrade"`
	WorstTrade        *float64 `json:"worstTrade"`
	AvgPnlPerTrade    *float64 `json:"avgPnlPerTrade"`
	AvgHoldingMinutes *float64 `json:"avgHoldingMinutes"`
	ProfitFactor      *float64 `json:"profitFactor"`
	RewardRisk        *float64 `json:"rewardRisk"`
	LongestWinStreak  int      `json:"longestWinStreak"`
	LongestLossStreak int      `json:"longestLossStreak"`
	TradesPerWeek     float64  `json:"tradesPerWeek"`
	Chart             backtestChartResponse `json:"chart"`
}

type backtestChartResponse struct {
	Equity   []chartPointResponse `json:"equity"`
	Drawdown []chartPointResponse `json:"drawdown"`
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
	EntryTs    string  `json:"entryTs"`
	ExitTs     string  `json:"exitTs"`
	Side       string  `json:"side"`
	Quantity   int     `json:"quantity"`
	EntryPrice float64 `json:"entryPrice"`
	ExitPrice  float64 `json:"exitPrice"`
	Pnl        float64 `json:"pnl"`
	Reason     string  `json:"reason"`
	ExitReason string  `json:"exitReason"`
}

type backtestListResponse struct {
	Backtests []backtestSummaryResponse `json:"backtests"`
}

type backtestSummaryResponse struct {
	ID           string  `json:"id"`
	StrategySlug string  `json:"strategySlug"`
	Underlying   string  `json:"underlying"`
	From         string  `json:"from"`
	To           string  `json:"to"`
	Status       string  `json:"status"`
	ReturnPct    float64 `json:"returnPct"`
	WinRate      int     `json:"winRate"`
	TradeCount   int     `json:"tradeCount"`
	Capital      float64 `json:"capital"`
	RanAt        string  `json:"ranAt"`
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
		StrategySlug:   derefStr(run.StrategySlug),
		Underlying:     derefStr(run.Underlying),
		Interval:       run.CandleInterval,
		From:           run.FromTs.UTC().Format(dateFormat),
		To:             run.ToTs.UTC().Format(dateFormat),
		Lots:           derefInt(run.Lots),
		CapStart:       capital,
		CapEnd:         round2(capital + netPnl),
		NetPnl:         round2(netPnl),
		ReturnPct:      computeReturnPct(run),
		TradeCount:     derefInt(run.TotalTrades),
		WinRate:        computeWinRate(run),
		MaxDrawdownPct: round2(derefFloat(run.MaxDrawdown) * 100),
		Chart: backtestChartResponse{
			Equity:   []chartPointResponse{},
			Drawdown: []chartPointResponse{},
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
	drawdown := make([]chartPointResponse, len(cd.Drawdown))
	for i, pt := range cd.Drawdown {
		drawdown[i] = chartPointResponse{Ts: pt.Ts.UTC().Format(time.RFC3339), Value: pt.Value}
	}
	return backtestChartResponse{Equity: equity, Drawdown: drawdown}
}

func toBacktestTradesPageResponse(tradesJSON json.RawMessage, page, limit int) backtestTradesPageResponse {
	var all []models.Trade
	if len(tradesJSON) > 0 {
		_ = json.Unmarshal(tradesJSON, &all)
	}
	total := len(all)

	start := (page - 1) * limit
	if start >= total {
		return backtestTradesPageResponse{
			Trades: []backtestTradeResponse{},
			Total:  total,
			Page:   page,
			Limit:  limit,
		}
	}
	end := start + limit
	if end > total {
		end = total
	}
	slice := all[start:end]

	trades := make([]backtestTradeResponse, len(slice))
	for i, tr := range slice {
		trades[i] = backtestTradeResponse{
			EntryTs:    tr.EntryTimestamp.UTC().Format(time.RFC3339),
			ExitTs:     tr.ExitTimestamp.UTC().Format(time.RFC3339),
			Side:       string(tr.Side),
			Quantity:   tr.Quantity,
			EntryPrice: tr.EntryPrice,
			ExitPrice:  tr.ExitPrice,
			Pnl:        tr.PnL,
			Reason:     tr.Reason,
			ExitReason: tr.ExitReason,
		}
	}

	return backtestTradesPageResponse{
		Trades: trades,
		Total:  total,
		Page:   page,
		Limit:  limit,
	}
}

func toBacktestListResponse(runs []models.BacktestRun) backtestListResponse {
	items := make([]backtestSummaryResponse, len(runs))
	for i := range runs {
		items[i] = toBacktestSummaryResponse(&runs[i])
	}
	return backtestListResponse{Backtests: items}
}

func toBacktestSummaryResponse(run *models.BacktestRun) backtestSummaryResponse {
	slug := ""
	if run.StrategySlug != nil {
		slug = *run.StrategySlug
	}
	underlying := ""
	if run.Underlying != nil {
		underlying = *run.Underlying
	}
	capital := 0.0
	if run.Capital != nil {
		capital = *run.Capital
	}
	return backtestSummaryResponse{
		ID:           run.ID.String(),
		StrategySlug: slug,
		Underlying:   underlying,
		From:         run.FromTs.UTC().Format(dateFormat),
		To:           run.ToTs.UTC().Format(dateFormat),
		Status:       run.Status,
		ReturnPct:    computeReturnPct(run),
		WinRate:      computeWinRate(run),
		TradeCount:   derefInt(run.TotalTrades),
		Capital:      capital,
		RanAt:        formatCompletedAt(run.CompletedAt),
	}
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
