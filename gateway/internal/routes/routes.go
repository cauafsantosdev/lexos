package routes

import (
	"lexos-gateway/internal/handlers"

	"github.com/labstack/echo/v4"
)

// Register wires up all the API endpoints
func Register(e *echo.Echo) {
	e.GET("/health", handlers.HealthCheck)
	
	// Faster-Whisper Service Endpoint
	e.POST("/transcribe", handlers.HandleTranscriptionRequest)

	// Task Retrieval Endpoint
	e.GET("/task/:id", handlers.GetTranscriptionResult)
}