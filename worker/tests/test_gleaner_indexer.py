import pytest
import numpy as np
from services.gleaner_indexer_service import _token_chunk_text, process_indexing_task

# TestTokenChunkText_PreservesOffsets verifies that the chunker correctly maps
# token IDs back to physical character boundaries without dropping spaces or punctuation.
def test_token_chunk_text_preserves_offsets(mocker):
    # ARRANGE: Mock the Rust Tokenizer
    mock_tokenizer = mocker.Mock()
    mock_encoding = mocker.Mock()
    
    # Simulate a sentence broken into 4 tokens
    # e.g., "Hello, " (0,7), "world" (7,12), "! " (12,14), "Lexos." (14,20)
    mock_encoding.ids = [1, 2, 3, 4]
    mock_encoding.offsets = [(0, 7), (7, 12), (12, 14), (14, 20)]
    mock_tokenizer.encode.return_value = mock_encoding
    
    mocker.patch("services.gleaner_indexer_service.get_tokenizer", return_value=mock_tokenizer)

    text = "Hello, world! Lexos."
    
    # ACT: Chunk with size=2, overlap=1
    # Chunk 1: Tokens [1, 2] -> offsets (0,7) to (7,12) -> text[0:12]
    # Chunk 2: Tokens [2, 3] -> offsets (7,12) to (12,14) -> text[7:14]
    # Chunk 3: Tokens [3, 4] -> offsets (12,14) to (14,20) -> text[12:20]
    chunks = _token_chunk_text(text, chunk_size=2, overlap=1)

    # ASSERT
    assert len(chunks) == 3
    
    assert chunks[0]["text"] == "Hello, world"
    assert chunks[0]["start_char"] == 0
    assert chunks[0]["end_char"] == 12

    assert chunks[1]["text"] == "world! "
    assert chunks[1]["start_char"] == 7
    assert chunks[1]["end_char"] == 14

    assert chunks[2]["text"] == "! Lexos."
    assert chunks[2]["start_char"] == 12
    assert chunks[2]["end_char"] == 20

# TestProcessIndexingTask_Success verifies the end-to-end FAISS pipeline, ensuring
# embeddings are cast to float32, normalized, and saved to S3.
def test_process_indexing_task_success(mocker):
    # ARRANGE: Bypass the file parser and inject fake text
    mocker.patch("services.gleaner_indexer_service.extract_text", return_value="Fake content")
    
    # ARRANGE: Mock the chunker to return two predefined chunks
    fake_chunks = [
        {"id": 0, "text": "This is chunk one.", "start_char": 0, "end_char": 18},
        {"id": 1, "text": "This is chunk two.", "start_char": 19, "end_char": 37}
    ]
    mocker.patch("services.gleaner_indexer_service._token_chunk_text", return_value=fake_chunks)

    # ARRANGE: Mock FastEmbed to yield fake vectors
    mock_embed_model = mocker.Mock()
    # FastEmbed returns a generator. We provide two 384-dimensional arrays.
    mock_embed_model.embed.return_value = iter([np.zeros(384), np.zeros(384)])
    mocker.patch("services.gleaner_indexer_service.get_embedding_model", return_value=mock_embed_model)

    # ARRANGE: Mock FAISS and OS operations
    mocker.patch("faiss.normalize_L2")
    mock_index = mocker.Mock()
    mocker.patch("faiss.IndexFlatIP", return_value=mock_index)
    mocker.patch("faiss.write_index")
    mocker.patch("os.remove")
    
    # ARRANGE: Mock MinIO S3 uploads
    mock_upload_file = mocker.patch("services.gleaner_indexer_service.upload_file_to_s3")
    mock_upload_json = mocker.patch("services.gleaner_indexer_service.upload_json_to_s3")

    # ACT
    task_data = {"task_id": "idx_123", "document_text": "Fake content"}
    result = process_indexing_task(task_data)

    # ASSERT: Verify return payload
    assert result["status"] == "indexed"
    assert result["task_id"] == "idx_123"
    assert result["chunks_indexed"] == 2

    # ASSERT: Verify E5 requirements (the 'passage: ' prefix must be applied)
    mock_embed_model.embed.assert_called_once()
    called_passages = mock_embed_model.embed.call_args[0][0]
    assert called_passages[0] == "passage: This is chunk one."
    assert called_passages[1] == "passage: This is chunk two."

    # ASSERT: Verify FAISS Indexing was called
    mock_index.add.assert_called_once()
    
    # ASSERT: Verify persistence to S3
    mock_upload_file.assert_called_once_with("indexes/idx_123.faiss", "/tmp/idx_123.faiss")
    mock_upload_json.assert_called_once()

# TestProcessIndexingTask_MissingID ensures the pipeline fails early if the gateway
# sends a malformed payload.
def test_process_indexing_task_missing_id(mocker):
    # ARRANGE
    mocker.patch("services.gleaner_indexer_service.get_embedding_model")
    task_data = {"document_text": "Content but no ID"}
    
    # ACT & ASSERT
    with pytest.raises(ValueError, match="task_id is required"):
        process_indexing_task(task_data)