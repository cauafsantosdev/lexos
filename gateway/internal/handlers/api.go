package handlers

import (
	"context"
	"io"
	"time"

	"github.com/redis/go-redis/v9"
)

// QueueService defines the Redis operations required by the HTTP handlers.
// The interface keeps handler tests isolated from a live Redis instance.
type QueueService interface {
	RPush(ctx context.Context, queue string, values ...interface{}) *redis.IntCmd
	HSet(ctx context.Context, key string, values ...interface{}) *redis.IntCmd
	HGetAll(ctx context.Context, key string) *redis.MapStringStringCmd
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd
	Get(ctx context.Context, key string) *redis.StringCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd
	Subscribe(ctx context.Context, channels ...string) *redis.PubSub
}

// StorageService defines the S3-compatible object storage operations required by the gateway.
// MinIO in development and Cloudflare R2 in production share this contract.
type StorageService interface {
	UploadStream(key string, reader io.Reader, size int64, contentType string) (string, error)
	StatObject(ctx context.Context, bucket string, key string) (bool, error)
	GetObject(ctx context.Context, bucket string, key string) (io.ReadCloser, error)
	RemoveObject(ctx context.Context, bucket string, key string) error
	BucketName() string
}

// API binds queue and storage dependencies to HTTP routes.
type API struct {
	Queue   QueueService
	Storage StorageService
}

// NewAPI constructs an API instance with queue and storage dependencies.
func NewAPI(q QueueService, s StorageService) *API {
	return &API{
		Queue:   q,
		Storage: s,
	}
}