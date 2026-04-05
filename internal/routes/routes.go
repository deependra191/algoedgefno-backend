package routes

import (
	"github.com/deependra191/algoedgefno-backend/internal/handlers"
	"github.com/deependra191/algoedgefno-backend/internal/middleware"
	"github.com/deependra191/algoedgefno-backend/internal/services"
	"github.com/gin-gonic/gin"
)

func Register(r *gin.Engine, authSvc *services.AuthService) {
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
		protected.Use(middleware.JWTAuth(authSvc))
		{
			protected.GET("/config/app", handlers.AppConfig)
		}
	}
}
