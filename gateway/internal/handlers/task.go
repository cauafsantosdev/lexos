package handlers

import (
	"fmt"
	"net/http"

	"lexos-gateway/internal/queue"

	"github.com/labstack/echo/v4"
)

// GetTaskState fetches the live task state directly from Redis Hashes
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