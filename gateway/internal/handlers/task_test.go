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

// TestGetTaskState_CompletedWithProxyURL verifies private storage metadata is replaced by a gateway proxy URL.
func TestGetTaskState_CompletedWithProxyURL(t *testing.T) {
	// Arrange: construct the HTTP request and infrastructure mocks.
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/task/task_123", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("task_123")

	queue := new(mocks.MockQueueClient)
	queue.On("HGetAll", mock.Anything, "task:task_123").Return(map[string]string{
		"task_id":       "task_123",
		"status":        "completed",
		"result_url":    "s3://lexos-storage/cache/distiller/fingerprint/result.json",
		"result_s3_key": "cache/distiller/fingerprint/result.json",
		"cache_key":     "lexos:cache:distiller:fingerprint",
		"content_hash":  "secret-internal-hash",
	}, nil).Once()

	// Act: execute the handler with mocked infrastructure.
	api := handlers.NewAPI(queue, nil)
	err := api.GetTaskState(c)

	// Assert: verify the HTTP contract and expected infrastructure interactions.
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "/task/task_123/result")
	assert.NotContains(t, rec.Body.String(), "s3://")
	assert.NotContains(t, rec.Body.String(), "cache_key")
	assert.NotContains(t, rec.Body.String(), "content_hash")
}

// TestGetTaskState_NotFound verifies missing Redis task state returns HTTP 404.
func TestGetTaskState_NotFound(t *testing.T) {
	// Arrange: construct the HTTP request and infrastructure mocks.
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/task/task_999", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("task_999")

	queue := new(mocks.MockQueueClient)
	queue.On("HGetAll", mock.Anything, "task:task_999").Return(map[string]string{}, nil).Once()

	// Act: execute the handler with mocked infrastructure.
	api := handlers.NewAPI(queue, nil)
	err := api.GetTaskState(c)

	// Assert: verify the HTTP contract and expected infrastructure interactions.
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestGetTaskResult_Success verifies private JSON artifacts are streamed through the gateway.
func TestGetTaskResult_Success(t *testing.T) {
	// Arrange: construct the HTTP request and infrastructure mocks.
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/task/task_123/result", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("task_123")

	queue := new(mocks.MockQueueClient)
	queue.On("HGetAll", mock.Anything, "task:task_123").Return(map[string]string{
		"status":        "completed",
		"result_s3_key": "cache/distiller/fingerprint/result.json",
	}, nil).Once()

	storage := new(mocks.MockStorageClient)
	storage.On("StatObject", mock.Anything, "lexos-storage", "cache/distiller/fingerprint/result.json").Return(true, nil).Once()
	fakeStream := io.NopCloser(strings.NewReader(`{"summary": "This is a great result."}`))
	storage.On("GetObject", mock.Anything, "lexos-storage", "cache/distiller/fingerprint/result.json").Return(fakeStream, nil).Once()

	// Act: execute the handler with mocked infrastructure.
	api := handlers.NewAPI(queue, storage)
	err := api.GetTaskResult(c)

	// Assert: verify the HTTP contract and expected infrastructure interactions.
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Body.String(), "This is a great result.")
}

// TestGetTaskResult_NotFound verifies missing derived artifacts return HTTP 404.
func TestGetTaskResult_NotFound(t *testing.T) {
	// Arrange: construct the HTTP request and infrastructure mocks.
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/task/task_999/result", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("task_999")

	queue := new(mocks.MockQueueClient)
	queue.On("HGetAll", mock.Anything, "task:task_999").Return(map[string]string{
		"status":        "completed",
		"result_s3_key": "cache/distiller/missing/result.json",
	}, nil).Once()

	storage := new(mocks.MockStorageClient)
	storage.On("StatObject", mock.Anything, "lexos-storage", "cache/distiller/missing/result.json").Return(false, nil).Once()

	// Act: execute the handler with mocked infrastructure.
	api := handlers.NewAPI(queue, storage)
	err := api.GetTaskResult(c)

	// Assert: verify the HTTP contract and expected infrastructure interactions.
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "Result file not found")
}

// TestGetTaskState_ParsesCacheFlagsAsBooleans verifies Redis string flags become JSON booleans.
func TestGetTaskState_ParsesCacheFlagsAsBooleans(t *testing.T) {
	// Arrange: construct the HTTP request and infrastructure mocks.
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/task/task_cached", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("task_cached")

	queue := new(mocks.MockQueueClient)
	queue.On("HGetAll", mock.Anything, "task:task_cached").Return(map[string]string{
		"task_id":      "task_cached",
		"status":       "completed",
		"cache_hit":    "1",
		"deduplicated": "true",
	}, nil).Once()

	// Act: execute the handler with mocked infrastructure.
	api := handlers.NewAPI(queue, nil)
	err := api.GetTaskState(c)

	// Assert: verify the HTTP contract and expected infrastructure interactions.
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"cache_hit":true`)
	assert.Contains(t, rec.Body.String(), `"deduplicated":true`)
}