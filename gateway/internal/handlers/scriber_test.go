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

// TestHandleTranscriptionRequest_Success validates the audio upload pipeline,
// ensuring the file streams to MinIO and the job hits the correct Redis queue.
func TestHandleTranscriptionRequest_Success(t *testing.T) {
	// Construct a simulated .wav file upload
	e := echo.New()
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("audio", "test_audio.wav")
	part.Write([]byte("fake audio data"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/transcribe", body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Setup the Mocks
	mockQueue := new(mocks.MockQueueClient)
	mockStorage := new(mocks.MockStorageClient)

	// Expect the file to upload and the transcription task to queue
	mockStorage.On("UploadStream", mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("int64"), mock.Anything).Return("audio/task_123.wav", nil)
	mockQueue.On("HSet", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil)
	mockQueue.On("RPush", mock.Anything, "lexos:queue:transcription", mock.Anything).Return(nil)

	api := handlers.NewAPI(mockQueue, mockStorage)
	err := api.HandleTranscriptionRequest(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Contains(t, rec.Body.String(), "task_id")
	assert.Contains(t, rec.Body.String(), "queued")
	
	// Verify our infrastructure dependencies were actually invoked
	mockStorage.AssertExpectations(t)
	mockQueue.AssertExpectations(t)
}

// TestHandleTranscriptionRequest_MissingFile ensures the handler gracefully
// catches bad payloads where the client forgets to attach the audio file.
func TestHandleTranscriptionRequest_MissingFile(t *testing.T) {
	// Send an empty multipart payload
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/transcribe", nil)
	req.Header.Set(echo.HeaderContentType, "multipart/form-data")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Execute with nil dependencies since the logic should fail early
	api := handlers.NewAPI(nil, nil) 
	err := api.HandleTranscriptionRequest(c)

	// Expect a 400 Bad Request
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "Missing 'audio' file")
}