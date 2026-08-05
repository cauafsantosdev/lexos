import json
import os
from datetime import datetime, timezone
import redis
from services.scriber_service import transcribe_audio
from services.distiller_service import process_summarization_task
from services.gleaner_indexer_service import process_indexing_task
from services.gleaner_qa_service import process_qa_task
from utils.cache import (
    cache_artifacts_available,
    cache_ttl_seconds,
    cached_task_mapping,
    clear_owned_cache,
    release_owned_lock,
    task_ttl_seconds,
)
from utils.s3 import delete_s3_object, download_s3_file_to_temp, upload_json_to_s3
from utils.logger import get_logger


# Initialize the worker-specific logger once at module import.
logger = get_logger("consumer")


def _redis_client():
    """
    Creates a Redis client configured for blocking queue consumption.

    Returns:
        redis.Redis: Connected Redis client instance.
    """
    # Resolve Redis connectivity once for the blocking worker process.
    redis_url = os.getenv("REDIS_URL", "redis://redis:6379")
    if not redis_url.startswith("redis://") and not redis_url.startswith("rediss://"):
        redis_url = f"redis://{redis_url}"

    return redis.from_url(
        redis_url,
        decode_responses=True,
        socket_keepalive=True,
        socket_timeout=None,
        health_check_interval=30,
    )


def _remove_redundant_raw_object(s3_key: str) -> None:
    """
    Removes an input object when a queued task resolves from completed cache.

    Args:
        s3_key (str): Task-scoped raw object key.
    """
    if not s3_key:
        return

    try:
        delete_s3_object(s3_key)
    except Exception as exc:
        logger.warning(f"Failed to remove redundant raw object {s3_key}: {exc}")


def _apply_cached_completion(redis_client, task_hash: str, cache_entry: dict[str, str]) -> None:
    """
    Applies a completed reusable cache entry to a queued task.

    Args:
        redis_client: Active Redis client.
        task_hash (str): Redis task hash key.
        cache_entry (dict[str, str]): Completed processing-cache metadata.
    """
    redis_client.hset(task_hash, mapping=cached_task_mapping(cache_entry, cache_hit=True))
    redis_client.expire(task_hash, task_ttl_seconds())


def _mark_processing_cache(redis_client, task_data: dict, task_id: str) -> None:
    """
    Refreshes the processing cache metadata for the active owner task.

    Args:
        redis_client: Active Redis client.
        task_data (dict): Queue payload.
        task_id (str): Current owner task identifier.
    """
    cache_key = task_data.get("cache_key")
    if not cache_key:
        return

    mapping = {
        "status": "processing",
        "owner_task_id": task_id,
        "updated_at": datetime.now(timezone.utc).isoformat(),
    }
    redis_client.hset(cache_key, mapping=mapping)
    redis_client.expire(cache_key, cache_ttl_seconds())


def _mark_completed(redis_client, task_hash: str, task_data: dict, result_s3_key: str, result_url: str) -> None:
    """
    Persists completed task state and reusable processing-cache metadata.

    Args:
        redis_client: Active Redis client.
        task_hash (str): Redis task hash key.
        task_data (dict): Queue payload containing cache metadata.
        result_s3_key (str): Stable content-addressed result object key.
        result_url (str): Internal S3 URI returned by object storage.
    """
    now = datetime.now(timezone.utc).isoformat()
    task_mapping = {
        "status": "completed",
        "result_s3_key": result_s3_key,
        "result_url": result_url,
        "updated_at": now,
    }

    for field in ("fingerprint", "artifact_id", "index_s3_key", "meta_s3_key", "cache_key"):
        value = task_data.get(field)
        if value:
            task_mapping[field] = value

    redis_client.hset(task_hash, mapping=task_mapping)
    redis_client.expire(task_hash, task_ttl_seconds())

    cache_key = task_data.get("cache_key")
    if cache_key:
        cache_mapping = {
            "cache_key": cache_key,
            "status": "completed",
            "operation": {
                "transcription": "scriber",
                "summarization": "distiller",
                "indexing": "gleaner",
            }.get(task_data.get("type"), task_data.get("type", "")),
            "fingerprint": task_data.get("fingerprint", ""),
            "content_hash": task_data.get("content_hash", ""),
            "owner_task_id": task_data.get("task_id", ""),
            "result_s3_key": result_s3_key,
            "result_url": result_url,
            "updated_at": now,
        }
        for field in ("artifact_id", "index_s3_key", "meta_s3_key"):
            value = task_data.get(field)
            if value:
                cache_mapping[field] = value

        redis_client.hset(cache_key, mapping=cache_mapping)
        redis_client.expire(cache_key, cache_ttl_seconds())


def _publish_stream_failure(redis_client, task_id: str) -> None:
    """
    Terminates a QA SSE stream after worker failure.

    Args:
        redis_client: Active Redis client.
        task_id (str): QA stream task identifier.
    """
    channel = f"lexos:stream:{task_id}"
    message = "The answer could not be generated because the processing task failed."
    redis_client.publish(channel, json.dumps({"token": message}))
    redis_client.publish(channel, "[DONE]")


