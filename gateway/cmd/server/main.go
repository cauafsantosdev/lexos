package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func main() {
	// Initializing Echo
	e := echo.New()

	// Health Check
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "healthy",
			"message": "Lexos Gateway is running!",
		})
	})

	// Start server at port 8000
	e.Logger.Fatal(e.Start(":8000"))
}