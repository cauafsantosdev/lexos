package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"path/filepath"

	"lexos-gateway/internal/queue"
	"lexos-gateway/internal/storage"

	"github.com/labstack/echo/v4"
)

// IndexDocument handles uploading a file, streaming it to MinIO, and queuing the indexing task
func IndexDocument(c echo.Context) error {
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
	_, err = storage.UploadStream(s3Key, file, fileHeader.Size, fileHeader.Header.Get("Content-Type"))
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
	queue.Client.HSet(queue.Ctx, taskHashKey, taskState)

	// Push task to queue
	payload, _ := json.Marshal(map[string]string{
		"task_id": taskID,
		"s3_key":  s3Key,
		"type":    "indexing",
	})
	queue.Client.RPush(queue.Ctx, "lexos:queue:gleaner:index", payload)

	return c.JSON(http.StatusAccepted, map[string]string{
		"message":     "Document uploaded to MinIO and queued for indexing",
		"document_id": taskID,
		"status":      "queued",
	})
}

// StreamQA handles user questions, triggers the QA queue, and streams the response via SSE
func StreamQA(c echo.Context) error {
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
	queue.Client.HSet(queue.Ctx, taskHashKey, taskState)

	if err := queue.Client.RPush(queue.Ctx, "lexos:queue:gleaner:ask", payload).Err(); err != nil {
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
	pubsub := queue.Client.Subscribe(queue.Ctx, streamChannel)
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