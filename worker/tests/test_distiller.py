import pytest
from services.distiller_service import (
    _sanitize_document,
    _chunk_text,
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

def test_chunk_text_respects_overlap():
    # 100 character string
    text = "A" * 100 
    
    # Chunk size 60, overlap 20. 
    # Chunk 1: 0-60
    # Chunk 2: 40-100
    chunks = _chunk_text(text, chunk_size=60, overlap=20)
    
    assert len(chunks) == 2
    assert len(chunks[0]) == 60
    assert len(chunks[1]) == 60

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
    # Mock file parsing and inference to avoid reading files or running LLMs
    mocker.patch("services.distiller_service.extract_text", return_value="Raw input data")
    
    # Return a bad, bulleted response from the mocked LLM
    mocker.patch(
        "services.distiller_service._run_inference", 
        return_value="- This is point one.\n- This is point two."
    )
    
    task_data = {"document_text": "Fake text", "style": "short_paragraph"}
    result = process_summarization_task(task_data)
    
    # Verify the post-processing regex wiped out the bullets and newlines
    assert "\n" not in result["summary"]
    assert "- " not in result["summary"]
    assert result["summary"] == "This is point one. This is point two."