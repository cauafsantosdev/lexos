package mocks

import (
	"context"
	"io"

	"github.com/stretchr/testify/mock"
)

// MockStorageClient provides a testify-backed S3-compatible storage dependency.
type MockStorageClient struct {
	mock.Mock
}

func (m *MockStorageClient) UploadStream(key string, reader io.Reader, size int64, contentType string) (string, error) {
	args := m.Called(key, mock.Anything, size, contentType)
	return args.String(0), args.Error(1)
}

func (m *MockStorageClient) StatObject(ctx context.Context, bucket string, key string) (bool, error) {
	args := m.Called(ctx, bucket, key)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorageClient) GetObject(ctx context.Context, bucket string, key string) (io.ReadCloser, error) {
	args := m.Called(ctx, bucket, key)
	if args.Get(0) != nil {
		return args.Get(0).(io.ReadCloser), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockStorageClient) RemoveObject(ctx context.Context, bucket string, key string) error {
	args := m.Called(ctx, bucket, key)
	return args.Error(0)
}

func (m *MockStorageClient) BucketName() string {
	return "lexos-storage"
}