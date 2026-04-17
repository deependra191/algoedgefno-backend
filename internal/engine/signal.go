package engine

import "time"

type SignalSide string

const (
	SignalBuy  SignalSide = "BUY"
	SignalSell SignalSide = "SELL"
)

type Signal struct {
	Timestamp time.Time
	Side      SignalSide
	Price     float64
	Reason    string
}
