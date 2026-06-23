package queue

import (
	"context"
	"os"

	"github.com/redis/go-redis/v9"
)

var (
	Client *redis.Client
	Ctx    = context.Background()
)

// Init connects to Redis and pings it to ensure the connection is alive
func Init() error {
	Client = redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_URL"),
	})

	return Client.Ping(Ctx).Err()
}