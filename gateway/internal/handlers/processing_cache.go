package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Processing operation identifiers namespace Redis cache keys, lock keys, and artifact paths.
const (
	operationScriber   = "scriber"
	operationDistiller = "distiller"
	operationGleaner   = "gleaner"
)

// artifactPaths defines stable content-addressed object keys for derived artifacts.
type artifactPaths struct {
	ResultKey  string
	IndexKey   string
	MetaKey    string
	ArtifactID string
}

// processingResolution describes whether a request owns new work or reuses an
// existing completed/in-flight computation.
type processingResolution struct {
	ShouldEnqueue bool
	TaskID        string
	Status        string
	CacheHit      bool
	Deduplicated  bool
}

// hashBytes returns the lowercase SHA-256 digest used by processing fingerprints.
func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// processingFingerprint derives a deterministic cache identity from the operation,
// exact source content hash, and processing-relevant request parameters.
func processingFingerprint(operation string, contentHash string, variants ...string) string {
	parts := []string{"lexos", operation, contentHash}
	parts = append(parts, variants...)
	return hashBytes([]byte(strings.Join(parts, "|")))
}

// artifactKeys maps a processing fingerprint to stable derived-object locations.
func artifactKeys(operation string, fingerprint string) artifactPaths {
	base := fmt.Sprintf("cache/%s/%s", operation, fingerprint)
	paths := artifactPaths{
		ResultKey: fmt.Sprintf("%s/result.json", base),
	}

	if operation == operationGleaner {
		paths.ArtifactID = fingerprint
		paths.IndexKey = fmt.Sprintf("%s/index.faiss", base)
		paths.MetaKey = fmt.Sprintf("%s/meta.json", base)
	}

	return paths
}

// cacheHashKey returns the Redis hash that records reusable processing metadata.
func cacheHashKey(operation string, fingerprint string) string {
	return fmt.Sprintf("lexos:cache:%s:%s", operation, fingerprint)
}

// lockKey returns the fingerprint-scoped Redis lease used for duplicate suppression.
func lockKey(operation string, fingerprint string) string {
	return fmt.Sprintf("lexos:lock:%s:%s", operation, fingerprint)
}

// taskHashKey returns the Redis hash key for client-visible asynchronous task state.
func taskHashKey(taskID string) string {
	return fmt.Sprintf("task:%s", taskID)
}

// cacheTTL controls how long completed derived artifacts remain reusable through Redis metadata.
func cacheTTL() time.Duration {
	return envSeconds("CACHE_TTL_SECONDS", 7*24*60*60)
}

// taskTTL controls how long client-facing task aliases remain queryable.
func taskTTL() time.Duration {
	return envSeconds("TASK_TTL_SECONDS", 24*60*60)
}

// processingLockTTL bounds orphaned processing leases after worker or gateway failure.
func processingLockTTL() time.Duration {
	return envSeconds("PROCESSING_LOCK_TTL_SECONDS", 60*60)
}

// getenvDefault resolves a non-empty environment value or returns the supplied fallback.
func getenvDefault(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

// envSeconds resolves positive second-based TTL configuration with a safe fallback.
func envSeconds(name string, fallback int) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return time.Duration(fallback) * time.Second
	}

	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return time.Duration(fallback) * time.Second
	}

	return time.Duration(seconds) * time.Second
}

// cloneState copies a task-state map before cache-specific fields are added.
func cloneState(source map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(source)+12)
	for key, value := range source {
		result[key] = value
	}
	return result
}

// setHashWithTTL writes a Redis hash and immediately applies its bounded retention window.
func (api *API) setHashWithTTL(ctx context.Context, key string, values map[string]interface{}, ttl time.Duration) error {
	if err := api.Queue.HSet(ctx, key, values).Err(); err != nil {
		return err
	}
	if err := api.Queue.Expire(ctx, key, ttl).Err(); err != nil {
		return err
	}
	return nil
}

// removeRawObject deletes a redundant task-scoped upload after cache reuse is confirmed.
func (api *API) removeRawObject(ctx context.Context, rawKey string) {
	if rawKey == "" || api.Storage == nil {
		return
	}
	if err := api.Storage.RemoveObject(ctx, api.Storage.BucketName(), rawKey); err != nil {
		log.Printf("Failed to remove redundant raw object %s: %v", rawKey, err)
	}
}

