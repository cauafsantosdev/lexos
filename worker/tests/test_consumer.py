import json
import pytest
from broker.consumer import start_worker

def test_consumer_processes_transcription_task_successfully(mocker):
    # ARRANGE: Mock the Redis client and its behavior
    mock_redis = mocker.Mock()
    
    # Simulate blpop returning a task once, then raising KeyboardInterrupt to bypass the 'except Exception' block
    task_payload = json.dumps({
        "task_id": "task_999",
        "s3_key": "audio/test.wav"
    })
    mock_redis.blpop.side_effect = [
        ("lexos:queue:transcription", task_payload),
        KeyboardInterrupt()
    ]
    
    mocker.patch("broker.consumer.redis.from_url", return_value=mock_redis)
    
    # ARRANGE: Mock S3 utilities and file system checks
    mocker.patch("broker.consumer.download_s3_file_to_temp", return_value="/tmp/fake_audio.wav")
    mocker.patch("broker.consumer.upload_json_to_s3", return_value="s3://lexos-storage/results/task_999.json")
    mocker.patch("os.path.exists", return_value=True)
    mock_remove = mocker.patch("os.remove")

    # ARRANGE: Mock the Scriber service function
    mock_transcribe = mocker.patch(
        "broker.consumer.transcribe_audio", 
        return_value={"language": "en", "language_probability": 0.99, "text": "Hello world"}
    )

    # ACT & ASSERT: Run start_worker and catch our forced KeyboardInterrupt exit
    with pytest.raises(KeyboardInterrupt):
        start_worker()

    # ASSERT: Verify Redis status updates were correctly dispatched
    hset_calls = mock_redis.hset.call_args_list
    assert any("processing" in str(call) for call in hset_calls)
    assert any("completed" in str(call) for call in hset_calls)

    # ASSERT: Verify the correct service was routed to
    mock_transcribe.assert_called_once_with("/tmp/fake_audio.wav")

    # ASSERT: Verify strict disk cleanup in the finally block
    mock_remove.assert_called_once_with("/tmp/fake_audio.wav")


def test_consumer_handles_task_failure_gracefully(mocker):
    # ARRANGE: Mock Redis to return a summarization task, then break the loop
    mock_redis = mocker.Mock()
    task_payload = json.dumps({
        "task_id": "task_fail",
        "s3_key": "documents/bad.txt"
    })
    mock_redis.blpop.side_effect = [
        ("lexos:queue:summarization", task_payload),
        KeyboardInterrupt()
    ]
    
    mocker.patch("broker.consumer.redis.from_url", return_value=mock_redis)
    mocker.patch("broker.consumer.download_s3_file_to_temp", return_value="/tmp/fake.txt")
    mocker.patch("os.path.exists", return_value=False)

    # ARRANGE: Force the summarization service to throw an unexpected error
    mocker.patch(
        "broker.consumer.process_summarization_task", 
        side_effect=RuntimeError("LLM Engine crashed")
    )

    # ACT & ASSERT
    with pytest.raises(KeyboardInterrupt):
        start_worker()

    # ASSERT: Verify Redis captured the failure state and error message
    hset_calls = mock_redis.hset.call_args_list
    failed_call_found = False
    for call in hset_calls:
        mapping_arg = call.kwargs.get("mapping", {})
        if mapping_arg.get("status") == "failed" and "LLM Engine crashed" in mapping_arg.get("error", ""):
            failed_call_found = True
            break
            
    assert failed_call_found, "Redis was not updated with the correct 'failed' status and error trace."