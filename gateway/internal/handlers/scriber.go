package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// TaskPayload represents the queue payload shared by asynchronous worker tasks.
type TaskPayload struct {
	TaskID      string `json:"task_id"`
	S3Key       string `json:"s3_key,omitempty"`
	Type        string `json:"type"`
	ContentHash string `json:"content_hash,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	CacheKey    string `json:"cache_key,omitempty"`
	LockKey     string `json:"lock_key,omitempty"`
	ResultS3Key string `json:"result_s3_key,omitempty"`
	ArtifactID  string `json:"artifact_id,omitempty"`
	IndexS3Key  string `json:"index_s3_key,omitempty"`
	MetaS3Key   string `json:"meta_s3_key,omitempty"`
}

// HandleTranscriptionRequest godoc
// @Summary Submit audio for transcription
// @Description Uploads an audio file to S3-compatible object storage and queues it for transcription using Faster-Whisper. Identical inputs reuse completed or in-flight processing.
// @Tags Scriber
// @Accept multipart/form-data
// @Produce json
// @Param audio formData file true "Audio file to transcribe"
// @Success 200 {object} map[string]interface{} "Cached task reused successfully"
// @Success 202 {object} map[string]interface{} "Task queued successfully"
// @Failure 400 {object} map[string]string "Missing audio file"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /transcribe [post]
func (api *API) HandleTranscriptionRequest(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse the uploaded audio file from the multipart request.
	fileHeader, err := c.FormFile("audio")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing 'audio' file"})
	}

	// Open the audio as a stream to avoid gateway-local file persistence.
	file, err := fileHeader.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to open uploaded file"})
	}
	defer file.Close()

	// Keep raw uploads task-scoped while derived transcription results remain content-addressed.
	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	rawKey := fmt.Sprintf("raw/%s/source%s", taskID, ext)

	// Compute SHA-256 during the upload stream without reading the audio twice.
	hasher := sha256.New()
	reader := io.TeeReader(file, hasher)
	if _, err = api.Storage.UploadStream(rawKey, reader, fileHeader.Size, fileHeader.Header.Get("Content-Type")); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to stream audio to object storage"})
	}

	// File format is included because decoder behavior can depend on the uploaded container type.
	contentHash := hex.EncodeToString(hasher.Sum(nil))
	fingerprint := processingFingerprint(operationScriber, contentHash, ext)

	baseTaskState := map[string]interface{}{
		"task_id":    taskID,
		"type":       "transcription",
		"s3_key":     rawKey,
		"created_at": time.Now().UTC().Format(time.RFC3339),
	}

	// Resolve completed or active duplicates before queueing Faster-Whisper inference.
	resolution, err := api.resolveOrRegisterProcessing(
		ctx,
		operationScriber,
		fingerprint,
		contentHash,
		taskID,
		rawKey,
		baseTaskState,
	)
	if err != nil {
		api.removeRawObject(ctx, rawKey)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to register transcription task"})
	}

	if !resolution.ShouldEnqueue {
		statusCode := http.StatusAccepted
		message := "Identical audio is already being processed"
		if resolution.CacheHit {
			statusCode = http.StatusOK
			message = "Cached transcription reused"
		}
		return c.JSON(statusCode, responseForResolution(message, resolution))
	}

	paths := artifactKeys(operationScriber, fingerprint)
	payload := TaskPayload{
		TaskID:      taskID,
		S3Key:       rawKey,
		Type:        "transcription",
		ContentHash: contentHash,
		Fingerprint: fingerprint,
		CacheKey:    cacheHashKey(operationScriber, fingerprint),
		LockKey:     lockKey(operationScriber, fingerprint),
		ResultS3Key: paths.ResultKey,
	}

	// Encode and enqueue only the task that owns the fingerprint processing lock.
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		api.failRegisteredProcessing(ctx, operationScriber, fingerprint, taskID, "Failed to encode task payload")
		api.removeRawObject(ctx, rawKey)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to encode task payload"})
	}

	if err = api.Queue.RPush(ctx, "lexos:queue:transcription", payloadBytes).Err(); err != nil {
		api.failRegisteredProcessing(ctx, operationScriber, fingerprint, taskID, "Failed to push task to queue")
		api.removeRawObject(ctx, rawKey)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to push task to queue"})
	}

	return c.JSON(http.StatusAccepted, responseForResolution(
		"Audio received, stored, and queued for processing",
		resolution,
	))
}