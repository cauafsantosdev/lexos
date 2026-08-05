import os
import faiss
import numpy as np
from utils.logger import get_logger
from utils.file_parser import extract_text
from utils.model_manager import get_embedding_model, get_tokenizer
from utils.s3 import upload_file_to_s3, upload_json_to_s3


logger = get_logger("gleaner_indexer")


def _token_chunk_text(text: str, chunk_size: int = 350, overlap: int = 50) -> list[dict]:
    """
    Splits document text into overlapping token-aware semantic chunks.

    Token boundaries determine chunk size while exact character offsets slice the
    source string. The resulting chunks preserve source Unicode, spacing, and
    punctuation without tokenizer decode mutations.

    Args:
        text (str): Raw document text.
        chunk_size (int, optional): Maximum tokens per chunk. Defaults to 350.
        overlap (int, optional): Token overlap between sequential chunks. Defaults to 50.

    Returns:
        list[dict]: Chunk dictionaries containing id, text, start_char, and end_char.
    """
    tokenizer = get_tokenizer()
    encoding = tokenizer.encode(text)
    tokens = encoding.ids
    offsets = encoding.offsets

    if not tokens:
        return []

    chunks = []
    start = 0
    chunk_id = 0

    while start < len(tokens):
        end = min(start + chunk_size, len(tokens))
        # Extract exact source boundaries from tokenizer offsets rather than decoding token IDs.
        start_char = offsets[start][0]
        end_char = offsets[end - 1][1]
        # Slice the original string to preserve Unicode, whitespace, and punctuation exactly.
        chunk_text = text[start_char:end_char]

        if chunk_text.strip():
            chunks.append({
                "id": chunk_id,
                "text": chunk_text,
                "start_char": start_char,
                "end_char": end_char,
            })
            chunk_id += 1

        if end == len(tokens):
            break
        start += chunk_size - overlap

    return chunks


def process_indexing_task(task_data: dict) -> dict:
    """
    Executes document ingestion, chunking, embedding, and FAISS persistence.

    The queue payload supplies content-addressed artifact keys derived from the
    processing fingerprint. Repeated documents can therefore reuse the same
    index and metadata objects without duplicating embedding computation.

    Args:
        task_data (dict): Queue payload containing task identifiers, input source,
            artifact identifiers, and destination object keys.

    Raises:
        ValueError: If required identifiers, artifact keys, or document text are missing.

    Returns:
        dict: Stable indexing metadata suitable for cache reuse.
    """
    embedding_model = get_embedding_model()

    file_path = task_data.get("file_path")
    task_id = task_data.get("task_id")
    artifact_id = task_data.get("artifact_id") or task_data.get("fingerprint")
    index_s3_key = task_data.get("index_s3_key")
    meta_s3_key = task_data.get("meta_s3_key")

    if not task_id:
        raise ValueError("task_id is required for indexing.")
    if not artifact_id:
        raise ValueError("artifact_id is required for content-addressed indexing.")
    if not index_s3_key or not meta_s3_key:
        raise ValueError("index_s3_key and meta_s3_key are required for indexing.")

    doc_text = extract_text(file_path) if file_path else task_data.get("document_text")
    if not doc_text:
        raise ValueError("No text provided or extracted from document.")

    chunk_size = 350
    overlap = int(chunk_size * 0.15)
    logger.info(f"Chunking artifact {artifact_id} (Length: {len(doc_text)} chars)...")
    chunks = _token_chunk_text(doc_text, chunk_size=chunk_size, overlap=overlap)
    if not chunks:
        raise ValueError("Document did not produce any indexable text chunks.")
    logger.info(f"Generated {len(chunks)} token-aligned chunks.")

    # E5 passage embeddings require the explicit "passage: " prefix.
    passages = [f"passage: {chunk['text'].strip()}" for chunk in chunks]

    logger.info("Generating vector embeddings via FastEmbed...")
    embeddings_generator = embedding_model.embed(passages)
    # FAISS expects float32 vectors; FastEmbed output is normalized explicitly below.
    embeddings = np.vstack(list(embeddings_generator)).astype(np.float32)
    # L2 normalization makes IndexFlatIP equivalent to cosine-similarity ranking.
    faiss.normalize_L2(embeddings)

    dimension = embeddings.shape[-1]
    index = faiss.IndexFlatIP(dimension)
    index.add(embeddings)

    # The local FAISS file is temporary; object storage is the durable derived-artifact layer.
    index_path = f"/tmp/{artifact_id}.faiss"
    try:
        faiss.write_index(index, index_path)

        # Persist model and chunking metadata beside the index for reproducible retrieval.
        metadata = {
            "artifact_id": artifact_id,
            "embedding_model": "intfloat/multilingual-e5-small",
            "embedding_dimension": int(dimension),
            "normalized": True,
            "chunk_size": chunk_size,
            "chunk_overlap": overlap,
            "total_chunks": len(chunks),
            "chunks": chunks,
        }

        # Upload both index and metadata under the same content-addressed fingerprint prefix.
        upload_file_to_s3(index_s3_key, index_path)
        upload_json_to_s3(meta_s3_key, metadata)
    finally:
        # Guarantee cleanup even when an object-storage upload fails.
        if os.path.exists(index_path):
            os.remove(index_path)

    logger.info(f"Indexing complete for artifact {artifact_id}.")

    return {
        "status": "indexed",
        "artifact_id": artifact_id,
        "chunks_indexed": len(chunks),
    }