package handlers

import (
	"encoding/json"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

const dateFormat = "2006-01-02"

type backtestSubmitRequest struct {
	StrategyID string  `json:"strategyId" binding:"required"`
	Underlying string  `json:"underlying" binding:"required"`
	From       string  `json:"from"       binding:"required"`
	To         string  `json:"to"         binding:"required"`
	Lots       int     `json:"lots"       binding:"required,min=1,max=50"`
	Capital    float64 `json:"capital"    binding:"required,gt=0,max=10000000"`
}

type backtestResultResponse struct {
	ID         string                  `json:"id"`
	ReturnPct  float64                 `json:"returnPct"`
	WinRate    int                     `json:"winRate"`
	TradeCount int                     `json:"tradeCount"`
	Trades     []backtestTradeResponse `json:"trades"`
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

func toBacktestResultResponse(run *models.BacktestRun) backtestResultResponse {
	return backtestResultResponse{
		ID:         run.ID.String(),
		ReturnPct:  computeReturnPct(run),
		WinRate:    computeWinRate(run),
		TradeCount: derefInt(run.TotalTrades),
		Trades:     toBacktestTradeResponses(run.Trades),
	}
}

func toBacktestTradeResponses(tradesJSON json.RawMessage) []backtestTradeResponse {
	if len(tradesJSON) == 0 {
		return []backtestTradeResponse{}
	}
	var trades []models.Trade
	if err := json.Unmarshal(tradesJSON, &trades); err != nil {
		return []backtestTradeResponse{}
	}
	result := make([]backtestTradeResponse, len(trades))
	for i, t := range trades {
		result[i] = backtestTradeResponse{
			EntryTs:    t.EntryTimestamp.UTC().Format(time.RFC3339),
			ExitTs:     t.ExitTimestamp.UTC().Format(time.RFC3339),
			Side:       string(t.Side),
			Quantity:   t.Quantity,
			EntryPrice: t.EntryPrice,
			ExitPrice:  t.ExitPrice,
			Pnl:        t.PnL,
			Reason:     t.Reason,
			ExitReason: t.ExitReason,
		}
	}
	return result
}
