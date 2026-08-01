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
    Splits a document's text into overlapping, token-aware semantic chunks.

    This function uses token boundaries to determine chunk size but slices the 
    original string using exact character offsets. This guarantees zero mutation 
    of the text (preserving raw unicode, spacing, and punctuation) while strictly 
    adhering to the embedding model's context window.

    Args:
        text (str): The raw document text to be chunked.
        chunk_size (int, optional): Maximum tokens per chunk. Defaults to 350.
        overlap (int, optional): Token overlap between sequential chunks. Defaults to 50.

    Returns:
        list[dict]: A list of chunk dictionaries containing 'id', 'text', 'start_char', and 'end_char'.
    """
    tokenizer = get_tokenizer()
    encoding = tokenizer.encode(text)
    tokens = encoding.ids
    offsets = encoding.offsets  # List of (start_char, end_char) for every token

    if not tokens:
        return []

    chunks = []
    start = 0
    chunk_id = 0

    while start < len(tokens):
        end = min(start + chunk_size, len(tokens))
        
        # Extract the exact character boundaries from the token offsets
        start_char = offsets[start][0]
        end_char = offsets[end - 1][1]
        
        # Slice the original text to guarantee 100% fidelity (no space/unicode loss)
        chunk_text = text[start_char:end_char]
        
        chunks.append({
            "id": chunk_id,
            "text": chunk_text,
            "start_char": start_char,
            "end_char": end_char
        })
        
        chunk_id += 1
        if end == len(tokens):
            break
        start += (chunk_size - overlap)
        
    return chunks

def process_indexing_task(task_data: dict) -> dict:
    """
    Executes the document ingestion, chunking, and FAISS indexing pipeline.

    Extracts text from a provided file, generates token-aligned chunks, embeds them using the E5 passage format, 
    and builds an L2-normalized FAISS IndexFlatIP for cosine similarity retrieval.

    Args:
        task_data (dict): Queue payload containing 'task_id' and 'file_path'/'document_text'.

    Raises:
        ValueError: If essential task parameters or document text are missing.

    Returns:
        dict: A status summary containing indexing metrics and file paths.
    """
    embedding_model = get_embedding_model()
    
    file_path = task_data.get("file_path")
    task_id = task_data.get("task_id")
    
    if not task_id:
        raise ValueError("task_id is required for indexing.")
        
    doc_text = extract_text(file_path) if file_path else task_data.get("document_text")
    if not doc_text:
        raise ValueError("No text provided or extracted from document.")

    logger.info(f"Chunking document {task_id} (Length: {len(doc_text)} chars)...")
    chunks = _token_chunk_text(doc_text, chunk_size=350, overlap=int(350 * 0.15))
    logger.info(f"Generated {len(chunks)} token-aligned chunks.")

    # E5 models strictly require the "passage: " prefix and stripped text
    passages = [f"passage: {c['text'].strip()}" for c in chunks]
    
    logger.info("Generating vector embeddings via FastEmbed...")
    embeddings_generator = embedding_model.embed(passages)
    
    # Cast to float32 to guarantee FAISS compatibility
    embeddings = np.vstack(list(embeddings_generator)).astype(np.float32)
    
    # Normalize embeddings for Cosine Similarity equivalence via FAISS IndexFlatIP
    faiss.normalize_L2(embeddings)

    logger.info("Building FAISS index...")
    dimension = embeddings.shape[-1]
    index = faiss.IndexFlatIP(dimension)
    index.add(embeddings)

    # Persist FAISS vector index
    index_path = f"/tmp/{task_id}.faiss"
    faiss.write_index(index, index_path)

    # Persist rich metadata registry with model verification data
    metadata = {
        "task_id": task_id,
        "embedding_model": "intfloat/multilingual-e5-small",
        "embedding_dimension": int(dimension),
        "normalized": True,
        "total_chunks": len(chunks),
        "chunks": chunks
    }
    
    upload_file_to_s3(f"indexes/{task_id}.faiss", index_path)
    upload_json_to_s3(f"indexes/{task_id}_meta.json", metadata)
    
    # Cleanup local tmp
    os.remove(index_path)

    logger.info(f"Indexing complete! Saved to MinIO under indexes/{task_id}")

    return {
        "status": "indexed",
        "task_id": task_id,
        "chunks_indexed": len(chunks)
    }