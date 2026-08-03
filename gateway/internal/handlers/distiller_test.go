package handlers_test

import (
	"bytes"
	"encoding/json"
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

// TestHandleSummarizationRequest_JSONPayload verifies that the Distiller 
// correctly processes direct text submissions via JSON.
func TestHandleSummarizationRequest_JSONPayload(t *testing.T) {
	// Setup the Echo context and HTTP request
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

	// Setup the mocks. Since this is JSON text, it shouldn't hit MinIO.
	mockQueue := new(mocks.MockQueueClient)
	mockStorage := new(mocks.MockStorageClient)

	// Expect Redis to save the task state and push the task to the summarization queue
	mockQueue.On("HSet", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil)
	mockQueue.On("RPush", mock.Anything, "lexos:queue:summarization", mock.Anything).Return(nil)

	// Inject mocks and trigger the handler
	api := handlers.NewAPI(mockQueue, mockStorage)
	err := api.HandleSummarizationRequest(c)

	// Verify the HTTP response and ensure our queue mocks were triggered
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Contains(t, rec.Body.String(), "task_id")
	mockQueue.AssertExpectations(t)
}

// TestHandleSummarizationRequest_MultipartPayload verifies that the Distiller 
// correctly processes physical file uploads and routes them to storage.
func TestHandleSummarizationRequest_MultipartPayload(t *testing.T) {
	// Construct a simulated multipart/form-data file upload
	e := echo.New()
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	
	// Create a fake PDF file in memory
	part, _ := writer.CreateFormFile("document", "thesis_draft.pdf")
	part.Write([]byte("fake pdf content"))
	writer.WriteField("style", "executive")
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/summarize", body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Setup the mocks
	mockQueue := new(mocks.MockQueueClient)
	mockStorage := new(mocks.MockStorageClient)

	// Expect Storage to receive the stream, and Redis to queue the task
	mockStorage.On("UploadStream", mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("int64"), mock.Anything).Return("documents/task_123.pdf", nil)
	mockQueue.On("HSet", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil)
	mockQueue.On("RPush", mock.Anything, "lexos:queue:summarization", mock.Anything).Return(nil)

	// Inject and execute
	api := handlers.NewAPI(mockQueue, mockStorage)
	err := api.HandleSummarizationRequest(c)

	// Verify both the Storage and Queue operations fired successfully
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, rec.Code)
	mockStorage.AssertExpectations(t)
	mockQueue.AssertExpectations(t)
}