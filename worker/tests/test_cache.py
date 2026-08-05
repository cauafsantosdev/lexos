from utils.cache import cached_task_mapping, clear_owned_cache, release_owned_lock


def test_cached_task_mapping_carries_gleaner_artifact_references():
    """Completed cache entries expose stable index references to task state."""
    # Configure deterministic inputs and dependency state.
    entry = {
        "fingerprint": "abc",
        "cache_key": "lexos:cache:gleaner:abc",
        "result_s3_key": "cache/gleaner/abc/result.json",
        "result_url": "s3://lexos-storage/cache/gleaner/abc/result.json",
        "artifact_id": "abc",
        "index_s3_key": "cache/gleaner/abc/index.faiss",
        "meta_s3_key": "cache/gleaner/abc/meta.json",
    }

    mapping = cached_task_mapping(entry)

    assert mapping["status"] == "completed"
    assert mapping["cache_hit"] is True
    assert mapping["artifact_id"] == "abc"
    assert mapping["index_s3_key"].endswith("index.faiss")


def test_release_owned_lock_uses_atomic_compare_and_delete(mocker):
    """Distributed lock cleanup uses a single atomic ownership check."""
    # Configure deterministic inputs and dependency state.
    redis_client = mocker.Mock()

    release_owned_lock(redis_client, "lexos:lock:scriber:abc", "task_123")

    redis_client.eval.assert_called_once()
    call = redis_client.eval.call_args
    assert call.args[1:] == (1, "lexos:lock:scriber:abc", "task_123")


def test_clear_owned_cache_uses_atomic_owner_and_status_check(mocker):
    """Failed-task cache cleanup uses a single atomic ownership check."""
    # Configure deterministic inputs and dependency state.
    redis_client = mocker.Mock()

    clear_owned_cache(redis_client, "lexos:cache:scriber:abc", "task_123")

    redis_client.eval.assert_called_once()
    call = redis_client.eval.call_args
    assert call.args[1:] == (1, "lexos:cache:scriber:abc", "task_123")


def test_gleaner_cache_requires_result_index_and_metadata(mocker):
    """Gleaner cache reuse requires every persisted artifact to remain available."""
    # Configure deterministic inputs and dependency state.
    from utils.cache import cache_artifacts_available

    exists = mocker.patch("utils.cache.object_exists")
    exists.side_effect = lambda key: not key.endswith("meta.json")
    entry = {
        "operation": "gleaner",
        "result_s3_key": "cache/gleaner/abc/result.json",
        "index_s3_key": "cache/gleaner/abc/index.faiss",
        "meta_s3_key": "cache/gleaner/abc/meta.json",
    }

    assert cache_artifacts_available(entry) is False
    assert exists.call_count == 3
