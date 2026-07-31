package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"lexos-gateway/internal/queue"
	"lexos-gateway/internal/utils"

	"github.com/labstack/echo/v4"
)

// IndexDocument handles uploading a file, saving it, and queuing the indexing task
func IndexDocument(c echo.Context) error {
	file, err := c.FormFile("document")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing 'document' file"})
	}

	// Using the same timestamp-based ID strategy as Scriber/Distiller
	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())

	// Reusing your existing utility to save to the /uploads volume
	dstPath, err := utils.SaveUploadedFile(file, taskID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save document to disk"})
	}

	// Create and encode the payload
	payload := map[string]string{
		"task_id":   taskID,
		"file_path": dstPath,
		"type":      "indexing",
	}
	
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to encode task"})
	}

	// Enqueue the task using your existing Redis client
	err = queue.Client.RPush(queue.Ctx, "lexos:queue:gleaner:index", payloadBytes).Err()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to push task to queue"})
	}

	return c.JSON(http.StatusAccepted, map[string]string{
		"message":     "Document queued for indexing",
		"document_id": taskID,
		"status":      "pending",
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