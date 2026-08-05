package handlers_test

import (
	"bytes"
	"encoding/json"
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

// TestHandleSummarizationRequest_JSONPayload verifies direct-text submissions and queue dispatch.
func TestHandleSummarizationRequest_JSONPayload(t *testing.T) {
	// Arrange: construct the HTTP request and infrastructure mocks.
	e := echo.New()
	payload := handlers.SummarizeRequest{
		DocumentText: "Test document text for summarization.",
		Style:        "bullet_points",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/summarize", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	queue := new(mocks.MockQueueClient)
	storage := new(mocks.MockStorageClient)
	expectNewProcessingRegistration(queue, "distiller")
	queue.On("RPush", mock.Anything, "lexos:queue:summarization", mock.Anything).Return(nil).Once()

	// Act: execute the handler with mocked infrastructure.
	api := handlers.NewAPI(queue, storage)
	err := api.HandleSummarizationRequest(c)

	// Assert: verify the HTTP contract and expected infrastructure interactions.
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Contains(t, rec.Body.String(), "task_id")
	storage.AssertNotCalled(t, "UploadStream", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	queue.AssertExpectations(t)
}

// TestHandleSummarizationRequest_MultipartPayload verifies streamed document uploads and queue dispatch.
func TestHandleSummarizationRequest_MultipartPayload(t *testing.T) {
	// Arrange: construct the HTTP request and infrastructure mocks.
	e := echo.New()
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("document", "thesis_draft.pdf")
	_, _ = part.Write([]byte("fake pdf content"))
	_ = writer.WriteField("style", "executive")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/summarize", body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	queue := new(mocks.MockQueueClient)
	storage := new(mocks.MockStorageClient)
	storage.On("UploadStream", mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "raw/task_") && strings.HasSuffix(key, "/source.pdf")
	}), mock.Anything, mock.AnythingOfType("int64"), mock.Anything).Return("raw/task/source.pdf", nil).Once()
	expectNewProcessingRegistration(queue, "distiller")
	queue.On("RPush", mock.Anything, "lexos:queue:summarization", mock.Anything).Return(nil).Once()

	// Act: execute the handler with mocked infrastructure.
	api := handlers.NewAPI(queue, storage)
	err := api.HandleSummarizationRequest(c)

	// Assert: verify the HTTP contract and expected infrastructure interactions.
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, rec.Code)
	storage.AssertExpectations(t)
	queue.AssertExpectations(t)
}