// cacheArtifactsAvailable verifies that Redis metadata still points to every required
// object before a completed cache entry is reused.
func (api *API) cacheArtifactsAvailable(ctx context.Context, cacheData map[string]string) (bool, error) {
	resultKey := cacheData["result_s3_key"]
	if resultKey == "" {
		return false, nil
	}

	exists, err := api.Storage.StatObject(ctx, api.Storage.BucketName(), resultKey)
	if err != nil || !exists {
		return exists, err
	}

	if cacheData["operation"] != operationGleaner {
		return true, nil
	}

	for _, key := range []string{cacheData["index_s3_key"], cacheData["meta_s3_key"]} {
		if key == "" {
			return false, nil
		}
		exists, err = api.Storage.StatObject(ctx, api.Storage.BucketName(), key)
		if err != nil || !exists {
			return exists, err
		}
	}

	return true, nil
}

// aliasCompletedCache creates a fresh task alias that references an existing derived artifact.
// The alias receives an independent task TTL without extending artifact retention.
func (api *API) aliasCompletedCache(
	ctx context.Context,
	taskID string,
	baseTaskState map[string]interface{},
	cacheData map[string]string,
) error {
	state := cloneState(baseTaskState)
	delete(state, "s3_key")
	state["status"] = "completed"
	state["cache_hit"] = true
	state["deduplicated"] = true
	state["source_task_id"] = cacheData["owner_task_id"]
	state["fingerprint"] = cacheData["fingerprint"]
	state["cache_key"] = cacheData["cache_key"]
	state["result_s3_key"] = cacheData["result_s3_key"]
	state["result_url"] = fmt.Sprintf("s3://%s/%s", api.Storage.BucketName(), cacheData["result_s3_key"])
	state["updated_at"] = time.Now().UTC().Format(time.RFC3339)

	for _, field := range []string{"artifact_id", "index_s3_key", "meta_s3_key"} {
		if value := cacheData[field]; value != "" {
			state[field] = value
		}
	}

	return api.setHashWithTTL(ctx, taskHashKey(taskID), state, taskTTL())
}

// existingProcessingResolution returns the visible status of the task currently
// protected by a fingerprint lock.
func (api *API) existingProcessingResolution(ctx context.Context, ownerTaskID string) processingResolution {
	status := "processing"
	if ownerTaskID != "" {
		if taskData, err := api.Queue.HGetAll(ctx, taskHashKey(ownerTaskID)).Result(); err == nil {
			if currentStatus := taskData["status"]; currentStatus != "" {
				status = currentStatus
			}
		}
	}

	return processingResolution{
		ShouldEnqueue: false,
		TaskID:        ownerTaskID,
		Status:        status,
		CacheHit:      false,
		Deduplicated:  true,
	}
}

