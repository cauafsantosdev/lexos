package main

// @title Lexos API
// @version 1.0
// @description The cloud-native API gateway for the Lexos AI document processing engine.
// @host localhost:8000
// @BasePath /

import (
	"log"
	"net/http"
	"os"
	"strings"

	"lexos-gateway/internal/queue"
	"lexos-gateway/internal/routes"
	"lexos-gateway/internal/storage"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"
)

// envList parses a comma-separated environment variable into a cleaned list.
func envList(name string, fallback []string) []string {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}

	values := make([]string, 0)
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value != "" {
			values = append(values, value)
		}
	}

	if len(values) == 0 {
		return fallback
	}

	return values
}

func main() {
	// Initialize Redis queue
	if err := queue.Init(); err != nil {
		log.Fatalf("Fatal: Could not connect to Redis: %v", err)
	}

	// Initialize S3-compatible object storage (MinIO in development, R2 in production)
	if err := storage.InitObjectStorage(); err != nil {
		log.Fatalf("Fatal: Could not connect to object storage: %v", err)
	}

	// Setup Echo
	e := echo.New()
	e.HideBanner = true

	// Configure IP extraction based on TRUST_PROXY environment variable
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TRUST_PROXY")), "true") {
		e.IPExtractor = echo.ExtractIPFromXFFHeader()
	} else {
		e.IPExtractor = echo.ExtractIPDirect()
	}

	// Protect upload endpoints at the application boundary as defense in depth
	maxRequestBodySize := strings.TrimSpace(os.Getenv("MAX_REQUEST_BODY_SIZE"))
	if maxRequestBodySize == "" {
		maxRequestBodySize = "50M"
	}
	e.Use(middleware.BodyLimit(maxRequestBodySize))

	// Restrict browser access to configured frontend origins.
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: envList(
			"CORS_ALLOWED_ORIGINS",
			[]string{"http://localhost:3000"},
		),
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
	}))

	// Standard Middleware
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus: true,
		LogURI:    true,
		LogMethod: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			log.Printf("URI: %v | Method: %v | Status: %v\n", v.URI, v.Method, v.Status)
			return nil
		},
	}))
	e.Use(middleware.Recover())

	// Global Rate Limiting. Protects the server from connection floods (20 req/sec)
	globalRateLimit := middleware.RateLimiterConfig{
		Skipper: middleware.DefaultSkipper,
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(
			middleware.RateLimiterMemoryStoreConfig{
				Rate:  rate.Limit(20),
				Burst: 50,
			},
		),
		IdentifierExtractor: func(ctx echo.Context) (string, error) {
			return ctx.RealIP(), nil
		},
		DenyHandler: func(context echo.Context, identifier string, err error) error {
			return context.JSON(http.StatusTooManyRequests, map[string]string{
				"error": "Server is receiving too many requests. Please slow down.",
			})
		},
	}
	e.Use(middleware.RateLimiterWithConfig(globalRateLimit))

	// Register routes
	routes.Register(e)

	// Start server at port 8000
	e.Logger.Fatal(e.Start(":8000"))
}