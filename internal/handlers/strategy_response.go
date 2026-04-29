package handlers

import (
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/internal/services"
)

const (
	constraintMaxDate      = "maxDate"
	sectionLabelBuiltin    = "Strategies"
	sectionLabelCustom     = "My Strategies"
	placeholderTitleCustom = "Coming soon"
	placeholderDescCustom  = "Your saved strategies will appear here."
)

type strategySectionsResponse struct {
	Sections []strategySectionResponse `json:"sections"`
}

type strategySectionResponse struct {
	Key         string                     `json:"key"`
	Label       string                     `json:"label"`
	Placeholder *sectionPlaceholder        `json:"placeholder"`
	Strategies  []strategyListItemResponse `json:"strategies"`
}

type sectionPlaceholder struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type strategyListItemResponse struct {
	ID           string                       `json:"id"`
	Name         string                       `json:"name"`
	Category     string                       `json:"category"`
	Description  string                       `json:"description"`
	LastBacktest *lastBacktestSummaryResponse `json:"lastBacktest"`
}

type lastBacktestSummaryResponse struct {
	ID         string  `json:"id"`
	ReturnPct  float64 `json:"returnPct"`
	TradeCount int     `json:"tradeCount"`
	RanAt      string  `json:"ranAt"`
}

type strategyDetailResponse struct {
	ID           string                      `json:"id"`
	Name         string                      `json:"name"`
	Category     string                      `json:"category"`
	Description  string                      `json:"description"`
	Logic        []string                    `json:"logic"`
	LastBacktest *lastBacktestDetailResponse `json:"lastBacktest"`
	Inputs       []strategyInputResponse     `json:"inputs"`
}

type lastBacktestDetailResponse struct {
	ID         string  `json:"id"`
	ReturnPct  float64 `json:"returnPct"`
	WinRate    int     `json:"winRate"`
	TradeCount int     `json:"tradeCount"`
	RanAt      string  `json:"ranAt"`
}

type strategyInputResponse struct {
	Key          string         `json:"key"`
	Label        string         `json:"label"`
	Type         string         `json:"type"`
	Options      []string       `json:"options,omitempty"`
	Constraints  map[string]any `json:"constraints,omitempty"`
	DefaultValue any            `json:"defaultValue,omitempty"`
	DefaultFrom  string         `json:"defaultFrom,omitempty"`
	DefaultTo    string         `json:"defaultTo,omitempty"`
}

func toStrategySectionsResponse(sections []services.StrategySection) strategySectionsResponse {
	resp := strategySectionsResponse{
		Sections: make([]strategySectionResponse, len(sections)),
	}
	for i, s := range sections {
		resp.Sections[i] = toStrategySectionResponse(s)
	}
	return resp
}

func toStrategySectionResponse(s services.StrategySection) strategySectionResponse {
	resp := strategySectionResponse{
		Key:        s.Key,
		Label:      sectionLabel(s.Key),
		Strategies: make([]strategyListItemResponse, len(s.Strategies)),
	}
	resp.Placeholder = sectionPlaceholderFor(s.Key)
	for i, item := range s.Strategies {
		resp.Strategies[i] = toStrategyListItemResponse(item)
	}
	return resp
}

func sectionLabel(key string) string {
	switch key {
	case services.SectionKeyBuiltin:
		return sectionLabelBuiltin
	case services.SectionKeyCustom:
		return sectionLabelCustom
	default:
		return key
	}
}

func sectionPlaceholderFor(key string) *sectionPlaceholder {
	switch key {
	case services.SectionKeyCustom:
		return &sectionPlaceholder{
			Title:       placeholderTitleCustom,
			Description: placeholderDescCustom,
		}
	default:
		return nil
	}
}

func toStrategyListItemResponse(item services.StrategyListItem) strategyListItemResponse {
	resp := strategyListItemResponse{
		ID:          item.Strategy.ID,
		Name:        item.Strategy.Name,
		Category:    item.Strategy.Category,
		Description: item.Strategy.Description,
	}
	if item.LastBacktest != nil {
		resp.LastBacktest = toLastBacktestSummary(item.LastBacktest)
	}
	return resp
}

func toLastBacktestSummary(run *models.BacktestRun) *lastBacktestSummaryResponse {
	if run == nil {
		return nil
	}
	return &lastBacktestSummaryResponse{
		ID:         run.ID.String(),
		ReturnPct:  computeReturnPct(run),
		TradeCount: derefInt(run.TotalTrades),
		RanAt:      formatCompletedAt(run.CompletedAt),
	}
}

func toLastBacktestDetail(run *models.BacktestRun) *lastBacktestDetailResponse {
	if run == nil {
		return nil
	}
	return &lastBacktestDetailResponse{
		ID:         run.ID.String(),
		ReturnPct:  computeReturnPct(run),
		WinRate:    computeWinRate(run),
		TradeCount: derefInt(run.TotalTrades),
		RanAt:      formatCompletedAt(run.CompletedAt),
	}
}

func toStrategyDetailResponse(detail *services.StrategyDetail) strategyDetailResponse {
	resp := strategyDetailResponse{
		ID:          detail.Strategy.ID,
		Name:        detail.Strategy.Name,
		Category:    detail.Strategy.Category,
		Description: detail.Strategy.Description,
		Logic:       detail.Strategy.Logic,
		Inputs:      toStrategyInputResponses(detail.Strategy.Inputs, detail.MaxDate),
	}
	if detail.LastBacktest != nil {
		resp.LastBacktest = toLastBacktestDetail(detail.LastBacktest)
	}
	return resp
}

func toStrategyInputResponses(inputs []models.StrategyInput, maxDate time.Time) []strategyInputResponse {
	result := make([]strategyInputResponse, len(inputs))
	for i, inp := range inputs {
		constraints := inp.Constraints
		if inp.Type == models.InputTypeDateRange && !maxDate.IsZero() {
			constraints = copyConstraints(constraints)
			constraints[constraintMaxDate] = maxDate.Format(dateFormat)
		}
		result[i] = strategyInputResponse{
			Key:          inp.Key,
			Label:        inp.Label,
			Type:         inp.Type,
			Options:      inp.Options,
			Constraints:  constraints,
			DefaultValue: inp.DefaultValue,
			DefaultFrom:  inp.DefaultFrom,
			DefaultTo:    inp.DefaultTo,
		}
	}
	return result
}

func copyConstraints(orig map[string]any) map[string]any {
	cp := make(map[string]any, len(orig)+1)
	for k, v := range orig {
		cp[k] = v
	}
	return cp
}

func computeReturnPct(run *models.BacktestRun) float64 {
	if run.Capital == nil || *run.Capital == 0 || run.NetPnl == nil {
		return 0
	}
	return *run.NetPnl / *run.Capital * 100
}

func computeWinRate(run *models.BacktestRun) int {
	if run.TotalTrades == nil || *run.TotalTrades == 0 || run.WinCount == nil {
		return 0
	}
	return *run.WinCount * 100 / *run.TotalTrades
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func formatCompletedAt(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
