import os
import json
import redis
import numpy as np
import faiss
from functools import lru_cache
from utils.logger import get_logger
from utils.model_manager import get_embedding_model, get_llm


logger = get_logger("gleaner_qa")

@lru_cache(maxsize=20)
def _get_index_and_meta(document_id: str) -> tuple:
    """
    Retrieves the FAISS vector index and rich metadata registry from disk.

    Utilizes an LRU in-memory cache to eliminate disk I/O latency for frequently 
    queried documents. Safely caps memory usage to the 20 most recent documents.

    Args:
        document_id (str): The unique identifier for the processed document.

    Raises:
        FileNotFoundError: If the index or metadata file does not exist.

    Returns:
        tuple: A tuple containing the loaded (faiss.IndexFlatIP, dict metadata).
    """
    index_path = os.path.join("/uploads", f"{document_id}.faiss")
    meta_path = os.path.join("/uploads", f"{document_id}_meta.json")
    
    if not os.path.exists(index_path) or not os.path.exists(meta_path):
        raise FileNotFoundError(f"Document index {document_id} not found. Please index it first.")
        
    logger.info(f"Loading FAISS index for document {document_id} from disk...")
    index = faiss.read_index(index_path)
    
    with open(meta_path, "r", encoding="utf-8") as f:
        metadata = json.load(f)
        
    return index, metadata

def _assemble_context(retrieved_items: list) -> str:
    """
    Assembles retrieved semantic chunks into a continuous context block.

    Sorts chunks chronologically and merges physically adjacent passages (within 
    20 characters of each other) to restore the original narrative flow and 
    conserve LLM token budget.

    Args:
        retrieved_items (list): A list of dictionaries containing 'chunk' data.

    Returns:
        str: A cleanly formatted, citation-ready string of contextual passages.
    """
    # Sort by chunk ID to restore chronological order
    sorted_items = sorted(retrieved_items, key=lambda x: x["chunk"]["id"])
    assembled_blocks = []
    current_group = []

    for item in sorted_items:
        chunk = item["chunk"]
        if not current_group:
            current_group.append(chunk)
        else:
            last_chunk = current_group[-1]
            # Merge if the chunks are physically adjacent in the original text (within 20 characters)
            if abs(chunk["start_char"] - last_chunk["end_char"]) < 20:
                current_group.append(chunk)
            else:
                assembled_blocks.append(current_group)
                current_group = [chunk]
                
    if current_group:
        assembled_blocks.append(current_group)

    context_strs = []
    for group in assembled_blocks:
        if len(group) == 1:
            context_strs.append(f"[Chunk {group[0]['id']}]: {group[0]['text']}")
        else:
            start_id = group[0]['id']
            end_id = group[-1]['id']
            combined_text = " ".join([c['text'] for c in group])
            context_strs.append(f"[Chunks {start_id}-{end_id}]: {combined_text}")

    return "\n\n".join(context_strs)

def _build_prompt(query: str, context_str: str) -> tuple[str, str]:
    """
    Constructs a rigid, hallucination-resistant prompt for the RAG generation phase.

    Enforces strict grounding rules, forcing the LLM to decline answering if the 
    context is insufficient, and demanding inline source citations.

    Args:
        query (str): The user's input question.
        context_str (str): The assembled semantic context string.

    Returns:
        tuple[str, str]: The (System Prompt, User Prompt) message pair.
    """
    system_prompt = """You are Lexos Gleaner, an expert AI research assistant.
You will answer the user's question based ONLY on the provided context.

CRITICAL RULES:
1. If the answer cannot be found in the context, explicitly say: "I cannot find the answer in the provided document." Do NOT guess or use outside knowledge.
2. If multiple chunks support the answer, cite ALL of them.
3. Never cite a chunk that was not provided in the context.
4. Keep the answer concise, objective, and strictly factual.
5. Never output <think> tags or internal reasoning."""

    user_prompt = f"""CONTEXT:
{context_str}

QUESTION:
{query}

Answer the question strictly using the context above."""

    return system_prompt, user_prompt

