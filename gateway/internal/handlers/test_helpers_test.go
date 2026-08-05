package handlers_test

import (
	"strings"

	"github.com/stretchr/testify/mock"

	"lexos-gateway/internal/mocks"
)

func expectNewProcessingRegistration(queue *mocks.MockQueueClient, operation string) {
	queue.On(
		"HGetAll",
		mock.Anything,
		mock.MatchedBy(func(key string) bool {
			return strings.HasPrefix(key, "lexos:cache:"+operation+":")
		}),
	).Return(map[string]string{}, nil).Once()

	queue.On(
		"SetNX",
		mock.Anything,
		mock.MatchedBy(func(key string) bool {
			return strings.HasPrefix(key, "lexos:lock:"+operation+":")
		}),
		mock.Anything,
		mock.Anything,
	).Return(true, nil).Once()

	queue.On("HSet", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "task:task_")
	}), mock.Anything).Return(nil).Once()
	queue.On("Expire", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "task:task_")
	}), mock.Anything).Return(nil).Once()

	queue.On("HSet", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "lexos:cache:"+operation+":")
	}), mock.Anything).Return(nil).Once()
	queue.On("Expire", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "lexos:cache:"+operation+":")
	}), mock.Anything).Return(nil).Once()
}
