// tradePersist is the on-disk JSON shape of a single trade in backtest_runs.trades_json.
// It exists so models.Trade can stay free of json: tags (CLAUDE.md rule 20).
package storage

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// tradePersist is the storage-layer DTO for a single trade. PascalCase json: tags
// match the shape written by B14 and earlier (Go default capitalisation). Only this
// type — not models.Trade — may carry json: tags for the trades_json blob.
type tradePersist struct {
	EntryTimestamp time.Time        `json:"EntryTimestamp"`
	ExitTimestamp  time.Time        `json:"ExitTimestamp"`
	Side           models.OrderSide `json:"Side"`
	Quantity       int              `json:"Quantity"`
	EntryPrice     float64          `json:"EntryPrice"`
	ExitPrice      float64          `json:"ExitPrice"`
	GrossPnL       float64          `json:"GrossPnL"`
	Slippage       float64          `json:"Slippage"`
	Brokerage      float64          `json:"Brokerage"`
	STT            float64          `json:"STT"`
	ExchangeFees   float64          `json:"ExchangeFees"`
	SEBIFees       float64          `json:"SEBIFees"`
	GST            float64          `json:"GST"`
	StampDuty      float64          `json:"StampDuty"`
	TotalCharges   float64          `json:"TotalCharges"`
	NetPnL         float64          `json:"NetPnL"`
	Reason         string           `json:"Reason"`
	ExitReason     string           `json:"ExitReason"`
}

// UnmarshalJSON handles backward compatibility for pre-B14 trades_json blobs.
// Before B14, persisted trades used the legacy Trade.PnL field, producing objects
// with a "PnL" key. Post-B14 the field was split into GrossPnL/NetPnL/TotalCharges
// and "PnL" is never written again.
//
// When a legacy "PnL" key is present, it is mirrored to both NetPnL and GrossPnL
// and the charge breakdown stays at zero. Detection is unambiguous because current
// code cannot emit "PnL", so a non-nil legacy pointer always means a pre-B14 record.
func (t *tradePersist) UnmarshalJSON(data []byte) error {
	type alias tradePersist
	aux := struct {
		*alias
		LegacyPnL *float64 `json:"PnL"`
	}{alias: (*alias)(t)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.LegacyPnL != nil {
		t.NetPnL = *aux.LegacyPnL
		t.GrossPnL = *aux.LegacyPnL
	}
	return nil
}

func toTradePersist(tr models.Trade) tradePersist {
	return tradePersist{
		EntryTimestamp: tr.EntryTimestamp,
		ExitTimestamp:  tr.ExitTimestamp,
		Side:           tr.Side,
		Quantity:       tr.Quantity,
		EntryPrice:     tr.EntryPrice,
		ExitPrice:      tr.ExitPrice,
		GrossPnL:       tr.GrossPnL,
		Slippage:       tr.Slippage,
		Brokerage:      tr.Brokerage,
		STT:            tr.STT,
		ExchangeFees:   tr.ExchangeFees,
		SEBIFees:       tr.SEBIFees,
		GST:            tr.GST,
		StampDuty:      tr.StampDuty,
		TotalCharges:   tr.TotalCharges,
		NetPnL:         tr.NetPnL,
		Reason:         tr.Reason,
		ExitReason:     tr.ExitReason,
	}
}

func toTradeModel(tp tradePersist) models.Trade {
	return models.Trade{
		EntryTimestamp: tp.EntryTimestamp,
		ExitTimestamp:  tp.ExitTimestamp,
		Side:           tp.Side,
		Quantity:       tp.Quantity,
		EntryPrice:     tp.EntryPrice,
		ExitPrice:      tp.ExitPrice,
		GrossPnL:       tp.GrossPnL,
		Slippage:       tp.Slippage,
		Brokerage:      tp.Brokerage,
		STT:            tp.STT,
		ExchangeFees:   tp.ExchangeFees,
		SEBIFees:       tp.SEBIFees,
		GST:            tp.GST,
		StampDuty:      tp.StampDuty,
		TotalCharges:   tp.TotalCharges,
		NetPnL:         tp.NetPnL,
		Reason:         tp.Reason,
		ExitReason:     tp.ExitReason,
	}
}

// encodeTradesJSON maps each trade to tradePersist and marshals the slice to JSON.
func encodeTradesJSON(trades []models.Trade) ([]byte, error) {
	dtos := make([]tradePersist, len(trades))
	for i, tr := range trades {
		dtos[i] = toTradePersist(tr)
	}
	return json.Marshal(dtos)
}

// decodeTradesJSON unmarshals trades_json bytes into []models.Trade via the
// tradePersist DTO, tolerating both pre-B14 and post-B14 stored shapes.
func decodeTradesJSON(data []byte) ([]models.Trade, error) {
	var dtos []tradePersist
	if err := json.Unmarshal(data, &dtos); err != nil {
		return nil, fmt.Errorf("unmarshaling trades_json: %w", err)
	}
	trades := make([]models.Trade, len(dtos))
	for i, dto := range dtos {
		trades[i] = toTradeModel(dto)
	}
	return trades, nil
}
