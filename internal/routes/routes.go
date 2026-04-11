package routes

import (
	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/handlers"
	"github.com/deependra191/algoedgefno-backend/internal/middleware"
	"github.com/deependra191/algoedgefno-backend/internal/providers"
	"github.com/deependra191/algoedgefno-backend/internal/repository"
	"github.com/deependra191/algoedgefno-backend/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Register(r *gin.Engine, pool *pgxpool.Pool, cfg *config.Config, registry *providers.Registry) {
	userRepo := repository.NewUserRepository(pool)
	authSvc := services.NewAuthService(userRepo, cfg.JWTSecret)
	authHandler := handlers.NewAuthHandler(authSvc)

	r.GET("/health", handlers.Health)

	v1 := r.Group("/api/v1")
	{
		auth := v1.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
		}

		protected := v1.Group("")
		protected.Use(middleware.Auth(cfg.AppSecretToken, authSvc))
		{
			protected.GET("/config/app", handlers.AppConfig)
		}
	}

	_ = registry // wired here; handlers added in B11
}
