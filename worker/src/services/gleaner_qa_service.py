import json
import os
import redis
import numpy as np
import faiss
from functools import lru_cache
from utils.logger import get_logger
from utils.model_manager import get_embedding_model, get_llm
from utils.s3 import get_s3_client


logger = get_logger("gleaner_qa")

@lru_cache(maxsize=20)
def _get_index_and_meta(artifact_id: str, index_s3_key: str, meta_s3_key: str) -> tuple:
    """
    Retrieves a content-addressed FAISS index and metadata registry.

    An LRU cache retains the most recently queried artifacts and avoids repeated
    downloads for active documents while bounding process memory usage.

    Args:
        artifact_id (str): Stable processing fingerprint for the indexed document.
        index_s3_key (str): Object key for the FAISS index.
        meta_s3_key (str): Object key for the chunk metadata registry.

    Returns:
        tuple: Loaded FAISS index and metadata dictionary.
    """
    local_index_path = f"/tmp/{artifact_id}.faiss"
    local_meta_path = f"/tmp/{artifact_id}_meta.json"
    client, bucket = get_s3_client()

    # Download content-addressed artifacts only when they are absent from the local LRU-backed cache.
    if not os.path.exists(local_index_path):
        logger.info(f"Downloading FAISS index for artifact {artifact_id}...")
        client.fget_object(bucket, index_s3_key, local_index_path)

    if not os.path.exists(local_meta_path):
        client.fget_object(bucket, meta_s3_key, local_meta_path)

    index = faiss.read_index(local_index_path)
    with open(local_meta_path, "r", encoding="utf-8") as handle:
        metadata = json.load(handle)

    return index, metadata


def _assemble_context(retrieved_items: list) -> str:
    """
    Assembles retrieved semantic chunks into citation-ready context blocks.

    Chunks are restored to document order and physically adjacent passages are
    merged to improve narrative continuity without introducing unrelated text.

    Args:
        retrieved_items (list): Retrieval entries containing chunk metadata and scores.

    Returns:
        str: Formatted context string for grounded generation.
    """
    # Restore document order after relevance-ranked FAISS retrieval.
    sorted_items = sorted(retrieved_items, key=lambda item: item["chunk"]["id"])
    assembled_blocks = []
    current_group = []

    for item in sorted_items:
        chunk = item["chunk"]
        if not current_group:
            current_group.append(chunk)
            continue

        last_chunk = current_group[-1]
        # Merge physically adjacent chunks to recover local narrative continuity.
        if abs(chunk["start_char"] - last_chunk["end_char"]) < 20:
            current_group.append(chunk)
        else:
            assembled_blocks.append(current_group)
            current_group = [chunk]

    if current_group:
        assembled_blocks.append(current_group)

    context_blocks = []
    for group in assembled_blocks:
        if len(group) == 1:
            context_blocks.append(f"[Chunk {group[0]['id']}]: {group[0]['text']}")
            continue

        start_id = group[0]["id"]
        end_id = group[-1]["id"]
        combined_text = " ".join(chunk["text"] for chunk in group)
        context_blocks.append(f"[Chunks {start_id}-{end_id}]: {combined_text}")

    return "\n\n".join(context_blocks)


def _build_prompt(query: str, context_str: str) -> tuple[str, str]:
    """
    Constructs a hallucination-resistant prompt for grounded document QA.

    Args:
        query (str): Question submitted against the indexed document.
        context_str (str): Retrieved document evidence.

    Returns:
        tuple[str, str]: System and user prompt pair.
    """
    system_prompt = """You are Lexos Gleaner, an expert AI research assistant.
Answer the question based ONLY on the provided context.

CRITICAL RULES:
1. If the answer cannot be found in the context, explicitly say: \"I cannot find the answer in the provided document.\" Do NOT guess or use outside knowledge.
2. If multiple chunks support the answer, cite ALL of them.
3. Never cite a chunk that was not provided in the context.
4. Keep the answer concise, objective, and strictly factual.
5. Never output <think> tags or internal reasoning."""

    user_prompt = f"""/no_think
CONTEXT:
{context_str}

QUESTION:
{query}

Answer the question strictly using the context above."""

    return system_prompt, user_prompt


def _clean_stream_delta(delta: str, state: dict[str, object]) -> str:
    """
    Removes Qwen thinking blocks from streaming output across token boundaries.

    Args:
        delta (str): Newly generated stream fragment.
        state (dict[str, object]): Mutable parser state containing buffer and mode.

    Returns:
        str: Safe answer text ready for publication.
    """
    buffer = str(state.get("buffer", "")) + delta
    is_thinking = bool(state.get("is_thinking", False))
    output = []

    while buffer:
        if is_thinking:
            end_index = buffer.find("</think>")
            if end_index == -1:
                keep = min(len(buffer), len("</think>") - 1)
                state["buffer"] = buffer[-keep:] if keep else ""
                state["is_thinking"] = True
                return "".join(output)
            buffer = buffer[end_index + len("</think>"):]
            is_thinking = False
            continue

        start_index = buffer.find("<think>")
        if start_index == -1:
            suffix_length = 0
            for length in range(1, min(len(buffer), len("<think>") - 1) + 1):
                if "<think>".startswith(buffer[-length:]):
                    suffix_length = length
            if suffix_length:
                output.append(buffer[:-suffix_length])
                state["buffer"] = buffer[-suffix_length:]
            else:
                output.append(buffer)
                state["buffer"] = ""
            state["is_thinking"] = False
            return "".join(output)

        output.append(buffer[:start_index])
        buffer = buffer[start_index + len("<think>"):]
        is_thinking = True

    state["buffer"] = ""
    state["is_thinking"] = is_thinking
    return "".join(output)


