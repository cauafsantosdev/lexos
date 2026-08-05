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

// gleanerIndexTaskPayload carries content-addressed index destinations and
// processing identity to the Python indexing worker.
type gleanerIndexTaskPayload struct {
	TaskID      string `json:"task_id"`
	S3Key       string `json:"s3_key"`
	Type        string `json:"type"`
	ContentHash string `json:"content_hash"`
	Fingerprint string `json:"fingerprint"`
	CacheKey    string `json:"cache_key"`
	LockKey     string `json:"lock_key"`
	ResultS3Key string `json:"result_s3_key"`
	ArtifactID  string `json:"artifact_id"`
	IndexS3Key  string `json:"index_s3_key"`
	MetaS3Key   string `json:"meta_s3_key"`
}

// IndexDocument godoc
// @Summary Upload document for vector indexing
// @Description Streams a document to S3-compatible storage, chunks it, embeds it through FastEmbed, and builds a FAISS index in the background. Identical documents reuse completed or in-flight indexes.
// @Tags Gleaner
// @Accept multipart/form-data
// @Produce json
// @Param document formData file true "Document to index (.txt, .pdf, .docx)"
// @Success 200 {object} map[string]interface{} "Cached index reused successfully"
// @Success 202 {object} map[string]interface{} "Document queued for indexing"
// @Failure 400 {object} map[string]string "Missing document file"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /glean/index [post]
func (api *API) IndexDocument(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse the uploaded document from the multipart request.
	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing 'document' file"})
	}

	// Open the multipart file as a stream; no gateway-local persistence is required.
	file, err := fileHeader.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to open uploaded file"})
	}
	defer file.Close()

	// Raw inputs remain task-scoped so their lifecycle is independent from reusable derived artifacts.
	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	rawKey := fmt.Sprintf("raw/%s/source%s", taskID, ext)
	// Hash the exact upload bytes while streaming the document to object storage.
	hasher := sha256.New()
	reader := io.TeeReader(file, hasher)

	if _, err = api.Storage.UploadStream(rawKey, reader, fileHeader.Size, fileHeader.Header.Get("Content-Type")); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to stream document to object storage"})
	}

	// File format is included because document extraction depends on the uploaded extension.
	contentHash := hex.EncodeToString(hasher.Sum(nil))
	fingerprint := processingFingerprint(operationGleaner, contentHash, ext)

	baseTaskState := map[string]interface{}{
		"task_id":    taskID,
		"status":     "queued",
		"type":       "indexing",
		"s3_key":     rawKey,
		"created_at": time.Now().UTC().Format(time.RFC3339),
	}

	// Reuse an existing index or attach to active work before enqueueing a new embedding job.
	resolution, err := api.resolveOrRegisterProcessing(
		ctx,
		operationGleaner,
		fingerprint,
		contentHash,
		taskID,
		rawKey,
		baseTaskState,
	)
	if err != nil {
		api.removeRawObject(ctx, rawKey)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to register indexing task"})
	}

	if !resolution.ShouldEnqueue {
		statusCode := http.StatusAccepted
		message := "Identical document is already being indexed"
		if resolution.CacheHit {
			statusCode = http.StatusOK
			message = "Cached document index reused"
		}
		response := responseForResolution(message, resolution)
		response["document_id"] = resolution.TaskID
		return c.JSON(statusCode, response)
	}

	paths := artifactKeys(operationGleaner, fingerprint)
	payload := gleanerIndexTaskPayload{
		TaskID:      taskID,
		S3Key:       rawKey,
		Type:        "indexing",
		ContentHash: contentHash,
		Fingerprint: fingerprint,
		CacheKey:    cacheHashKey(operationGleaner, fingerprint),
		LockKey:     lockKey(operationGleaner, fingerprint),
		ResultS3Key: paths.ResultKey,
		ArtifactID:  paths.ArtifactID,
		IndexS3Key:  paths.IndexKey,
		MetaS3Key:   paths.MetaKey,
	}

	// Push only cache misses that successfully acquired the fingerprint processing lock.
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		api.failRegisteredProcessing(ctx, operationGleaner, fingerprint, taskID, "Failed to encode task payload")
		api.removeRawObject(ctx, rawKey)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to encode task payload"})
	}

	if err = api.Queue.RPush(ctx, "lexos:queue:gleaner:index", payloadBytes).Err(); err != nil {
		api.failRegisteredProcessing(ctx, operationGleaner, fingerprint, taskID, "Failed to queue indexing task")
		api.removeRawObject(ctx, rawKey)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to queue indexing task"})
	}

	response := responseForResolution("Document stored and queued for indexing", resolution)
	response["document_id"] = taskID
	return c.JSON(http.StatusAccepted, response)
}

