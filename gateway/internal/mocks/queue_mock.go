package mocks

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/mock"
)

// MockQueueClient provides a testify-backed implementation of handler Redis dependencies.
type MockQueueClient struct {
	mock.Mock
}

func (m *MockQueueClient) RPush(ctx context.Context, queue string, values ...interface{}) *redis.IntCmd {
	args := m.Called(ctx, queue, mock.Anything)
	return redis.NewIntResult(1, args.Error(0))
}

func (m *MockQueueClient) HSet(ctx context.Context, key string, values ...interface{}) *redis.IntCmd {
	args := m.Called(ctx, key, mock.Anything)
	return redis.NewIntResult(1, args.Error(0))
}

func (m *MockQueueClient) HGetAll(ctx context.Context, key string) *redis.MapStringStringCmd {
	args := m.Called(ctx, key)
	data := map[string]string{}
	if args.Get(0) != nil {
		data = args.Get(0).(map[string]string)
	}
	return redis.NewMapStringStringResult(data, args.Error(1))
}

func (m *MockQueueClient) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd {
	args := m.Called(ctx, key, value, expiration)
	return redis.NewBoolResult(args.Bool(0), args.Error(1))
}

func (m *MockQueueClient) Get(ctx context.Context, key string) *redis.StringCmd {
	args := m.Called(ctx, key)
	return redis.NewStringResult(args.String(0), args.Error(1))
}

func (m *MockQueueClient) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	args := m.Called(ctx, keys)
	return redis.NewIntResult(1, args.Error(0))
}

func (m *MockQueueClient) Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd {
	args := m.Called(ctx, key, expiration)
	return redis.NewBoolResult(true, args.Error(0))
}

func (m *MockQueueClient) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	args := m.Called(ctx, channels)
	if args.Get(0) != nil {
		return args.Get(0).(*redis.PubSub)
	}
	return nil
}