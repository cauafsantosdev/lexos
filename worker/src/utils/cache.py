import os
from datetime import datetime, timezone
from utils.s3 import object_exists


# Lua scripts compare ownership before deletion so stale workers cannot clear another task's state.
_RELEASE_LOCK_SCRIPT = """
if redis.call('GET', KEYS[1]) == ARGV[1] then
    return redis.call('DEL', KEYS[1])
end
return 0
"""

_CLEAR_OWNED_CACHE_SCRIPT = """
local owner = redis.call('HGET', KEYS[1], 'owner_task_id')
local status = redis.call('HGET', KEYS[1], 'status')
if owner == ARGV[1] and status ~= 'completed' then
    return redis.call('DEL', KEYS[1])
end
return 0
"""


def _positive_int_env(name: str, default: int) -> int:
    """
    Resolves a positive integer environment variable.

    Args:
        name (str): Environment variable name.
        default (int): Fallback value.

    Returns:
        int: Configured positive integer or fallback value.
    """
    try:
        value = int(os.getenv(name, str(default)))
    except ValueError:
        return default
    return value if value > 0 else default


def cache_ttl_seconds() -> int:
    """Returns the Redis TTL for reusable derived-artifact metadata."""
    return _positive_int_env("CACHE_TTL_SECONDS", 7 * 24 * 60 * 60)


def task_ttl_seconds() -> int:
    """Returns the Redis TTL for task-state hashes."""
    return _positive_int_env("TASK_TTL_SECONDS", 24 * 60 * 60)


def cache_artifacts_available(cache_entry: dict[str, str]) -> bool:
    """
    Validates that every artifact referenced by a completed cache entry exists.

    Args:
        cache_entry (dict[str, str]): Redis cache metadata.

    Returns:
        bool: True when all required artifacts exist.
    """
    # A Redis cache hit is valid only while every referenced derived object still exists.
    result_key = cache_entry.get("result_s3_key", "")
    if not result_key or not object_exists(result_key):
        return False

    if cache_entry.get("operation") != "gleaner":
        return True

    index_key = cache_entry.get("index_s3_key", "")
    meta_key = cache_entry.get("meta_s3_key", "")
    return bool(index_key and meta_key and object_exists(index_key) and object_exists(meta_key))


def cached_task_mapping(cache_entry: dict[str, str], cache_hit: bool = True) -> dict[str, str | bool]:
    """
    Builds task-state fields from a reusable completed cache entry.

    Args:
        cache_entry (dict[str, str]): Redis cache metadata.
        cache_hit (bool, optional): Cache-hit marker for the task state.

    Returns:
        dict[str, str | bool]: Completed task-state fields.
    """
    mapping: dict[str, str | bool] = {
        "status": "completed",
        "cache_hit": cache_hit,
        "deduplicated": True,
        "fingerprint": cache_entry.get("fingerprint", ""),
        "cache_key": cache_entry.get("cache_key", ""),
        "result_s3_key": cache_entry.get("result_s3_key", ""),
        "result_url": cache_entry.get("result_url", ""),
        "source_task_id": cache_entry.get("owner_task_id", ""),
        "updated_at": datetime.now(timezone.utc).isoformat(),
    }

    for field in ("artifact_id", "index_s3_key", "meta_s3_key"):
        value = cache_entry.get(field, "")
        if value:
            mapping[field] = value

    return mapping


def release_owned_lock(redis_client, lock_key: str, task_id: str) -> None:
    """
    Releases a processing lock atomically when the current task owns it.

    Args:
        redis_client: Active Redis client.
        lock_key (str): Distributed lock key.
        task_id (str): Expected lock owner.
    """
    if not lock_key:
        return

    # Compare-and-delete keeps lock release atomic under concurrent retries.
    redis_client.eval(_RELEASE_LOCK_SCRIPT, 1, lock_key, task_id)


def clear_owned_cache(redis_client, cache_key: str, task_id: str) -> None:
    """
    Removes an incomplete cache entry atomically when the current task owns it.

    Args:
        redis_client: Active Redis client.
        cache_key (str): Processing-cache hash key.
        task_id (str): Expected cache owner.
    """
    if not cache_key:
        return

    # Incomplete cache metadata is removed only by the task recorded as its owner.
    redis_client.eval(_CLEAR_OWNED_CACHE_SCRIPT, 1, cache_key, task_id)
