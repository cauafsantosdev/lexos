package handlers

import (
	"net/http"
	
	"github.com/labstack/echo/v4"
)

// HealthCheck provides a simple uptime status
func HealthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok", "service": "lexos-gateway"})
}