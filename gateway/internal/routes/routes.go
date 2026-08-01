package routes

import (
	"lexos-gateway/internal/handlers"

	"github.com/labstack/echo/v4"

	_ "lexos-gateway/docs"
	echoSwagger "github.com/swaggo/echo-swagger"
)

// Register wires up all the API endpoints
func Register(e *echo.Echo) {
	e.GET("/health", handlers.HealthCheck)

	// Swagger UI Route
	e.GET("/swagger/*", echoSwagger.WrapHandler)
	
	// Scriber Service Endpoint
	e.POST("/transcribe", handlers.HandleTranscriptionRequest)

	// Distiller Service Endpoint
	e.POST("/summarize", handlers.HandleSummarizationRequest)
	
	// Gleaner Service Indexing Endpoint
	e.POST("/glean/index", handlers.IndexDocument)

	// Gleaner Service QA Endpoint
	e.GET("/glean/ask", handlers.StreamQA)

	// Task Retrieval Endpoint
	e.GET("/task/:id", handlers.GetTaskState)
}