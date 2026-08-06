import pytest
from services.distiller_service import (
    _sanitize_document,
    _chunk_text_by_tokens,
    _run_inference,
    process_summarization_task
)

def test_sanitize_document_removes_references():
    raw_text = "This is the core research.\nREFERENCES\n1. Smith et al. (2025)."
    clean_text = _sanitize_document(raw_text)
    
    assert "core research" in clean_text
    assert "Smith et al." not in clean_text
    assert "REFERENCES" not in clean_text

def test_sanitize_document_ignores_normal_text():
    raw_text = "This paper discusses the reference architectures of modern AI."
    clean_text = _sanitize_document(raw_text)
    
    # Should not cut off the word "reference" when used in normal context
    assert clean_text == raw_text

def test_chunk_text_by_tokens_respects_overlap(mocker):
    # Use ASCII bytes as deterministic stand-in token IDs so the test validates
    # chunk boundaries without loading the production Qwen model.
    text = "A" * 100

    mock_llm = mocker.Mock()
    mock_llm.tokenize.side_effect = (
        lambda value, add_bos=False, special=False: list(value)
    )
    mock_llm.detokenize.side_effect = (
        lambda tokens, special=False: bytes(tokens)
    )

    # Chunk size 60, overlap 20.
    # Chunk 1: tokens 0-60
    # Chunk 2: tokens 40-100
    chunks = _chunk_text_by_tokens(
        text,
        mock_llm,
        chunk_size=60,
        overlap=20,
    )

    assert len(chunks) == 2
    assert len(chunks[0]) == 60
    assert len(chunks[1]) == 60

def test_chunk_text_by_tokens_rejects_invalid_overlap(mocker):
    mock_llm = mocker.Mock()

    with pytest.raises(ValueError, match="overlap must be smaller"):
        _chunk_text_by_tokens(
            "document",
            mock_llm,
            chunk_size=100,
            overlap=100,
        )

def test_chunk_text_by_tokens_returns_empty_list_for_empty_text(mocker):
    mock_llm = mocker.Mock()

    assert _chunk_text_by_tokens("", mock_llm) == []

def test_run_inference_strips_think_tags(mocker):
    # Intercept the LLM call
    mock_llm = mocker.Mock()
    mock_llm.create_chat_completion.return_value = {
        "choices": [{"message": {"content": "<think>Thinking process...</think> The actual summary."}}]
    }
    mocker.patch("services.distiller_service.get_llm", return_value=mock_llm)
    
    result = _run_inference("System prompt", "User prompt")
    
    assert "Thinking process..." not in result
    assert result == "The actual summary."

def test_process_summarization_short_paragraph_formatting(mocker):
    # Mock file parsing and inference to avoid reading files or running LLMs.
    mocker.patch(
        "services.distiller_service.extract_text",
        return_value="Raw input data",
    )

    # Keep token counting isolated from the production Qwen model.
    mock_llm = mocker.Mock()
    mock_llm.tokenize.return_value = [1, 2, 3]
    mocker.patch(
        "services.distiller_service.get_llm",
        return_value=mock_llm,
    )

    # Return a bad, bulleted response from the mocked LLM.
    mocker.patch(
        "services.distiller_service._run_inference",
        return_value="- This is point one.\n- This is point two.",
    )

    task_data = {
        "document_text": "Fake text",
        "style": "short_paragraph",
    }

    result = process_summarization_task(task_data)

    # Verify the post-processing regex wiped out the bullets and newlines.
    assert "\n" not in result["summary"]
    assert "- " not in result["summary"]
    assert result["summary"] == "This is point one. This is point two."