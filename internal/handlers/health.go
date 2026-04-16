package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const appVersion = "0.1.0"

func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"version": appVersion,
	})
}
