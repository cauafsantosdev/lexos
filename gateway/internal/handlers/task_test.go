package handlers_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"lexos-gateway/internal/handlers"
	"lexos-gateway/internal/mocks"
)

// TestGetTaskState_CompletedWithProxyURL verifies that when a task is finished,
// the internal MinIO S3 URI is properly scrubbed and replaced with a public API route.
func TestGetTaskState_CompletedWithProxyURL(t *testing.T) {
	// Request the state of task_123
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/task/task_123", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("task_123")

	// Simulate Redis returning a completed task containing an internal S3 URL
	mockQueue := new(mocks.MockQueueClient)
	mockData := map[string]string{
		"status":     "completed",
		"result_url": "s3://lexos-storage/results/task_123.json",
	}
	mockQueue.On("HGetAll", mock.Anything, "task:task_123").Return(mockData, nil)

	api := handlers.NewAPI(mockQueue, nil)
	err := api.GetTaskState(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	
	// Ensure the s3:// URI was rewritten to the local proxy endpoint so the frontend can access it
	assert.Contains(t, rec.Body.String(), "/task/task_123/result")
	assert.NotContains(t, rec.Body.String(), "s3://")
}

// TestGetTaskState_NotFound ensures a 404 is returned if a task hash doesn't exist in Redis.
func TestGetTaskState_NotFound(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/task/task_999", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("task_999")

	// Simulate Redis returning an empty map (key not found)
	mockQueue := new(mocks.MockQueueClient)
	mockQueue.On("HGetAll", mock.Anything, "task:task_999").Return(map[string]string{}, nil)

	api := handlers.NewAPI(mockQueue, nil)
	err := api.GetTaskState(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "Task not found")
}

// TestGetTaskResult_Success verifies that the proxy endpoint successfully
// retrieves and streams the JSON artifact from MinIO to the client.
func TestGetTaskResult_Success(t *testing.T) {
	// Request the result artifact for task_123
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/task/task_123/result", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("task_123")

	mockStorage := new(mocks.MockStorageClient)
	
	// Simulate the file existing in MinIO
	mockStorage.On("StatObject", mock.Anything, "lexos-storage", "results/task_123.json").Return(true, nil)
	
	// Simulate the actual file stream returned by MinIO
	fakeJsonStream := io.NopCloser(strings.NewReader(`{"summary": "This is a great result."}`))
	mockStorage.On("GetObject", mock.Anything, "lexos-storage", "results/task_123.json").Return(fakeJsonStream, nil)

	api := handlers.NewAPI(nil, mockStorage)
	err := api.GetTaskResult(c)

	// Verify the JSON payload is successfully proxied to the response body
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Body.String(), "This is a great result.")
}

// TestGetTaskResult_NotFound handles the edge case where a task exists in Redis,
// but the worker hasn't finished uploading the result file to MinIO yet.
func TestGetTaskResult_NotFound(t *testing.T) {
	// ARRANGE
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/task/task_999/result", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("task_999")

	mockStorage := new(mocks.MockStorageClient)
	
	// Simulate MinIO throwing an error because the file doesn't exist
	mockStorage.On("StatObject", mock.Anything, "lexos-storage", "results/task_999.json").Return(false, assert.AnError)

	api := handlers.NewAPI(nil, mockStorage)
	err := api.GetTaskResult(c)

	// Expect a 404
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "Result file not found")
}