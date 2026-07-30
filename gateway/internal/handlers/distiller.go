package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"lexos-gateway/internal/queue"
	"lexos-gateway/internal/utils"

	"github.com/labstack/echo/v4"
)

// SummarizeRequest represents the expected JSON body from the client
type SummarizeRequest struct {
	DocumentText string `json:"document_text"`
	Style        string `json:"style"`
}

// HandleSummarizationRequest routes the request based on Content-Type
func HandleSummarizationRequest(c echo.Context) error {
	contentType := c.Request().Header.Get("Content-Type")
	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())

	payload := map[string]string{
		"task_id": taskID,
		"type":    "summarization",
	}

	// Handle Direct Text Payload
	if strings.HasPrefix(contentType, "application/json") {
		var req SummarizeRequest
		if err := c.Bind(&req); err != nil || req.DocumentText == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing 'document_text' in JSON"})
		}
		payload["document_text"] = req.DocumentText
		
		// Map the style if the user provided one
		if req.Style != "" {
			payload["style"] = req.Style
		}

	// Handle File Upload (.txt, .pdf, or .docx)
	} else if strings.HasPrefix(contentType, "multipart/form-data") {
		file, err := c.FormFile("document")
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing 'document' file in form"})
		}
		
		dstPath, err := utils.SaveUploadedFile(file, taskID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save document to disk"})
		}
		payload["file_path"] = dstPath

		// Map the style from the form data if the user provided one
		style := c.FormValue("style")
		if style != "" {
			payload["style"] = style
		}

	// Reject anything else
	} else {
		return c.JSON(http.StatusUnsupportedMediaType, map[string]string{
			"error": "Use application/json for text or multipart/form-data for files",
		})
	}

	// Queue the task
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to encode task"})
	}

	err = queue.Client.RPush(queue.Ctx, "lexos:queue:summarization", payloadBytes).Err()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to queue task"})
	}

	return c.JSON(http.StatusAccepted, map[string]string{
		"message": "Document queued for summarization",
		"task_id": taskID,
		"status":  "pending",
	})
}