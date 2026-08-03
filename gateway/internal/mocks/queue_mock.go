package mocks

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/mock"
)

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
	return redis.NewMapStringStringResult(args.Get(0).(map[string]string), args.Error(1))
}

func (m *MockQueueClient) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	args := m.Called(ctx, channels)
	if args.Get(0) != nil {
		return args.Get(0).(*redis.PubSub)
	}
	return nil
}