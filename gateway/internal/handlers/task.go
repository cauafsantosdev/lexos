package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

// GetTaskState godoc
// @Summary Get the status of an async task
// @Description Fetches the current processing state and result URLs for a given task ID.
// @Tags Core
// @Accept json
// @Produce json
// @Param id path string true "Task ID"
// @Success 200 {object} map[string]interface{} "Task state details"
// @Failure 404 {object} map[string]string "Task not found"
// @Router /task/{id} [get]
func (api *API) GetTaskState(c echo.Context) error {
	taskID := c.Param("id")
	taskHashKey := fmt.Sprintf("task:%s", taskID)

	taskData, err := api.Queue.HGetAll(context.Background(), taskHashKey).Result()
	if err != nil || len(taskData) == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Task not found"})
	}

	// Rewrite the internal s3:// URI to a public relative API endpoint
	if resultURL, exists := taskData["result_url"]; exists && resultURL != "" {
		taskData["result_url"] = fmt.Sprintf("/task/%s/result", taskID)
	}

	return c.JSON(http.StatusOK, taskData)
}

// GetTaskResult godoc
// @Summary Download task result
// @Description Securely proxies the resulting JSON artifact from private MinIO storage.
// @Tags Core
// @Produce json
// @Param id path string true "Task ID"
// @Success 200 {object} interface{} "The raw JSON result"
// @Failure 404 {object} map[string]string "Result not found"
// @Router /task/{id}/result [get]
func (api *API) GetTaskResult(c echo.Context) error {
	taskID := c.Param("id")
	objectName := fmt.Sprintf("results/%s.json", taskID)

	// 1. Verify the file actually exists in MinIO
	_, err := api.Storage.StatObject(context.Background(), "lexos-storage", objectName)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Result file not found in storage"})
	}

	// 2. Safely fetch and stream the object
	object, _ := api.Storage.GetObject(context.Background(), "lexos-storage", objectName)
	defer object.Close()

	return c.Stream(http.StatusOK, "application/json", object)
}