def start_worker():
    """
    Starts the blocking Redis consumer loop for all Lexos AI workloads.

    The consumer updates task state, downloads raw artifacts when required,
    executes the selected service, persists derived artifacts, maintains the
    content-addressed processing cache, and releases distributed locks.

    Queues monitored:
        - lexos:queue:transcription
        - lexos:queue:summarization
        - lexos:queue:gleaner:index
        - lexos:queue:gleaner:ask

    Returns:
        None: The loop continues until the process receives an external stop signal.
    """
    redis_client = _redis_client()
    # Keep all CPU-bound workloads behind the same controlled consumer loop.
    queues = [
        "lexos:queue:transcription",
        "lexos:queue:summarization",
        "lexos:queue:gleaner:index",
        "lexos:queue:gleaner:ask",
    ]
    logger.info(f"Listening for tasks on: {', '.join(queues)}...")

    # Process one queued workload at a time to preserve the configured CPU/RAM budget.
    while True:
        temp_local_file = None
        task_hash = None
        task_id = None
        task_data = {}
        queue_name = None

        try:
            # BLPOP blocks efficiently while still returning periodically for connection health checks.
            popped = redis_client.blpop(queues, timeout=5)
            if not popped:
                continue

            # Decode the gateway payload and recover task-scoped processing metadata.
            queue_name, message = popped
            task_data = json.loads(message)
            task_id = task_data.get("task_id")
            s3_key = task_data.get("s3_key")

            if not task_id:
                raise ValueError("Queue payload is missing task_id.")

            task_hash = f"task:{task_id}"
            logger.info(f"Received task {task_id} from {queue_name}")

            # Recheck completed cache state in the worker to preserve idempotency across races.
            cache_key = task_data.get("cache_key")
            if cache_key:
                cache_entry = redis_client.hgetall(cache_key)
                if cache_entry.get("status") == "completed":
                    if cache_artifacts_available(cache_entry):
                        _apply_cached_completion(redis_client, task_hash, cache_entry)
                        _remove_redundant_raw_object(s3_key or "")
                        release_owned_lock(redis_client, task_data.get("lock_key", ""), task_id)
                        logger.info(f"Task {task_id} resolved from completed processing cache.")
                        continue
                    redis_client.delete(cache_key)

            # Transition the owner task to processing before any expensive model work begins.
            redis_client.hset(task_hash, mapping={
                "status": "processing",
                "updated_at": datetime.now(timezone.utc).isoformat(),
            })
            redis_client.expire(task_hash, task_ttl_seconds())
            _mark_processing_cache(redis_client, task_data, task_id)

            # Download raw input only for pipelines that require a physical local file.
            if s3_key:
                temp_local_file = download_s3_file_to_temp(s3_key)
                task_data["file_path"] = temp_local_file

            # Route the payload to the service associated with the source Redis queue.
            if queue_name == "lexos:queue:transcription":
                result = transcribe_audio(temp_local_file)
            elif queue_name == "lexos:queue:summarization":
                result = process_summarization_task(task_data)
            elif queue_name == "lexos:queue:gleaner:index":
                result = process_indexing_task(task_data)
            elif queue_name == "lexos:queue:gleaner:ask":
                result = process_qa_task(task_data)
            else:
                raise ValueError(f"Unsupported queue: {queue_name}")

            # QA output is streamed through Pub/Sub and does not produce a reusable result object.
            if queue_name == "lexos:queue:gleaner:ask":
                redis_client.hset(task_hash, mapping={
                    "status": "completed",
                    "updated_at": datetime.now(timezone.utc).isoformat(),
                })
                redis_client.expire(task_hash, task_ttl_seconds())
                continue

            # Persist non-streaming results under the stable content-addressed destination supplied by the gateway.
            result_s3_key = task_data.get("result_s3_key") or f"results/{task_id}.json"
            result_url = upload_json_to_s3(result_s3_key, result)
            _mark_completed(redis_client, task_hash, task_data, result_s3_key, result_url)
            release_owned_lock(redis_client, task_data.get("lock_key", ""), task_id)

            logger.info(f"Task {task_id} completed successfully. Result stored at {result_url}")

        except Exception as exc:
            # Record failure state without terminating the long-running consumer loop.
            logger.error(f"Error processing task: {exc}", exc_info=True)

            if task_hash and task_id:
                redis_client.hset(task_hash, mapping={
                    "status": "failed",
                    "error": str(exc),
                    "updated_at": datetime.now(timezone.utc).isoformat(),
                })
                redis_client.expire(task_hash, task_ttl_seconds())
                clear_owned_cache(redis_client, task_data.get("cache_key", ""), task_id)
                release_owned_lock(redis_client, task_data.get("lock_key", ""), task_id)

                if queue_name == "lexos:queue:gleaner:ask":
                    _publish_stream_failure(redis_client, task_id)
        finally:
            # Remove temporary downloads after every success or failure path.
            if temp_local_file and os.path.exists(temp_local_file):
                os.remove(temp_local_file)