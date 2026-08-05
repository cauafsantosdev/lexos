package queue

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/redis/go-redis/v9"
)

var (
	Client *redis.Client
	Ctx    = context.Background()
)

// Init connects to Redis and verifies the connection with a ping.
func Init() error {
	rawURL := strings.TrimSpace(os.Getenv("REDIS_URL"))
	if rawURL == "" {
		rawURL = "redis:6379"
	}

	// Accept both complete Redis URLs and the historical host:port format.
	var options *redis.Options
	var err error
	if strings.Contains(rawURL, "://") {
		options, err = redis.ParseURL(rawURL)
		if err != nil {
			return fmt.Errorf("invalid REDIS_URL: %w", err)
		}
	} else {
		options = &redis.Options{Addr: rawURL}
	}

	Client = redis.NewClient(options)
	return Client.Ping(Ctx).Err()
}