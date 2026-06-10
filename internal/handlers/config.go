package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	navTabRouteStrategies = "strategies"
	navTabRouteBacktest   = "backtest"
	navTabIconStrategies  = "strategies"
	navTabIconBacktest    = "backtest"
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
		{Route: navTabRouteStrategies, IconKey: navTabIconStrategies},
		{Route: navTabRouteBacktest, IconKey: navTabIconBacktest},
	}

	c.JSON(http.StatusOK, gin.H{
		"navTabs": tabs,
	})
}
