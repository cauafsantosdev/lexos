import json
import pytest
from broker.consumer import start_worker

def test_consumer_processes_transcription_task_successfully(mocker):
    # Configure the Redis client behavior.
    mock_redis = mocker.Mock()
    
    # Return one task, then raise KeyboardInterrupt to terminate the worker loop.
    task_payload = json.dumps({
        "task_id": "task_999",
        "type": "transcription",
        "s3_key": "raw/task_999/source.wav",
        "result_s3_key": "cache/scriber/fingerprint/result.json",
    })
    mock_redis.blpop.side_effect = [
        ("lexos:queue:transcription", task_payload),
        KeyboardInterrupt()
    ]
    
    mocker.patch("broker.consumer.redis.from_url", return_value=mock_redis)
    
    # Mock object-storage utilities and local file checks.
    mocker.patch("broker.consumer.download_s3_file_to_temp", return_value="/tmp/fake_audio.wav")
    mocker.patch("broker.consumer.upload_json_to_s3", return_value="s3://lexos-storage/cache/scriber/fingerprint/result.json")
    mocker.patch("os.path.exists", return_value=True)
    mock_remove = mocker.patch("os.remove")

    # Mock the Scriber service function.
    mock_transcribe = mocker.patch(
        "broker.consumer.transcribe_audio", 
        return_value={"language": "en", "language_probability": 0.99, "text": "Hello world"}
    )

    # Execute the worker until the forced KeyboardInterrupt exit.
    with pytest.raises(KeyboardInterrupt):
        start_worker()

    # Verify Redis status updates.
    hset_calls = mock_redis.hset.call_args_list
    assert any("processing" in str(call) for call in hset_calls)
    assert any("completed" in str(call) for call in hset_calls)

    # Verify service routing.
    mock_transcribe.assert_called_once_with("/tmp/fake_audio.wav")

    # Verify temporary-file cleanup.
    mock_remove.assert_called_once_with("/tmp/fake_audio.wav")


def test_consumer_handles_task_failure_gracefully(mocker):
    # Return a summarization task, then terminate the loop.
    mock_redis = mocker.Mock()
    task_payload = json.dumps({
        "task_id": "task_fail",
        "type": "summarization",
        "s3_key": "raw/task_fail/source.txt",
        "result_s3_key": "cache/distiller/fingerprint/result.json",
    })
    mock_redis.blpop.side_effect = [
        ("lexos:queue:summarization", task_payload),
        KeyboardInterrupt()
    ]
    
    mocker.patch("broker.consumer.redis.from_url", return_value=mock_redis)
    mocker.patch("broker.consumer.download_s3_file_to_temp", return_value="/tmp/fake.txt")
    mocker.patch("os.path.exists", return_value=False)

    # Force an unexpected summarization failure.
    mocker.patch(
        "broker.consumer.process_summarization_task", 
        side_effect=RuntimeError("LLM Engine crashed")
    )

    # Execute the worker until the forced exit.
    with pytest.raises(KeyboardInterrupt):
        start_worker()

    # Verify the persisted failure state and error message.
    hset_calls = mock_redis.hset.call_args_list
    failed_call_found = False
    for call in hset_calls:
        mapping_arg = call.kwargs.get("mapping", {})
        if mapping_arg.get("status") == "failed" and "LLM Engine crashed" in mapping_arg.get("error", ""):
            failed_call_found = True
            break
            
    assert failed_call_found, "Redis was not updated with the correct 'failed' status and error trace."

def test_consumer_reuses_completed_cache_without_downloading_or_running_model(mocker):
    """Completed reusable artifacts bypass raw downloads and model inference."""
    mock_redis = mocker.Mock()
    task_payload = json.dumps({
        "task_id": "task_cached",
        "type": "transcription",
        "s3_key": "raw/task_cached/source.wav",
        "cache_key": "lexos:cache:scriber:fingerprint",
        "lock_key": "lexos:lock:scriber:fingerprint",
    })
    mock_redis.blpop.side_effect = [
        ("lexos:queue:transcription", task_payload),
        KeyboardInterrupt(),
    ]
    mock_redis.hgetall.return_value = {
        "status": "completed",
        "fingerprint": "fingerprint",
        "cache_key": "lexos:cache:scriber:fingerprint",
        "result_s3_key": "cache/scriber/fingerprint/result.json",
        "result_url": "s3://lexos-storage/cache/scriber/fingerprint/result.json",
    }

    mocker.patch("broker.consumer.redis.from_url", return_value=mock_redis)
    mocker.patch("broker.consumer.cache_artifacts_available", return_value=True)
    download = mocker.patch("broker.consumer.download_s3_file_to_temp")
    transcribe = mocker.patch("broker.consumer.transcribe_audio")
    release = mocker.patch("broker.consumer.release_owned_lock")
    delete_raw = mocker.patch("broker.consumer.delete_s3_object")

    with pytest.raises(KeyboardInterrupt):
        start_worker()

    download.assert_not_called()
    transcribe.assert_not_called()
    delete_raw.assert_called_once_with("raw/task_cached/source.wav")
    release.assert_called_once_with(
        mock_redis,
        "lexos:lock:scriber:fingerprint",
        "task_cached",
    )
    completed_calls = [
        call for call in mock_redis.hset.call_args_list
        if call.args and call.args[0] == "task:task_cached"
    ]
    assert completed_calls
    assert any(call.kwargs.get("mapping", {}).get("status") == "completed" for call in completed_calls)
