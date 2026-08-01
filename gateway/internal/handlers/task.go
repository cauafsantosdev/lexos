package handlers

import (
	"fmt"
	"net/http"

	"lexos-gateway/internal/queue"

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
func GetTaskState(c echo.Context) error {
	taskID := c.Param("id")
	taskHashKey := fmt.Sprintf("task:%s", taskID)

	taskData, err := queue.Client.HGetAll(queue.Ctx, taskHashKey).Result()
	if err != nil || len(taskData) == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error":  "Task not found",
			"status": "missing",
		})
	}

	return c.JSON(http.StatusOK, taskData)
}