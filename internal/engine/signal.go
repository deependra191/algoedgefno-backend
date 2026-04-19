package engine

import (
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

type Signal struct {
	Timestamp time.Time
	Side      models.OrderSide
	Price     float64
	Reason    string
}