// resolveOrRegisterProcessing performs cache lookup, artifact validation, stale-lock
// recovery, and atomic owner selection before expensive work is enqueued.
func (api *API) resolveOrRegisterProcessing(
	ctx context.Context,
	operation string,
	fingerprint string,
	contentHash string,
	taskID string,
	rawKey string,
	baseTaskState map[string]interface{},
) (processingResolution, error) {
	cacheKey := cacheHashKey(operation, fingerprint)
	processingLockKey := lockKey(operation, fingerprint)
	paths := artifactKeys(operation, fingerprint)

	cacheData, err := api.Queue.HGetAll(ctx, cacheKey).Result()
	if err != nil && err != redis.Nil {
		return processingResolution{}, err
	}

	// Reuse completed work only when every derived object still exists in storage.
	if cacheData["status"] == "completed" {
		available, availabilityErr := api.cacheArtifactsAvailable(ctx, cacheData)
		if availabilityErr != nil {
			return processingResolution{}, availabilityErr
		}
		if available {
			api.removeRawObject(ctx, rawKey)
			if err := api.aliasCompletedCache(ctx, taskID, baseTaskState, cacheData); err != nil {
				return processingResolution{}, err
			}
			return processingResolution{
				ShouldEnqueue: false,
				TaskID:        taskID,
				Status:        "completed",
				CacheHit:      true,
				Deduplicated:  true,
			}, nil
		}

		if err := api.Queue.Del(ctx, cacheKey, processingLockKey).Err(); err != nil {
			return processingResolution{}, err
		}
	}

	// Active cache entries are reusable only while the fingerprint lease has a live owner.
	if cacheData["status"] == "processing" && cacheData["owner_task_id"] != "" {
		lockOwner, lockErr := api.Queue.Get(ctx, processingLockKey).Result()
		if lockErr != nil && lockErr != redis.Nil {
			return processingResolution{}, lockErr
		}

		if lockOwner != "" {
			resolution := api.existingProcessingResolution(ctx, lockOwner)
			if resolution.Status != "failed" {
				api.removeRawObject(ctx, rawKey)
				return resolution, nil
			}

			// A failed owner cannot continue protecting the fingerprint.
			if err := api.Queue.Del(ctx, cacheKey, processingLockKey).Err(); err != nil {
				return processingResolution{}, err
			}
		} else {
			// A processing cache entry without a live lock is stale and must not
			// prevent the same content from being processed again after a crash.
			if err := api.Queue.Del(ctx, cacheKey).Err(); err != nil {
				return processingResolution{}, err
			}
		}
	}

	// SET NX elects exactly one owner for concurrent requests with the same fingerprint.
	acquired, err := api.Queue.SetNX(ctx, processingLockKey, taskID, processingLockTTL()).Result()
	if err != nil {
		return processingResolution{}, err
	}
	if !acquired {
		ownerTaskID, getErr := api.Queue.Get(ctx, processingLockKey).Result()
		if getErr != nil && getErr != redis.Nil {
			return processingResolution{}, getErr
		}
		if ownerTaskID != "" {
			resolution := api.existingProcessingResolution(ctx, ownerTaskID)
			if resolution.Status != "failed" {
				api.removeRawObject(ctx, rawKey)
				return resolution, nil
			}
		}

		if err := api.Queue.Del(ctx, processingLockKey).Err(); err != nil {
			return processingResolution{}, err
		}
		acquired, err = api.Queue.SetNX(ctx, processingLockKey, taskID, processingLockTTL()).Result()
		if err != nil || !acquired {
			if err != nil {
				return processingResolution{}, err
			}
			return processingResolution{}, fmt.Errorf("failed to acquire processing lock")
		}
	}

	// Persist client-visible task state before the queue receives the owner payload.
	taskState := cloneState(baseTaskState)
	taskState["status"] = "queued"
	taskState["content_hash"] = contentHash
	taskState["fingerprint"] = fingerprint
	taskState["cache_key"] = cacheKey
	taskState["lock_key"] = processingLockKey
	taskState["result_s3_key"] = paths.ResultKey
	taskState["cache_hit"] = false
	taskState["deduplicated"] = false

	if paths.ArtifactID != "" {
		taskState["artifact_id"] = paths.ArtifactID
		taskState["index_s3_key"] = paths.IndexKey
		taskState["meta_s3_key"] = paths.MetaKey
	}

	if err := api.setHashWithTTL(ctx, taskHashKey(taskID), taskState, taskTTL()); err != nil {
		_ = api.Queue.Del(ctx, processingLockKey).Err()
		return processingResolution{}, err
	}

	// Register the in-flight computation so later duplicates can attach to the owner task.
	cacheState := map[string]interface{}{
		"cache_key":     cacheKey,
		"status":        "processing",
		"operation":     operation,
		"fingerprint":   fingerprint,
		"content_hash":  contentHash,
		"owner_task_id": taskID,
		"result_s3_key": paths.ResultKey,
		"created_at":    time.Now().UTC().Format(time.RFC3339),
		"updated_at":    time.Now().UTC().Format(time.RFC3339),
	}
	if paths.ArtifactID != "" {
		cacheState["artifact_id"] = paths.ArtifactID
		cacheState["index_s3_key"] = paths.IndexKey
		cacheState["meta_s3_key"] = paths.MetaKey
	}

	if err := api.setHashWithTTL(ctx, cacheKey, cacheState, cacheTTL()); err != nil {
		_ = api.Queue.Del(ctx, processingLockKey).Err()
		return processingResolution{}, err
	}

	return processingResolution{
		ShouldEnqueue: true,
		TaskID:        taskID,
		Status:        "queued",
		CacheHit:      false,
		Deduplicated:  false,
	}, nil
}

// responseForResolution normalizes cache-hit and deduplication metadata returned by ingestion handlers.
func responseForResolution(message string, resolution processingResolution) map[string]interface{} {
	return map[string]interface{}{
		"message":      message,
		"task_id":      resolution.TaskID,
		"status":       resolution.Status,
		"cache_hit":    resolution.CacheHit,
		"deduplicated": resolution.Deduplicated,
	}
}

// failRegisteredProcessing marks pre-dispatch failures and releases cache/lock state so
// a later identical request can retry instead of remaining blocked by stale metadata.
func (api *API) failRegisteredProcessing(ctx context.Context, operation string, fingerprint string, taskID string, errorMessage string) {
	_ = api.Queue.HSet(ctx, taskHashKey(taskID), map[string]interface{}{
		"status":     "failed",
		"error":      errorMessage,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}).Err()
	_ = api.Queue.Expire(ctx, taskHashKey(taskID), taskTTL()).Err()
	_ = api.Queue.Del(ctx, cacheHashKey(operation, fingerprint), lockKey(operation, fingerprint)).Err()
}