// StreamQA godoc
// @Summary Ask a question against an indexed document
// @Description Submits a query against a completed Gleaner indexing task and streams the grounded answer token-by-token through Server-Sent Events (SSE).
// @Tags Gleaner
// @Produce text/event-stream
// @Param document_id query string true "The task_id of the completed indexing task"
// @Param query query string true "Question to answer from the indexed document"
// @Success 200 {string} string "SSE Stream"
// @Failure 400 {object} map[string]string "Missing parameters"
// @Failure 404 {object} map[string]string "Indexed document not found"
// @Failure 409 {object} map[string]string "Document indexing not completed"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /glean/ask [get]
func (api *API) StreamQA(c echo.Context) error {
	ctx := c.Request().Context()
	documentID := c.QueryParam("document_id")
	queryText := c.QueryParam("query")

	if documentID == "" || queryText == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "document_id and query are required parameters"})
	}

	// Resolve the indexed artifact keys from the completed document task state.
	documentState, err := api.Queue.HGetAll(ctx, taskHashKey(documentID)).Result()
	if err != nil || len(documentState) == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Indexed document task not found"})
	}
	if documentState["status"] != "completed" {
		return c.JSON(http.StatusConflict, map[string]string{"error": "Document indexing is not completed"})
	}

	artifactID := documentState["artifact_id"]
	// Legacy index keys remain supported for tasks created before content-addressed artifacts.
	if artifactID == "" {
		artifactID = documentID
	}
	indexS3Key := documentState["index_s3_key"]
	if indexS3Key == "" {
		indexS3Key = fmt.Sprintf("indexes/%s.faiss", documentID)
	}
	metaS3Key := documentState["meta_s3_key"]
	if metaS3Key == "" {
		metaS3Key = fmt.Sprintf("indexes/%s_meta.json", documentID)
	}

	// QA streams are request-scoped and are not reused as processing-cache artifacts.
	taskID := fmt.Sprintf("stream_%d", time.Now().UnixNano())
	streamChannel := fmt.Sprintf("lexos:stream:%s", taskID)
	taskHash := taskHashKey(taskID)

	taskData := map[string]string{
		"task_id":      taskID,
		"document_id":  documentID,
		"artifact_id":  artifactID,
		"index_s3_key": indexS3Key,
		"meta_s3_key":  metaS3Key,
		"query":        queryText,
		"type":         "qa_stream",
	}
	payload, err := json.Marshal(taskData)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to encode QA task"})
	}

	taskState := map[string]interface{}{
		"task_id":     taskID,
		"status":      "queued",
		"type":        "qa_stream",
		"document_id": documentID,
		"artifact_id": artifactID,
		"created_at":  time.Now().UTC().Format(time.RFC3339),
	}
	// Persist task state before queue dispatch so status polling remains consistent.
	if err = api.setHashWithTTL(ctx, taskHash, taskState, taskTTL()); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to register QA task"})
	}

	// Subscription is established before queue dispatch so early worker tokens cannot be lost.
	pubsub := api.Queue.Subscribe(ctx, streamChannel)
	if pubsub == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to subscribe to answer stream"})
	}
	defer pubsub.Close()

	if err = api.Queue.RPush(ctx, "lexos:queue:gleaner:ask", payload).Err(); err != nil {
		_ = api.Queue.HSet(ctx, taskHash, map[string]interface{}{
			"status":     "failed",
			"error":      "Failed to queue QA task",
			"updated_at": time.Now().UTC().Format(time.RFC3339),
		}).Err()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to queue QA task"})
	}

	// Prepare the HTTP response for Server-Sent Events after queue dispatch succeeds.
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Transfer-Encoding", "chunked")
	c.Response().WriteHeader(http.StatusOK)

	// Relay Redis Pub/Sub messages until generation completes or the client disconnects.
	ch := pubsub.Channel()
	clientCtx := c.Request().Context()

	for {
		select {
		case <-clientCtx.Done():
			// Stop relay work immediately when the HTTP client closes the connection.
			return nil
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			if msg.Payload == "[DONE]" {
				// Forward the terminal sentinel before closing the HTTP stream.
				fmt.Fprint(c.Response(), "data: [DONE]\n\n")
				c.Response().Flush()
				return nil
			}

			// SSE framing requires each message to end with a blank line.
			fmt.Fprintf(c.Response(), "data: %s\n\n", msg.Payload)
			c.Response().Flush()
		}
	}
}