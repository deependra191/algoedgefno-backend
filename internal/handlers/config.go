package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// NavTab is a single bottom-navigation entry delivered to the Android client.
type NavTab struct {
	Route   string `json:"route"`
	IconKey string `json:"iconKey"`
}

// AppConfig returns public, static pre-login configuration for Android.
// Dynamic or user-specific configuration must move behind authenticated routes.
func AppConfig(c *gin.Context) {
	tabs := []NavTab{
		{Route: "home", IconKey: "home"},
		{Route: "scanner", IconKey: "scanner"},
		{Route: "strategies", IconKey: "strategies"},
		{Route: "backtest", IconKey: "backtest"},
		{Route: "profile", IconKey: "profile"},
	}

	c.JSON(http.StatusOK, gin.H{
		"navTabs": tabs,
	})
}
