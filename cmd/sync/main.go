package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deependra191/algoedgefno-backend/internal/providers/nse"
	"github.com/deependra191/algoedgefno-backend/internal/storage"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://algoedge:algoedge@localhost:5432/algoedgefno?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ping: %v", err)
	}
	log.Println("connected to database")

	instrumentStore := storage.NewInstrumentStore(pool)
	candleStore := storage.NewCandleStore(pool)
	provider := nse.NewEODProvider(instrumentStore, candleStore)

	log.Println("syncing instruments from NSE bhavcopy...")
	instCount, err := provider.SyncInstruments(ctx)
	if err != nil {
		log.Fatalf("sync instruments: %v", err)
	}
	fmt.Printf("instruments synced: %d\n", instCount)

	log.Println("syncing candles from NSE bhavcopy...")
	candleCount, err := provider.SyncCandles(ctx)
	if err != nil {
		log.Fatalf("sync candles: %v", err)
	}
	fmt.Printf("candles synced: %d\n", candleCount)

	log.Println("done")
}
