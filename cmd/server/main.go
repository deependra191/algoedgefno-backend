package main

import (
	"log"

	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/database"
	"github.com/deependra191/algoedgefno-backend/internal/middleware"
	"github.com/deependra191/algoedgefno-backend/internal/providers"
	"github.com/deependra191/algoedgefno-backend/internal/providers/nse"
	"github.com/deependra191/algoedgefno-backend/internal/providers/vendor"
	"github.com/deependra191/algoedgefno-backend/internal/routes"
	"github.com/deependra191/algoedgefno-backend/internal/storage"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	pool := database.Connect(cfg)
	defer pool.Close()

	instrumentStore := storage.NewInstrumentStore(pool)
	candleStore := storage.NewCandleStore(pool)

	registry := providers.NewRegistry()
	registry.Register(nse.NewEODProvider(instrumentStore, candleStore))
	registry.Register(vendor.NewStub())

	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(middleware.Logger())
	r.Use(gin.Recovery())
	r.Use(cors.Default())

	routes.Register(r, pool, cfg, registry)

	log.Printf("starting server on :%s (env=%s)", cfg.Port, cfg.Env)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
