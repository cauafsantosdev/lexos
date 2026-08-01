package handlers

import (
	"net/http"
	
	"github.com/labstack/echo/v4"
)

// HealthCheck godoc
// @Summary Check API health
// @Description Returns the current status of the Lexos Gateway.
// @Tags Core
// @Produce json
// @Success 200 {object} map[string]string "API is healthy"
// @Router /health [get]
func HealthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok", "service": "lexos-gateway"})
}