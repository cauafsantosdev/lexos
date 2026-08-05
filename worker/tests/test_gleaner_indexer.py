import numpy as np
import pytest
from services.gleaner_indexer_service import _token_chunk_text, process_indexing_task


def test_token_chunk_text_preserves_offsets(mocker):
    """Token chunks preserve original character offsets without decode mutation."""
    # Mock tokenizer IDs and exact source character offsets.
    mock_tokenizer = mocker.Mock()
    mock_encoding = mocker.Mock()
    mock_encoding.ids = [1, 2, 3, 4]
    mock_encoding.offsets = [(0, 7), (7, 12), (12, 14), (14, 20)]
    mock_tokenizer.encode.return_value = mock_encoding
    mocker.patch("services.gleaner_indexer_service.get_tokenizer", return_value=mock_tokenizer)

    # Chunk with overlap while preserving original source slices.
    text = "Hello, world! Lexos."
    chunks = _token_chunk_text(text, chunk_size=2, overlap=1)

    # Verify chunk text and offsets remain faithful to the source string.
    assert len(chunks) == 3
    assert chunks[0] == {
        "id": 0,
        "text": "Hello, world",
        "start_char": 0,
        "end_char": 12,
    }
    assert chunks[1]["text"] == "world! "
    assert chunks[2]["text"] == "! Lexos."


def test_process_indexing_task_uses_content_addressed_keys(mocker):
    """FAISS and metadata artifacts are persisted under fingerprint-derived keys."""
    # Bypass parsing and model loading with deterministic test doubles.
    mocker.patch("services.gleaner_indexer_service.extract_text", return_value="Fake content")
    fake_chunks = [
        {"id": 0, "text": "This is chunk one.", "start_char": 0, "end_char": 18},
        {"id": 1, "text": "This is chunk two.", "start_char": 19, "end_char": 37},
    ]
    mocker.patch("services.gleaner_indexer_service._token_chunk_text", return_value=fake_chunks)

    mock_embed_model = mocker.Mock()
    mock_embed_model.embed.return_value = iter([np.zeros(384), np.zeros(384)])
    mocker.patch("services.gleaner_indexer_service.get_embedding_model", return_value=mock_embed_model)

    mocker.patch("faiss.normalize_L2")
    mock_index = mocker.Mock()
    mocker.patch("faiss.IndexFlatIP", return_value=mock_index)
    mocker.patch("faiss.write_index")
    mocker.patch("os.path.exists", return_value=True)
    mocker.patch("os.remove")

    mock_upload_file = mocker.patch("services.gleaner_indexer_service.upload_file_to_s3")
    mock_upload_json = mocker.patch("services.gleaner_indexer_service.upload_json_to_s3")

    task_data = {
        "task_id": "task_123",
        "artifact_id": "fingerprint_abc",
        "document_text": "Fake content",
        "index_s3_key": "cache/gleaner/fingerprint_abc/index.faiss",
        "meta_s3_key": "cache/gleaner/fingerprint_abc/meta.json",
    }
    # Execute indexing with fingerprint-derived artifact destinations.
    result = process_indexing_task(task_data)

    # Verify the result contract, E5 prefixes, FAISS insertion, and storage keys.
    assert result == {
        "status": "indexed",
        "artifact_id": "fingerprint_abc",
        "chunks_indexed": 2,
    }
    called_passages = mock_embed_model.embed.call_args[0][0]
    assert called_passages == [
        "passage: This is chunk one.",
        "passage: This is chunk two.",
    ]
    mock_index.add.assert_called_once()
    mock_upload_file.assert_called_once_with(
        "cache/gleaner/fingerprint_abc/index.faiss",
        "/tmp/fingerprint_abc.faiss",
    )
    mock_upload_json.assert_called_once()


def test_process_indexing_task_missing_artifact_id(mocker):
    """Indexing fails before model work when content-addressed metadata is incomplete."""
    # Model loading is mocked because validation must fail first.
    mocker.patch("services.gleaner_indexer_service.get_embedding_model")

    # Missing content-addressed identity fails before embedding work.
    with pytest.raises(ValueError, match="artifact_id is required"):
        process_indexing_task({"task_id": "task_123", "document_text": "Content"})
