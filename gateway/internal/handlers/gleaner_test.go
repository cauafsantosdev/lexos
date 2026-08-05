package handlers_test

import (
	"bytes"
	"mime/multipart"
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

// TestIndexDocument_Success verifies streamed upload, processing registration, and Gleaner queue dispatch.
func TestIndexDocument_Success(t *testing.T) {
	// Arrange: construct the HTTP request and infrastructure mocks.
	e := echo.New()
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("document", "rag_context.txt")
	_, _ = part.Write([]byte("Knowledge base data..."))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/glean/index", body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	queue := new(mocks.MockQueueClient)
	storage := new(mocks.MockStorageClient)
	storage.On("UploadStream", mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "raw/task_") && strings.HasSuffix(key, "/source.txt")
	}), mock.Anything, mock.AnythingOfType("int64"), mock.Anything).Return("raw/task/source.txt", nil).Once()
	expectNewProcessingRegistration(queue, "gleaner")
	queue.On("RPush", mock.Anything, "lexos:queue:gleaner:index", mock.Anything).Return(nil).Once()

	// Act: execute the handler with mocked infrastructure.
	api := handlers.NewAPI(queue, storage)
	err := api.IndexDocument(c)

	// Assert: verify the HTTP contract and expected infrastructure interactions.
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Contains(t, rec.Body.String(), "queued for indexing")
	assert.Contains(t, rec.Body.String(), "document_id")
	storage.AssertExpectations(t)
	queue.AssertExpectations(t)
}

// TestIndexDocument_CompletedCacheHitValidatesAllArtifacts verifies reuse only when result, index, and metadata objects exist.
func TestIndexDocument_CompletedCacheHitValidatesAllArtifacts(t *testing.T) {
	// Arrange: construct the HTTP request and infrastructure mocks.
	e := echo.New()
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("document", "rag_context.txt")
	_, _ = part.Write([]byte("Knowledge base data..."))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/glean/index", body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	queue := new(mocks.MockQueueClient)
	storage := new(mocks.MockStorageClient)
	storage.On("UploadStream", mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("int64"), mock.Anything).Return("raw/new/source.txt", nil).Once()
	queue.On("HGetAll", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "lexos:cache:gleaner:")
	})).Return(map[string]string{
		"status":        "completed",
		"operation":     "gleaner",
		"cache_key":     "lexos:cache:gleaner:fingerprint",
		"fingerprint":   "fingerprint",
		"owner_task_id": "task_original",
		"artifact_id":   "fingerprint",
		"result_s3_key": "cache/gleaner/fingerprint/result.json",
		"index_s3_key":  "cache/gleaner/fingerprint/index.faiss",
		"meta_s3_key":   "cache/gleaner/fingerprint/meta.json",
	}, nil).Once()
	storage.On("StatObject", mock.Anything, "lexos-storage", mock.AnythingOfType("string")).Return(true, nil).Times(3)
	storage.On("RemoveObject", mock.Anything, "lexos-storage", mock.AnythingOfType("string")).Return(nil).Once()
	queue.On("HSet", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "task:task_")
	}), mock.Anything).Return(nil).Once()
	queue.On("Expire", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "task:task_")
	}), mock.Anything).Return(nil).Once()

	// Act: execute the handler with mocked infrastructure.
	api := handlers.NewAPI(queue, storage)
	err := api.IndexDocument(c)

	// Assert: verify the HTTP contract and expected infrastructure interactions.
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"cache_hit":true`)
	queue.AssertNotCalled(t, "RPush", mock.Anything, mock.Anything, mock.Anything)
	storage.AssertExpectations(t)
	queue.AssertExpectations(t)
}

// TestStreamQA_MissingParams verifies that incomplete QA requests fail before infrastructure access.
func TestStreamQA_MissingParams(t *testing.T) {
	// Arrange: construct the HTTP request and infrastructure mocks.
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/glean/ask", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Act: execute validation without infrastructure dependencies.
	api := handlers.NewAPI(nil, nil)
	err := api.StreamQA(c)

	// Assert: verify the HTTP contract and expected infrastructure interactions.
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "document_id and query are required")
}

// TestStreamQA_MissingQuery verifies that a document identifier alone is insufficient for QA streaming.
func TestStreamQA_MissingQuery(t *testing.T) {
	// Arrange: construct the HTTP request and infrastructure mocks.
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/glean/ask?document_id=task_123", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Act: execute validation without infrastructure dependencies.
	api := handlers.NewAPI(nil, nil)
	err := api.StreamQA(c)

	// Assert: verify the HTTP contract and expected infrastructure interactions.
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}