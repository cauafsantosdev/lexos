package handlers_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"lexos-gateway/internal/handlers"
	"lexos-gateway/internal/mocks"
)

func transcriptionRequest(t *testing.T, content []byte) (*echo.Echo, *httptest.ResponseRecorder, echo.Context) {
	t.Helper()
	// Arrange: construct the HTTP request and infrastructure mocks.
	e := echo.New()
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("audio", "test_audio.wav")
	// Assert: verify the HTTP contract and expected infrastructure interactions.
	assert.NoError(t, err)
	_, err = part.Write(content)
	// Assert: verify the HTTP contract and expected infrastructure interactions.
	assert.NoError(t, err)
	assert.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/transcribe", body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	rec := httptest.NewRecorder()
	return e, rec, e.NewContext(req, rec)
}

// TestHandleTranscriptionRequest_Success verifies streamed audio upload and initial queue registration.
func TestHandleTranscriptionRequest_Success(t *testing.T) {
	_, rec, c := transcriptionRequest(t, []byte("fake audio data"))
	queue := new(mocks.MockQueueClient)
	storage := new(mocks.MockStorageClient)

	storage.On("UploadStream", mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "raw/task_") && strings.HasSuffix(key, "/source.wav")
	}), mock.Anything, mock.AnythingOfType("int64"), mock.Anything).Return("raw/task/source.wav", nil).Once()
	expectNewProcessingRegistration(queue, "scriber")
	queue.On("RPush", mock.Anything, "lexos:queue:transcription", mock.Anything).Return(nil).Once()

	// Act: execute the handler with mocked infrastructure.
	api := handlers.NewAPI(queue, storage)
	err := api.HandleTranscriptionRequest(c)

	// Assert: verify the HTTP contract and expected infrastructure interactions.
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"queued"`)
	assert.Contains(t, rec.Body.String(), `"cache_hit":false`)
	storage.AssertExpectations(t)
	queue.AssertExpectations(t)
}

// TestHandleTranscriptionRequest_CompletedCacheHitRemovesDuplicateRaw verifies cache reuse and redundant raw-object cleanup.
func TestHandleTranscriptionRequest_CompletedCacheHitRemovesDuplicateRaw(t *testing.T) {
	_, rec, c := transcriptionRequest(t, []byte("same audio"))
	queue := new(mocks.MockQueueClient)
	storage := new(mocks.MockStorageClient)

	storage.On("UploadStream", mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("int64"), mock.Anything).Return("raw/new/source.wav", nil).Once()
	queue.On("HGetAll", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "lexos:cache:scriber:")
	})).Return(map[string]string{
		"status":        "completed",
		"operation":     "scriber",
		"cache_key":     "lexos:cache:scriber:fingerprint",
		"fingerprint":   "fingerprint",
		"owner_task_id": "task_original",
		"result_s3_key": "cache/scriber/fingerprint/result.json",
	}, nil).Once()
	storage.On("StatObject", mock.Anything, "lexos-storage", "cache/scriber/fingerprint/result.json").Return(true, nil).Once()
	storage.On("RemoveObject", mock.Anything, "lexos-storage", mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "raw/task_")
	})).Return(nil).Once()
	queue.On("HSet", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "task:task_")
	}), mock.Anything).Return(nil).Once()
	queue.On("Expire", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "task:task_")
	}), mock.Anything).Return(nil).Once()

	// Act: execute the handler with mocked infrastructure.
	api := handlers.NewAPI(queue, storage)
	err := api.HandleTranscriptionRequest(c)

	// Assert: verify the HTTP contract and expected infrastructure interactions.
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"completed"`)
	assert.Contains(t, rec.Body.String(), `"cache_hit":true`)
	storage.AssertExpectations(t)
	queue.AssertExpectations(t)
}

// TestHandleTranscriptionRequest_MissingFile verifies early validation for missing multipart audio.
func TestHandleTranscriptionRequest_MissingFile(t *testing.T) {
	// Arrange: construct the HTTP request and infrastructure mocks.
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/transcribe", nil)
	req.Header.Set(echo.HeaderContentType, "multipart/form-data")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Act: execute validation without infrastructure dependencies.
	api := handlers.NewAPI(nil, nil)
	err := api.HandleTranscriptionRequest(c)

	// Assert: verify the HTTP contract and expected infrastructure interactions.
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "Missing 'audio' file")
}

