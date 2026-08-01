package handlers

import (
	"path/filepath"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"lexos-gateway/internal/queue"
	"lexos-gateway/internal/storage"

	"github.com/labstack/echo/v4"
)

type TaskPayload struct {
	TaskID   string `json:"task_id"`
	S3Key    string `json:"s3_key"`
	Type     string `json:"type"`
}

// HandleTranscriptionRequest validates the upload, streams it to MinIO, and queues the task
func HandleTranscriptionRequest(c echo.Context) error {
	// Parse the uploaded audio file from the form
	fileHeader, err := c.FormFile("audio")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing 'audio' file"})
	}

	// Open the uploaded file for streaming
	file, err := fileHeader.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to open uploaded file"})
	}
	defer file.Close()

	// Generate a unique task ID and S3 key for the uploaded audio
	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())
	ext := filepath.Ext(fileHeader.Filename)
	s3Key := fmt.Sprintf("audio/%s%s", taskID, ext)

	// Stream the file directly to MinIO without saving it to local disk
	_, err = storage.UploadStream(s3Key, file, fileHeader.Size, fileHeader.Header.Get("Content-Type"))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to stream audio to MinIO"})
	}

	// Set initial task state in Redis Hash
	taskHashKey := fmt.Sprintf("task:%s", taskID)
	taskState := map[string]interface{}{
		"task_id":    taskID,
		"status":     "queued",
		"type":       "transcription",
		"s3_key":     s3Key,
		"created_at": time.Now().Format(time.RFC3339),
	}
	queue.Client.HSet(queue.Ctx, taskHashKey, taskState)

	// Create and encode the payload
	payload := TaskPayload{
		TaskID: taskID,
		S3Key:  s3Key,
		Type:   "transcription",
	}
	
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to encode task payload"})
	}

	// Enqueue the task
	err = queue.Client.RPush(queue.Ctx, "lexos:queue:transcription", payloadBytes).Err()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to push task to queue"})
	}

	return c.JSON(http.StatusAccepted, map[string]string{
		"message": "Audio received, uploaded to MinIO, and queued for processing",
		"task_id": taskID,
		"status":  "queued",
	})
}