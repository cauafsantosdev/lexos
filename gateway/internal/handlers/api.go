package handlers

import (
	"context"
	"io"

	"github.com/redis/go-redis/v9"
)

// QueueService defines the Redis operations
type QueueService interface {
	RPush(ctx context.Context, queue string, values ...interface{}) *redis.IntCmd
	HSet(ctx context.Context, key string, values ...interface{}) *redis.IntCmd
	HGetAll(ctx context.Context, key string) *redis.MapStringStringCmd
	Subscribe(ctx context.Context, channels ...string) *redis.PubSub
}

// StorageService defines the MinIO operations
type StorageService interface {
	UploadStream(key string, reader io.Reader, size int64, contentType string) (string, error)
	StatObject(ctx context.Context, bucket string, key string) (bool, error)
	GetObject(ctx context.Context, bucket string, key string) (io.ReadCloser, error)
}

// API binds the dependencies to HTTP routes
type API struct {
	Queue   QueueService
	Storage StorageService
}

// NewAPI is a constructor for the API
func NewAPI(q QueueService, s StorageService) *API {
	return &API{
		Queue:   q,
		Storage: s,
	}
}