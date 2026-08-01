package routes

import (
	"net/http"
	"time"

	"lexos-gateway/internal/handlers"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"

	_ "lexos-gateway/docs"
	echoSwagger "github.com/swaggo/echo-swagger"
)

// Register wires up all the API endpoints
func Register(e *echo.Echo) {
	// AI Task Rate Limiter: 1 request every 15 seconds, max burst of 2. Prevents queue hoarding and server starvation
	aiLimiter := middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Skipper: middleware.DefaultSkipper,
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(
			middleware.RateLimiterMemoryStoreConfig{
				Rate:  rate.Every(15 * time.Second),
				Burst: 2,
			},
		),
		IdentifierExtractor: func(ctx echo.Context) (string, error) {
			return ctx.RealIP(), nil
		},
		DenyHandler: func(context echo.Context, identifier string, err error) error {
			return context.JSON(http.StatusTooManyRequests, map[string]string{
				"error": "AI processing limit reached. Please wait 15 seconds before submitting another document.",
			})
		},
	})

	e.GET("/health", handlers.HealthCheck)

	// Swagger UI Route
	e.GET("/swagger/*", echoSwagger.WrapHandler)
	
	// Scriber Service Endpoint
	e.POST("/transcribe", handlers.HandleTranscriptionRequest, aiLimiter)

	// Distiller Service Endpoint
	e.POST("/summarize", handlers.HandleSummarizationRequest, aiLimiter)
	
	// Gleaner Service Indexing Endpoint
	e.POST("/glean/index", handlers.IndexDocument, aiLimiter)

	// Gleaner Service QA Endpoint
	e.GET("/glean/ask", handlers.StreamQA)

	// Task Retrieval Endpoint
	e.GET("/task/:id", handlers.GetTaskState)
}