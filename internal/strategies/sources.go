package strategies

import "github.com/deependra191/algoedgefno-backend/internal/models"

type fixedSourceResolver struct {
	sources models.StrategySources
}

func (r fixedSourceResolver) Sources(_ models.StrategyParams) models.StrategySources {
	return r.sources
}

// indexSignalFuturesIndexTradeSources pairs clean index signals with continuous-futures execution.
func indexSignalFuturesIndexTradeSources() models.StrategySourceResolver {
	return fixedSourceResolver{
		sources: models.StrategySources{
			Signal: models.InstrumentSpec{Kind: models.InstrumentKindIndex},
			Trade:  models.InstrumentSpec{Kind: models.InstrumentKindFuturesIndexContinuous},
		},
	}
}