def process_qa_task(task_data: dict) -> dict:
    """
    Executes the Vector QA pipeline and handles Server-Sent Event (SSE) streaming.

    Retrieves highly relevant semantic chunks via FAISS Cosine Similarity, formulates 
    a strict grounding prompt, executes Qwen3 inference, filters internal reasoning 
    tags on-the-fly, and streams the clean response token-by-token via Redis Pub/Sub.

    Args:
        task_data (dict): Queue payload containing 'task_id', 'document_id', and 'query'.

    Raises:
        ValueError: If mandatory query parameters are missing.

    Returns:
        dict: A finalized summary payload with the full answer and citation scores.
    """
    embedding_model = get_embedding_model()
    llm = get_llm()
    
    task_id = task_data.get("task_id")
    document_id = task_data.get("document_id")
    query = task_data.get("query")
    
    if not all([task_id, document_id, query]):
        raise ValueError("task_id, document_id, and query are all required.")
        
    index, metadata = _get_index_and_meta(document_id)
        
    # Embed the query (Point 14)
    logger.info(f"Embedding query: '{query}'")
    query_embedding = next(embedding_model.embed([f"query: {query}"])).astype(np.float32)
    query_embedding = np.expand_dims(query_embedding, axis=0)
    faiss.normalize_L2(query_embedding)
    
    # Retrieve Top-5 chunks
    k = 5
    distances, indices = index.search(query_embedding, k)
    
    retrieved_items = []
    for i, idx in enumerate(indices[0]):
        if idx != -1:
            score = float(distances[0][i])
            if score >= 0.45:  # Filter out low-relevance garbage
                retrieved_items.append({
                    "chunk": metadata["chunks"][idx],
                    "score": score
                })
                
    # Explicitly sort by highest score first
    retrieved_items.sort(key=lambda x: x["score"], reverse=True)

    # Empty context check (Point 13)
    if not retrieved_items:
        return {
            "task_id": task_id,
            "answer": "I cannot find the answer in the provided document.",
            "sources_used": []
        }
            
    # Assemble context (Point 3)
    context_str = _assemble_context(retrieved_items)
            
    system_prompt, user_prompt = _build_prompt(query, context_str)
    messages = [
        {"role": "system", "content": system_prompt},
        {"role": "user", "content": user_prompt}
    ]
    
    # Robust Redis connection (Point 7)
    redis_url = os.getenv("REDIS_URL", "redis://redis:6379")
    if not redis_url.startswith("redis://"):
        redis_url = f"redis://{redis_url}"
    r = redis.from_url(
            redis_url, 
            decode_responses=True, 
            socket_keepalive=True,
            socket_timeout=None,
            health_check_interval=30
        )
    stream_channel = f"lexos:stream:{task_id}"
    
    logger.info(f"Starting Qwen3 streaming generation for {task_id}...")
    
    # Generation parameters (Points 9, 10, 11)
    streamer = llm.create_chat_completion(
        messages=messages,
        max_tokens=1024,
        temperature=0.0,
        seed=42,
        stream=True
    )
    
    full_answer = ""
    is_thinking = False

    for chunk in streamer:
        delta = chunk["choices"][0]["delta"].get("content", "")
        if not delta:
            continue

        # Toggle thinking state
        if "<think>" in delta:
            is_thinking = True
            delta = delta.replace("<think>", "")
            
        if "</think>" in delta:
            is_thinking = False
            delta = delta.replace("</think>", "")
            
        # If the model is currently thinking, swallow the tokens
        if is_thinking:
            continue

        # Prevent empty leading newlines right after it finishes thinking
        if not full_answer and delta.strip() == "":
            continue

        if delta:
            full_answer += delta
            r.publish(stream_channel, json.dumps({"token": delta}))

    # If the model spent ALL tokens thinking and never outputted a final answer
    if not full_answer.strip():
        fallback_msg = "The model ran out of token budget during reasoning. Please try narrowing your question."
        full_answer = fallback_msg
        r.publish(stream_channel, json.dumps({"token": fallback_msg}))
            
    r.publish(stream_channel, "[DONE]")
    logger.info("Generation complete and [DONE] flag sent.")
    
    # Return JSON with rich citation metrics (Point 15)
    return {
        "task_id": task_id,
        "document_id": document_id,
        "query": query,
        "answer": full_answer.strip(),
        "sources": [
            {
                "chunk_id": item["chunk"]["id"], 
                "score": round(item["score"], 4)
            } for item in retrieved_items
        ]
    }