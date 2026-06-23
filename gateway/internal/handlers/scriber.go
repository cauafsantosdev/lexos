package handlers

import (
	"os"
	"path/filepath"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"lexos-gateway/internal/queue"
	"lexos-gateway/internal/utils"

	"github.com/labstack/echo/v4"
)

type TaskPayload struct {
	TaskID   string `json:"task_id"`
	FilePath string `json:"file_path"`
	Type     string `json:"type"`
}

// HandleTranscriptionRequest validates the upload, saves it, and queues the task
func HandleTranscriptionRequest(c echo.Context) error {
	file, err := c.FormFile("audio")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing 'audio' file"})
	}

	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())

	// Call file saver util
	dstPath, err := utils.SaveUploadedFile(file, taskID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save file on disk"})
	}

	// Create and encode the payload
	payload := TaskPayload{
		TaskID:   taskID,
		FilePath: dstPath,
		Type:     "transcription",
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to encode task"})
	}

	// Enqueue the task
	err = queue.Client.RPush(queue.Ctx, "lexos:queue:transcription", payloadBytes).Err()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to push task to queue"})
	}

	return c.JSON(http.StatusAccepted, map[string]string{
		"message": "File received and queued for processing",
		"task_id": taskID,
		"status":  "pending",
	})
}

// GetTranscriptionResult fetches the processed JSON file based on the task ID.
func GetTranscriptionResult(c echo.Context) error {
	taskID := c.Param("id")
	
	// Construct the path to where Python saves the result
	resultPath := filepath.Join("/uploads", taskID+".json")

	// Check if the file exists on the disk
	if _, err := os.Stat(resultPath); os.IsNotExist(err) {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error":  "Result not found.",
			"detail": "The task is either still processing, or the ID is invalid.",
			"status": "pending_or_missing",
		})
	}

	return c.File(resultPath)
}