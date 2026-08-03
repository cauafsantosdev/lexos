package handlers_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"lexos-gateway/internal/handlers"
	"lexos-gateway/internal/mocks"
)

// TestIndexDocument_Success validates that documents are successfully uploaded
// to MinIO and queued for vector embedding.
func TestIndexDocument_Success(t *testing.T) {
	// Create a fake text file payload
	e := echo.New()
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("document", "rag_context.txt")
	part.Write([]byte("Knowledge base data..."))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/glean/index", body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Mock the infrastructure
	mockQueue := new(mocks.MockQueueClient)
	mockStorage := new(mocks.MockStorageClient)

	// Expect the storage to accept the file and return a path, and Redis to store the task state and queue it
	mockStorage.On("UploadStream", mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("int64"), mock.Anything).Return("documents/task_123.txt", nil)
	mockQueue.On("HSet", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil)
	mockQueue.On("RPush", mock.Anything, "lexos:queue:gleaner:index", mock.Anything).Return(nil)

	api := handlers.NewAPI(mockQueue, mockStorage)
	err := api.IndexDocument(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Contains(t, rec.Body.String(), "queued for indexing")
}

// TestStreamQA_MissingParams verifies the API correctly rejects QA requests
// if the client forgets to provide both the document ID and the query string.
func TestStreamQA_MissingParams(t *testing.T) {
	// Empty GET request with no query parameters
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/glean/ask", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	api := handlers.NewAPI(nil, nil)
	err := api.StreamQA(c)

	// Expect a 400 Bad Request
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "document_id and query are required")
}

// TestStreamQA_MissingQuery verifies the API correctly rejects requests
// if the document ID is provided, but the actual question is missing.
func TestStreamQA_MissingQuery(t *testing.T) {
	// GET request missing the 'query' parameter
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/glean/ask?document_id=task_123", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	api := handlers.NewAPI(nil, nil)
	err := api.StreamQA(c)

	// Expect a 400 Bad Request
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}