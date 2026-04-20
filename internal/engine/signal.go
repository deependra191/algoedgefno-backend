package engine

import (
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// Signal is a trade entry recommendation produced by a strategy evaluator.
type Signal struct {
	Timestamp time.Time
	Side      models.OrderSide
	Price     float64
	Reason    string
}
