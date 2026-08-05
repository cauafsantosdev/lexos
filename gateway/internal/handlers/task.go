package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

// GetTaskState godoc
// @Summary Get the status of an async task
// @Description Fetches the current processing state and result URL for a given task ID.
// @Tags Core
// @Accept json
// @Produce json
// @Param id path string true "Task ID"
// @Success 200 {object} map[string]interface{} "Task state details"
// @Failure 404 {object} map[string]string "Task not found"
// @Router /task/{id} [get]
func (api *API) GetTaskState(c echo.Context) error {
	taskID := c.Param("id")
	taskData, err := api.Queue.HGetAll(c.Request().Context(), taskHashKey(taskID)).Result()
	if err != nil || len(taskData) == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Task not found"})
	}

	// Expose only client-facing task fields; internal storage and lock keys remain private.
	response := map[string]interface{}{}
	for _, field := range []string{
		"task_id",
		"status",
		"type",
		"error",
		"source_task_id",
		"created_at",
		"updated_at",
	} {
		if value := taskData[field]; value != "" {
			response[field] = value
		}
	}

	// Rewrite the internal S3 URI to the gateway result proxy endpoint.
	if resultURL := taskData["result_url"]; resultURL != "" {
		response["result_url"] = fmt.Sprintf("/task/%s/result", taskID)
	}

	for _, field := range []string{"cache_hit", "deduplicated"} {
		if value, ok := taskData[field]; ok {
			if parsed, parseErr := strconv.ParseBool(value); parseErr == nil {
				response[field] = parsed
			}
		}
	}

	return c.JSON(http.StatusOK, response)
}

// GetTaskResult godoc
// @Summary Download task result
// @Description Securely proxies the resulting JSON artifact from private S3-compatible object storage.
// @Tags Core
// @Produce json
// @Param id path string true "Task ID"
// @Success 200 {object} interface{} "The raw JSON result"
// @Failure 404 {object} map[string]string "Result not found"
// @Router /task/{id}/result [get]
func (api *API) GetTaskResult(c echo.Context) error {
	ctx := c.Request().Context()
	taskID := c.Param("id")
	taskData, err := api.Queue.HGetAll(ctx, taskHashKey(taskID)).Result()
	if err != nil || len(taskData) == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Task not found"})
	}

	objectName := taskData["result_s3_key"]
	if objectName == "" {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Result file not available"})
	}

	// Validate object existence before opening the private storage stream.
	bucket := api.Storage.BucketName()
	exists, err := api.Storage.StatObject(ctx, bucket, objectName)
	if err != nil || !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Result file not found in storage"})
	}

	// Proxy the object body without exposing storage credentials or private bucket URLs.
	object, err := api.Storage.GetObject(ctx, bucket, objectName)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Result file not found in storage"})
	}
	defer object.Close()

	return c.Stream(http.StatusOK, "application/json", object)
}