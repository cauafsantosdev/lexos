import json
from services.gleaner_qa_service import _assemble_context, process_qa_task

def test_assemble_context_merges_adjacent_chunks():
    retrieved_items = [
        {"chunk": {"id": 1, "text": "Part A.", "start_char": 0, "end_char": 10}},
        # Adjacent chunk (starts at 12, difference < 20)
        {"chunk": {"id": 2, "text": "Part B.", "start_char": 12, "end_char": 20}},
        # Distant chunk (starts at 1000)
        {"chunk": {"id": 5, "text": "Part C.", "start_char": 1000, "end_char": 1010}}
    ]
    
    context_str = _assemble_context(retrieved_items)
    
    # Verify 1 and 2 merged into a single block
    assert "[Chunks 1-2]: Part A. Part B." in context_str
    # Verify 5 remained isolated
    assert "[Chunk 5]: Part C." in context_str

def test_process_qa_task_streaming_ignores_think_tags(mocker):
    # 1. Mock the Embedding Model & FAISS retrieval
    mock_embed = mocker.Mock()
    # Fake embedding matrix (1, 384)
    import numpy as np
    mock_embed.embed.return_value = iter([np.zeros(384)])
    mocker.patch("services.gleaner_qa_service.get_embedding_model", return_value=mock_embed)
    
    mock_index = mocker.Mock()
    mock_index.search.return_value = ([[0.8]], [[0]]) # Score 0.8, Chunk Index 0
    mock_meta = {"chunks": {0: {"id": 0, "text": "Fake chunk", "start_char": 0, "end_char": 10}}}
    mocker.patch("services.gleaner_qa_service._get_index_and_meta", return_value=(mock_index, mock_meta))
    
    # 2. Mock the LLM to stream tokens one by one, including <think> tags
    mock_llm = mocker.Mock()
    def fake_streamer(**kwargs):
        yield {"choices": [{"delta": {"content": "<think>"}}]}
        yield {"choices": [{"delta": {"content": "hidden reasoning"}}]}
        yield {"choices": [{"delta": {"content": "</think>"}}]}
        yield {"choices": [{"delta": {"content": "Final "}}]}
        yield {"choices": [{"delta": {"content": "Answer."}}]}
    
    mock_llm.create_chat_completion = fake_streamer
    mocker.patch("services.gleaner_qa_service.get_llm", return_value=mock_llm)

    # 3. Mock Redis to track what actually gets published
    mock_redis = mocker.Mock()
    mocker.patch("services.gleaner_qa_service.redis.from_url", return_value=mock_redis)
    
    # ACT
    result = process_qa_task({"task_id": "123", "document_id": "456", "query": "Test?"})
    
    # ASSERT
    assert result["answer"] == "Final Answer."
    
    # Verify that Redis NEVER published the "hidden reasoning" tokens to the client
    published_payloads = [call[0][1] for call in mock_redis.publish.call_args_list]
    for payload in published_payloads:
        if payload != "[DONE]":
            assert "hidden reasoning" not in payload