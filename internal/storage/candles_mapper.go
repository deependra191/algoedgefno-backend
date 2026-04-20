package storage

import "github.com/deependra191/algoedgefno-backend/internal/entities"
import "github.com/deependra191/algoedgefno-backend/internal/models"

func toCandleModel(e *entities.Candle) models.Candle {
	return models.Candle{
		InstrumentID: e.InstrumentID,
		Timestamp:    e.Timestamp,
		Interval:     e.Interval,
		Open:         e.Open,
		High:         e.High,
		Low:          e.Low,
		Close:        e.Close,
		Volume:       e.Volume,
		Provider:     e.Provider,
	}
}
