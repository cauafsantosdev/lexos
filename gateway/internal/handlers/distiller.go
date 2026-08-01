package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"lexos-gateway/internal/queue"
	"lexos-gateway/internal/storage"

	"github.com/labstack/echo/v4"
)

// SummarizeRequest represents the expected JSON body from the client
type SummarizeRequest struct {
	DocumentText string `json:"document_text"`
	Style        string `json:"style"`
}

// HandleSummarizationRequest godoc
// @Summary Submit a document for AI summarization
// @Description Uploads a text payload or a document (.txt, .pdf, .docx) to MinIO and queues it for Map-Reduce summarization by Qwen3.
// @Tags Distiller
// @Accept multipart/form-data
// @Accept json
// @Produce json
// @Param document formData file false "Document to summarize (if using multipart)"
// @Param style formData string false "Summary style (bullet_points, short_paragraph, executive)"
// @Param body body SummarizeRequest false "JSON payload (if using application/json)"
// @Success 202 {object} map[string]string "Task queued successfully"
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /summarize [post]
func HandleSummarizationRequest(c echo.Context) error {
	contentType := c.Request().Header.Get("Content-Type")
	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())
	
	payload := map[string]string{
		"task_id": taskID,
		"type":    "summarization",
	}

	taskState := map[string]interface{}{
		"task_id":    taskID,
		"status":     "queued",
		"type":       "summarization",
		"created_at": time.Now().Format(time.RFC3339),
	}

	// Handle Direct Text Payload
	if strings.HasPrefix(contentType, "application/json") {
		var req SummarizeRequest
		if err := c.Bind(&req); err != nil || req.DocumentText == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing 'document_text' in JSON"})
		}
		
		payload["document_text"] = req.DocumentText
		taskState["has_direct_text"] = true

		if req.Style != "" {
			payload["style"] = req.Style
		}

	// Handle File Upload (.txt, .pdf, or .docx)
	} else if strings.HasPrefix(contentType, "multipart/form-data") {
		fileHeader, err := c.FormFile("document")
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing 'document' file in form"})
		}
		
		file, err := fileHeader.Open()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to open uploaded file"})
		}
		defer file.Close()

		ext := filepath.Ext(fileHeader.Filename)
		s3Key := fmt.Sprintf("documents/%s%s", taskID, ext)

		// Stream directly to MinIO
		_, err = storage.UploadStream(s3Key, file, fileHeader.Size, fileHeader.Header.Get("Content-Type"))
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to stream document to MinIO"})
		}
		
		payload["s3_key"] = s3Key
		taskState["s3_key"] = s3Key

		style := c.FormValue("style")
		if style != "" {
			payload["style"] = style
		}
	} else {
		return c.JSON(http.StatusUnsupportedMediaType, map[string]string{
			"error": "Use application/json for text or multipart/form-data for files",
		})
	}

	// Set initial task state in Redis Hash
	taskHashKey := fmt.Sprintf("task:%s", taskID)
	queue.Client.HSet(queue.Ctx, taskHashKey, taskState)

	// Queue the task
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to encode task payload"})
	}

	err = queue.Client.RPush(queue.Ctx, "lexos:queue:summarization", payloadBytes).Err()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to queue task"})
	}

	return c.JSON(http.StatusAccepted, map[string]string{
		"message": "Document queued for summarization",
		"task_id": taskID,
		"status":  "queued",
	})
}