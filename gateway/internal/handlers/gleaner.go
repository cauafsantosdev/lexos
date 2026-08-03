package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"path/filepath"

	"github.com/labstack/echo/v4"
)

// IndexDocument godoc
// @Summary Upload document for vector indexing
// @Description Streams a document to MinIO, chunks it, embeds it via FastEmbed, and builds a FAISS index in the background.
// @Tags Gleaner
// @Accept multipart/form-data
// @Produce json
// @Param document formData file true "Document to index (.txt, .pdf, .docx)"
// @Success 202 {object} map[string]string "Document queued for indexing"
// @Failure 400 {object} map[string]string "Missing document file"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /glean/index [post]
func (api *API) IndexDocument(c echo.Context) error {
	// Parse the uploaded file from the form
	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing 'document' file"})
	}

	// Open the uploaded file for streaming
	file, err := fileHeader.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to open uploaded file"})
	}
	defer file.Close()

	// Generate a unique task ID and S3 key for the uploaded document
	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())
	ext := filepath.Ext(fileHeader.Filename)
	s3Key := fmt.Sprintf("documents/%s%s", taskID, ext)

	// Stream the file directly to MinIO without saving it to local disk
	_, err = api.Storage.UploadStream(s3Key, file, fileHeader.Size, fileHeader.Header.Get("Content-Type"))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to stream document to MinIO"})
	}

	// Set initial task state in a Redis Hash
	taskHashKey := fmt.Sprintf("task:%s", taskID)
	taskState := map[string]interface{}{
		"task_id":    taskID,
		"status":     "queued",
		"type":       "indexing",
		"s3_key":     s3Key,
		"created_at": time.Now().Format(time.RFC3339),
	}
	api.Queue.HSet(context.Background(), taskHashKey, taskState)

	// Push task to queue
	payload, _ := json.Marshal(map[string]string{
		"task_id": taskID,
		"s3_key":  s3Key,
		"type":    "indexing",
	})
	api.Queue.RPush(context.Background(), "lexos:queue:gleaner:index", payload)

	return c.JSON(http.StatusAccepted, map[string]string{
		"message":     "Document uploaded to MinIO and queued for indexing",
		"document_id": taskID,
		"status":      "queued",
	})
}

// StreamQA godoc
// @Summary Ask a question against an indexed document
// @Description Submits a query against a previously indexed document and streams the AI's answer token-by-token via Server-Sent Events (SSE).
// @Tags Gleaner
// @Produce text/event-stream
// @Param document_id query string true "The task_id of the previously indexed document"
// @Param query query string true "The user's question"
// @Success 200 {string} string "SSE Stream"
// @Failure 400 {object} map[string]string "Missing parameters"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /glean/ask [get]
func (api *API) StreamQA(c echo.Context) error {
	documentID := c.QueryParam("document_id")
	query := c.QueryParam("query")

	if documentID == "" || query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "document_id and query are required parameters"})
	}

	taskID := fmt.Sprintf("stream_%d", time.Now().UnixNano())
	streamChannel := fmt.Sprintf("lexos:stream:%s", taskID)

	// Push task to the QA queue
	taskData := map[string]string{
		"task_id":     taskID,
		"document_id": documentID,
		"query":       query,
		"type":        "qa_stream",
	}
	payload, _ := json.Marshal(taskData)

	// Set initial task state in Redis Hash for architectural consistency
	taskHashKey := fmt.Sprintf("task:%s", taskID)
	taskState := map[string]interface{}{
		"task_id":     taskID,
		"status":      "queued",
		"type":        "qa_stream",
		"document_id": documentID,
		"created_at":  time.Now().Format(time.RFC3339),
	}
	api.Queue.HSet(context.Background(), taskHashKey, taskState)

	if err := api.Queue.RPush(context.Background(), "lexos:queue:gleaner:ask", payload).Err(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to queue QA task"})
	}

	// Prepare headers for Server-Sent Events (SSE)
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Transfer-Encoding", "chunked")
	
	// Send 200 OK before starting the stream
	c.Response().WriteHeader(http.StatusOK)

	// Subscribe to Redis Pub/Sub
	pubsub := api.Queue.Subscribe(context.Background(), streamChannel)
	defer pubsub.Close()
	
	ch := pubsub.Channel()
	clientCtx := c.Request().Context()

	// Stream response to client
	for {
		select {
		case <-clientCtx.Done():
			// The client closed the connection (closed tab/canceled request)
			return nil 
		case msg := <-ch:
			if msg.Payload == "[DONE]" {
				fmt.Fprintf(c.Response(), "data: [DONE]\n\n")
				c.Response().Flush()
				return nil // Generation complete, end the HTTP request cleanly
			}

			// Write the SSE data format
			fmt.Fprintf(c.Response(), "data: %s\n\n", msg.Payload)
			
			// Flush pushes the data down the TCP socket immediately
			c.Response().Flush()
		}
	}
}