import json
import numpy as np
from services.gleaner_qa_service import _assemble_context, _clean_stream_delta, process_qa_task


def test_assemble_context_merges_adjacent_chunks():
    """Adjacent retrieval chunks are merged while distant passages remain separate."""
    # Provide adjacent and distant retrieval chunks.
    retrieved_items = [
        {"chunk": {"id": 1, "text": "Part A.", "start_char": 0, "end_char": 10}},
        {"chunk": {"id": 2, "text": "Part B.", "start_char": 12, "end_char": 20}},
        {"chunk": {"id": 5, "text": "Part C.", "start_char": 1000, "end_char": 1010}},
    ]

    # Assemble context in document order.
    context_str = _assemble_context(retrieved_items)

    # Adjacent chunks merge while distant evidence remains isolated.
    assert "[Chunks 1-2]: Part A. Part B." in context_str
    assert "[Chunk 5]: Part C." in context_str


def test_clean_stream_delta_handles_split_think_tags():
    """Thinking tags split across stream fragments never reach published output."""
    # Split thinking tags across independent generation fragments.
    state = {"buffer": "", "is_thinking": False}
    pieces = ["<th", "ink>", "hidden", " reasoning", "</th", "ink>", "Final answer"]
    # Feed each fragment through the streaming state machine.
    output = "".join(_clean_stream_delta(piece, state) for piece in pieces)

    # Internal reasoning never reaches the visible output.
    assert output == "Final answer"
    assert "hidden" not in output


def test_process_qa_task_streaming_ignores_think_tags(mocker):
    """QA streaming publishes only final answer tokens and uses stable artifact keys."""
    # Mock embeddings, FAISS retrieval, LLM streaming, and Redis Pub/Sub.
    mock_embed = mocker.Mock()
    mock_embed.embed.return_value = iter([np.zeros(384)])
    mocker.patch("services.gleaner_qa_service.get_embedding_model", return_value=mock_embed)

    mock_index = mocker.Mock()
    mock_index.ntotal = 1
    mock_index.search.return_value = (np.array([[0.8]], dtype=np.float32), np.array([[0]]))
    mock_meta = {
        "chunks": [
            {"id": 0, "text": "Fake chunk", "start_char": 0, "end_char": 10}
        ]
    }
    get_index = mocker.patch(
        "services.gleaner_qa_service._get_index_and_meta",
        return_value=(mock_index, mock_meta),
    )

    mock_llm = mocker.Mock()

    def fake_streamer(**kwargs):
        yield {"choices": [{"delta": {"content": "<th"}}]}
        yield {"choices": [{"delta": {"content": "ink>hidden reasoning</think>"}}]}
        yield {"choices": [{"delta": {"content": "Final "}}]}
        yield {"choices": [{"delta": {"content": "Answer."}}]}

    mock_llm.create_chat_completion = fake_streamer
    mocker.patch("services.gleaner_qa_service.get_llm", return_value=mock_llm)

    mock_redis = mocker.Mock()
    mocker.patch("services.gleaner_qa_service.redis.from_url", return_value=mock_redis)

    # Execute the complete QA streaming path.
    result = process_qa_task({
        "task_id": "stream_123",
        "document_id": "task_456",
        "artifact_id": "fingerprint_abc",
        "index_s3_key": "cache/gleaner/fingerprint_abc/index.faiss",
        "meta_s3_key": "cache/gleaner/fingerprint_abc/meta.json",
        "query": "Test?",
    })

    # Validate final answer, stable artifact lookup, and hidden-reasoning suppression.
    assert result["answer"] == "Final Answer."
    assert result["artifact_id"] == "fingerprint_abc"
    get_index.assert_called_once_with(
        "fingerprint_abc",
        "cache/gleaner/fingerprint_abc/index.faiss",
        "cache/gleaner/fingerprint_abc/meta.json",
    )

    published_payloads = [call.args[1] for call in mock_redis.publish.call_args_list]
    for payload in published_payloads:
        if payload != "[DONE]":
            assert "hidden reasoning" not in payload
            parsed = json.loads(payload)
            assert "<think>" not in parsed["token"]