def process_qa_task(task_data: dict) -> dict:
    """
    Executes vector retrieval and streams grounded Qwen3 answers through Redis.

    Args:
        task_data (dict): Queue payload containing task_id, document_id,
            artifact_id, index_s3_key, meta_s3_key, and query.

    Raises:
        ValueError: If mandatory identifiers or query text are missing.

    Returns:
        dict: Final answer payload with retrieval source scores.
    """
    embedding_model = get_embedding_model()
    llm = get_llm()

    task_id = task_data.get("task_id")
    document_id = task_data.get("document_id")
    artifact_id = task_data.get("artifact_id") or document_id
    index_s3_key = task_data.get("index_s3_key")
    meta_s3_key = task_data.get("meta_s3_key")
    query = task_data.get("query")

    if not all([task_id, document_id, artifact_id, index_s3_key, meta_s3_key, query]):
        raise ValueError(
            "task_id, document_id, artifact_id, index_s3_key, meta_s3_key, and query are required."
        )

    index, metadata = _get_index_and_meta(artifact_id, index_s3_key, meta_s3_key)

    # E5 query embeddings require the explicit "query: " prefix.
    logger.info(f"Embedding query for artifact {artifact_id}: '{query}'")
    query_embedding = next(embedding_model.embed([f"query: {query}"])).astype(np.float32)
    query_embedding = np.expand_dims(query_embedding, axis=0)
    faiss.normalize_L2(query_embedding)

    # Retrieve at most five candidates and never request more neighbors than the index contains.
    k = min(5, int(index.ntotal))
    if k <= 0:
        retrieved_items = []
    else:
        distances, indices = index.search(query_embedding, k)
        retrieved_items = []
        for position, idx in enumerate(indices[0]):
            if idx == -1:
                continue
            score = float(distances[0][position])
            if score >= 0.45:
                retrieved_items.append({
                    "chunk": metadata["chunks"][idx],
                    "score": score,
                })

    # Preserve descending retrieval scores for citations and debugging metadata.
    retrieved_items.sort(key=lambda item: item["score"], reverse=True)

    # Use a dedicated Redis connection for publishing SSE token events.
    redis_url = os.getenv("REDIS_URL", "redis://redis:6379")
    if not redis_url.startswith("redis://") and not redis_url.startswith("rediss://"):
        redis_url = f"redis://{redis_url}"
    redis_client = redis.from_url(
        redis_url,
        decode_responses=True,
        socket_keepalive=True,
        socket_timeout=None,
        health_check_interval=30,
    )
    stream_channel = f"lexos:stream:{task_id}"

    # Return the grounded fallback immediately when retrieval provides no admissible evidence.
    if not retrieved_items:
        fallback = "I cannot find the answer in the provided document."
        redis_client.publish(stream_channel, json.dumps({"token": fallback}))
        redis_client.publish(stream_channel, "[DONE]")
        return {
            "task_id": task_id,
            "document_id": document_id,
            "artifact_id": artifact_id,
            "query": query,
            "answer": fallback,
            "sources": [],
        }

    # Assemble only retrieved evidence into the generation context.
    context_str = _assemble_context(retrieved_items)
    system_prompt, user_prompt = _build_prompt(query, context_str)
    messages = [
        {"role": "system", "content": system_prompt},
        {"role": "user", "content": user_prompt},
    ]

    logger.info(f"Starting Qwen3 streaming generation for {task_id}...")
    # Deterministic generation settings keep grounded QA behavior reproducible.
    streamer = llm.create_chat_completion(
        messages=messages,
        max_tokens=1024,
        temperature=0.0,
        seed=42,
        stream=True,
    )

    full_answer = ""
    think_state: dict[str, object] = {"buffer": "", "is_thinking": False}

    for chunk in streamer:
        delta = chunk["choices"][0]["delta"].get("content", "")
        if not delta:
            continue

        # Suppress internal thinking blocks even when tags are split across stream chunks.
        safe_delta = _clean_stream_delta(delta, think_state)
        if not safe_delta:
            continue
        if not full_answer and safe_delta.strip() == "":
            continue

        full_answer += safe_delta
        redis_client.publish(stream_channel, json.dumps({"token": safe_delta}))

    trailing = str(think_state.get("buffer", ""))
    if trailing and not bool(think_state.get("is_thinking", False)):
        full_answer += trailing
        redis_client.publish(stream_channel, json.dumps({"token": trailing}))

    # Guarantee a user-visible terminal answer when the model spends the full budget without output.
    if not full_answer.strip():
        fallback = "The model exhausted the generation budget before producing a final answer."
        full_answer = fallback
        redis_client.publish(stream_channel, json.dumps({"token": fallback}))

    redis_client.publish(stream_channel, "[DONE]")
    logger.info("Generation complete and [DONE] flag sent.")

    # Return retrieval scores with the final answer for traceability and tests.
    return {
        "task_id": task_id,
        "document_id": document_id,
        "artifact_id": artifact_id,
        "query": query,
        "answer": full_answer.strip(),
        "sources": [
            {
                "chunk_id": item["chunk"]["id"],
                "score": round(item["score"], 4),
            }
            for item in retrieved_items
        ],
    }