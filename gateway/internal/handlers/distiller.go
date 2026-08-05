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

// SummarizeRequest represents the expected JSON body from the client.
type SummarizeRequest struct {
	DocumentText string `json:"document_text"`
	Style        string `json:"style"`
}

// summarizationTaskPayload carries the immutable processing identity and artifact
// destinations required by the asynchronous Distiller worker.
type summarizationTaskPayload struct {
	TaskID       string `json:"task_id"`
	Type         string `json:"type"`
	S3Key        string `json:"s3_key,omitempty"`
	DocumentText string `json:"document_text,omitempty"`
	Style        string `json:"style"`
	ContentHash  string `json:"content_hash"`
	Fingerprint  string `json:"fingerprint"`
	CacheKey     string `json:"cache_key"`
	LockKey      string `json:"lock_key"`
	ResultS3Key  string `json:"result_s3_key"`
}

// normalizeSummaryStyle constrains request input to the summary styles supported by the Distiller prompt pipeline.
func normalizeSummaryStyle(style string) string {
	switch style {
	case "short_paragraph", "executive":
		return style
	default:
		return "bullet_points"
	}
}

// HandleSummarizationRequest godoc
// @Summary Submit a document for AI summarization
// @Description Uploads text or a document (.txt, .pdf, .docx) to the asynchronous Qwen3 Map-Reduce pipeline. Identical content reuses completed or in-flight work when the summary style matches.
// @Tags Distiller
// @Accept multipart/form-data
// @Accept json
// @Produce json
// @Param document formData file false "Document to summarize (if using multipart)"
// @Param style formData string false "Summary style (bullet_points, short_paragraph, executive)"
// @Param body body SummarizeRequest false "JSON payload (if using application/json)"
// @Success 200 {object} map[string]interface{} "Cached task reused successfully"
// @Success 202 {object} map[string]interface{} "Task queued successfully"
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /summarize [post]
func (api *API) HandleSummarizationRequest(c echo.Context) error {
	ctx := c.Request().Context()
	contentType := c.Request().Header.Get("Content-Type")
	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())

	var rawKey string
	var documentText string
	var contentHash string
	style := "bullet_points"
	sourceVariant := "direct_text"

	baseTaskState := map[string]interface{}{
		"task_id":    taskID,
		"type":       "summarization",
		"created_at": time.Now().UTC().Format(time.RFC3339),
	}

	// Handle direct text payloads without writing a raw object to object storage.
	if strings.HasPrefix(contentType, "application/json") {
		var req SummarizeRequest
		if err := c.Bind(&req); err != nil || strings.TrimSpace(req.DocumentText) == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing 'document_text' in JSON"})
		}

		documentText = req.DocumentText
		style = normalizeSummaryStyle(req.Style)
		contentHash = hashBytes([]byte(documentText))
		baseTaskState["has_direct_text"] = true
		baseTaskState["style"] = style
		// Handle physical document uploads while hashing the exact byte stream.
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

		ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
		sourceVariant = ext
		rawKey = fmt.Sprintf("raw/%s/source%s", taskID, ext)
		// TeeReader computes SHA-256 during upload without buffering the whole file in memory.
		hasher := sha256.New()
		reader := io.TeeReader(file, hasher)

		if _, err = api.Storage.UploadStream(rawKey, reader, fileHeader.Size, fileHeader.Header.Get("Content-Type")); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to stream document to object storage"})
		}

		contentHash = hex.EncodeToString(hasher.Sum(nil))
		style = normalizeSummaryStyle(c.FormValue("style"))
		baseTaskState["s3_key"] = rawKey
		baseTaskState["style"] = style
	} else {
		return c.JSON(http.StatusUnsupportedMediaType, map[string]string{
			"error": "Use application/json for text or multipart/form-data for files",
		})
	}

	// Summary style and source format are processing-relevant and therefore part of cache identity.
	fingerprint := processingFingerprint(operationDistiller, contentHash, style, sourceVariant)
	// Resolve completed or in-flight duplicates before enqueueing CPU-bound work.
	resolution, err := api.resolveOrRegisterProcessing(
		ctx,
		operationDistiller,
		fingerprint,
		contentHash,
		taskID,
		rawKey,
		baseTaskState,
	)
	if err != nil {
		api.removeRawObject(ctx, rawKey)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to register summarization task"})
	}

	if !resolution.ShouldEnqueue {
		statusCode := http.StatusAccepted
		message := "Identical summary is already being processed"
		if resolution.CacheHit {
			statusCode = http.StatusOK
			message = "Cached summary reused"
		}
		return c.JSON(statusCode, responseForResolution(message, resolution))
	}

	paths := artifactKeys(operationDistiller, fingerprint)
	payload := summarizationTaskPayload{
		TaskID:       taskID,
		Type:         "summarization",
		S3Key:        rawKey,
		DocumentText: documentText,
		Style:        style,
		ContentHash:  contentHash,
		Fingerprint:  fingerprint,
		CacheKey:     cacheHashKey(operationDistiller, fingerprint),
		LockKey:      lockKey(operationDistiller, fingerprint),
		ResultS3Key:  paths.ResultKey,
	}

	// Queue only the owner task selected by the fingerprint-scoped processing lock.
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		api.failRegisteredProcessing(ctx, operationDistiller, fingerprint, taskID, "Failed to encode task payload")
		api.removeRawObject(ctx, rawKey)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to encode task payload"})
	}

	if err = api.Queue.RPush(ctx, "lexos:queue:summarization", payloadBytes).Err(); err != nil {
		api.failRegisteredProcessing(ctx, operationDistiller, fingerprint, taskID, "Failed to queue task")
		api.removeRawObject(ctx, rawKey)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to queue task"})
	}

	return c.JSON(http.StatusAccepted, responseForResolution(
		"Document queued for summarization",
		resolution,
	))
}