// TestHandleTranscriptionRequest_InFlightDuplicateReusesOwnerAndRemovesRaw verifies concurrent duplicate suppression.
func TestHandleTranscriptionRequest_InFlightDuplicateReusesOwnerAndRemovesRaw(t *testing.T) {
	_, rec, c := transcriptionRequest(t, []byte("same in-flight audio"))
	queue := new(mocks.MockQueueClient)
	storage := new(mocks.MockStorageClient)

	storage.On("UploadStream", mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("int64"), mock.Anything).Return("raw/new/source.wav", nil).Once()
	queue.On("HGetAll", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "lexos:cache:scriber:")
	})).Return(map[string]string{
		"status":        "processing",
		"owner_task_id": "task_original",
	}, nil).Once()
	queue.On("Get", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "lexos:lock:scriber:")
	})).Return("task_original", nil).Once()
	queue.On("HGetAll", mock.Anything, "task:task_original").Return(map[string]string{
		"status": "processing",
	}, nil).Once()
	storage.On("RemoveObject", mock.Anything, "lexos-storage", mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "raw/task_")
	})).Return(nil).Once()

	// Act: execute the handler with mocked infrastructure.
	api := handlers.NewAPI(queue, storage)
	err := api.HandleTranscriptionRequest(c)

	// Assert: verify the HTTP contract and expected infrastructure interactions.
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Contains(t, rec.Body.String(), `"task_id":"task_original"`)
	assert.Contains(t, rec.Body.String(), `"deduplicated":true`)
	queue.AssertNotCalled(t, "RPush", mock.Anything, mock.Anything, mock.Anything)
	queue.AssertNotCalled(t, "SetNX", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	storage.AssertExpectations(t)
	queue.AssertExpectations(t)
}

// TestHandleTranscriptionRequest_StaleProcessingCacheIsReplaced verifies recovery after an expired processing lease.
func TestHandleTranscriptionRequest_StaleProcessingCacheIsReplaced(t *testing.T) {
	_, rec, c := transcriptionRequest(t, []byte("audio after crashed owner"))
	queue := new(mocks.MockQueueClient)
	storage := new(mocks.MockStorageClient)

	storage.On("UploadStream", mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("int64"), mock.Anything).Return("raw/new/source.wav", nil).Once()
	queue.On("HGetAll", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "lexos:cache:scriber:")
	})).Return(map[string]string{
		"status":        "processing",
		"owner_task_id": "task_crashed",
	}, nil).Once()
	queue.On("Get", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "lexos:lock:scriber:")
	})).Return("", redis.Nil).Once()
	queue.On("Del", mock.Anything, mock.Anything).Return(nil).Once()
	queue.On("SetNX", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "lexos:lock:scriber:")
	}), mock.Anything, mock.Anything).Return(true, nil).Once()
	queue.On("HSet", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "task:task_")
	}), mock.Anything).Return(nil).Once()
	queue.On("Expire", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "task:task_")
	}), mock.Anything).Return(nil).Once()
	queue.On("HSet", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "lexos:cache:scriber:")
	}), mock.Anything).Return(nil).Once()
	queue.On("Expire", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "lexos:cache:scriber:")
	}), mock.Anything).Return(nil).Once()
	queue.On("RPush", mock.Anything, "lexos:queue:transcription", mock.Anything).Return(nil).Once()

	// Act: execute the handler with mocked infrastructure.
	api := handlers.NewAPI(queue, storage)
	err := api.HandleTranscriptionRequest(c)

	// Assert: verify the HTTP contract and expected infrastructure interactions.
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"queued"`)
	assert.Contains(t, rec.Body.String(), `"deduplicated":false`)
	queue.AssertExpectations(t)
	storage.AssertExpectations(t)
}

// TestHandleTranscriptionRequest_QueueFailureRemovesRawObject verifies cleanup when dispatch fails after upload.
func TestHandleTranscriptionRequest_QueueFailureRemovesRawObject(t *testing.T) {
	_, rec, c := transcriptionRequest(t, []byte("audio that cannot be queued"))
	queue := new(mocks.MockQueueClient)
	storage := new(mocks.MockStorageClient)

	storage.On("UploadStream", mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("int64"), mock.Anything).Return("raw/new/source.wav", nil).Once()
	expectNewProcessingRegistration(queue, "scriber")
	queue.On("RPush", mock.Anything, "lexos:queue:transcription", mock.Anything).Return(assert.AnError).Once()
	queue.On("HSet", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "task:task_")
	}), mock.Anything).Return(nil).Once()
	queue.On("Expire", mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "task:task_")
	}), mock.Anything).Return(nil).Once()
	queue.On("Del", mock.Anything, mock.Anything).Return(nil).Once()
	storage.On("RemoveObject", mock.Anything, "lexos-storage", mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "raw/task_")
	})).Return(nil).Once()

	// Act: execute the handler with mocked infrastructure.
	api := handlers.NewAPI(queue, storage)
	err := api.HandleTranscriptionRequest(c)

	// Assert: verify the HTTP contract and expected infrastructure interactions.
	assert.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "Failed to push task to queue")
	storage.AssertExpectations(t)
	queue.AssertExpectations(t)
}