package main

import (
	"log"

	"lexos-gateway/internal/queue"
	"lexos-gateway/internal/routes"
	"lexos-gateway/internal/storage"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	// Initialize Redis queue
	if err := queue.Init(); err != nil {
		log.Fatalf("Fatal: Could not connect to Redis: %v", err)
	}

	// Initialize MinIO storage
	if err := storage.InitMinIO(); err != nil {
		log.Fatalf("Fatal: Could not connect to MinIO: %v", err)
	}

	// Setup Echo
	e := echo.New()
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	// Register routes
	routes.Register(e)

	// Start server at port 8000
	e.Logger.Fatal(e.Start(":8000"))
}