import re
import time
from utils.logger import get_logger
from utils.model_manager import get_llm
from utils.file_parser import extract_text
from utils.prompt_builder import build_map_system_prompt, build_map_user_prompt, build_reduce_system_prompt, build_reduce_user_prompt


logger = get_logger("distiller")

def _sanitize_document(text: str) -> str:
    """
    Heuristically removes known noise sections (like References) from formal documents.
    Gracefully ignores if the headers are not found (e.g., non-academic texts).

    Args:
        text (str): The raw text extracted from the document.

    Returns:
        str: The sanitized text with trailing reference sections removed.
    """
    # Regex to catch "\nREFERENCES\n", "\nVI. REFERENCES\n", "\nBibliography\n", etc.
    ref_pattern = re.compile(r'\n(?:\d+\.?|[IVX]+\.)?\s*(?:REFERENCES|References|BIBLIOGRAPHY|Bibliography|WORKS CITED)\s*\n')
    
    match = ref_pattern.search(text)
    if match:
        logger.info("Found References/Bibliography section. Sanitizing document tail...")
        return text[:match.start()].strip()
    
    return text

def _chunk_text_by_tokens(
    text: str,
    llm,
    chunk_size: int = 1500,
    overlap: int = 96,
) -> list[str]:
    """
    Splits text into overlapping chunks using the generation model tokenizer.

    Token-aware chunking keeps each Map input within a known token budget and
    avoids relying on the variable relationship between characters and model
    tokens.

    Args:
        text (str): The full document text to be chunked.
        llm: Loaded Llama.cpp model instance providing the Qwen tokenizer.
        chunk_size (int, optional): Maximum number of source tokens per chunk.
            Defaults to MAP_CHUNK_SIZE_TOKENS.
        overlap (int, optional): Number of tokens shared between consecutive
            chunks. Defaults to MAP_CHUNK_OVERLAP_TOKENS.

    Returns:
        list[str]: Ordered text chunks reconstructed from Qwen token sequences.

    Raises:
        ValueError: If the chunk size or overlap configuration is invalid.
    """
    if chunk_size <= 0:
        raise ValueError("chunk_size must be greater than zero")

    if overlap < 0:
        raise ValueError("overlap cannot be negative")

    if overlap >= chunk_size:
        raise ValueError("overlap must be smaller than chunk_size")

    if not text:
        return []

    tokens = llm.tokenize(
        text.encode("utf-8"),
        add_bos=False,
        special=False,
    )

    if not tokens:
        return []

    chunks = []
    step = chunk_size - overlap

    for start in range(0, len(tokens), step):
        end = min(start + chunk_size, len(tokens))
        chunk_tokens = tokens[start:end]

        chunk = llm.detokenize(
            chunk_tokens,
            special=False,
        ).decode("utf-8", errors="replace").strip()

        if chunk:
            chunks.append(chunk)

        if end == len(tokens):
            break

    return chunks

def _run_inference(system_prompt: str, user_prompt: str, max_tokens: int = 512) -> str:
    """
    Helper function to execute Llama.cpp chat completion and sanitize model output.

    Args:
        system_prompt (str): The system instructions defining the model's behavior.
        user_prompt (str): The user input or data payload.
        max_tokens (int, optional): Maximum tokens allowed for generation. Defaults to 512.

    Returns:
        str: The generated text, stripped of residual formatting or reasoning tags.
    """
    llm = get_llm()

    messages = [
        {"role": "system", "content": system_prompt},
        {"role": "user", "content": user_prompt}
    ]

    started_at = time.perf_counter()
    
    response = llm.create_chat_completion(
        messages=messages,
        max_tokens=max_tokens,
        temperature=0
    )

    elapsed = time.perf_counter() - started_at

    usage = response.get("usage", {})

    logger.info(
        "Inference completed in %.2fs "
        "(prompt_tokens=%s, completion_tokens=%s, total_tokens=%s)",
        elapsed,
        usage.get("prompt_tokens", "unknown"),
        usage.get("completion_tokens", "unknown"),
        usage.get("total_tokens", "unknown"),
    )
    
    raw_output = response["choices"][0]["message"]["content"].strip()
    
    # Strip any residual thinking tags mechanically
    if "<think>" in raw_output and "</think>" in raw_output:
        raw_output = raw_output.split("</think>")[-1].strip()
        
    return raw_output

def process_summarization_task(task_data: dict) -> dict:
    """
    Orchestrates the Map-Reduce hierarchical summarization pipeline.
    For documents exceeding the context window, it extracts facts from chunks (Map) 
    and synthesizes them into a final summary (Reduce).

    Args:
        task_data (dict): Payload containing processing instructions. Expected keys:
            - file_path (str, optional): Path to a local document file.
            - document_text (str, optional): Raw text if a file is not provided.
            - style (str, optional): Summary style ('bullet_points', 'short_paragraph', 'executive'). Defaults to 'bullet_points'.

    Returns:
        dict: A dictionary containing the summary results:
            - model (str): Name of the model used for inference.
            - style (str): The style applied to the summary.
            - original_length (int): Character count of the sanitized source document.
            - summary (str): The final generated summary.

    Raises:
        ValueError: If neither 'file_path' nor 'document_text' yields valid text.
        RuntimeError: If the Llama.cpp model engine failed to initialize.
    """
    file_path = task_data.get("file_path")
    doc_text = extract_text(file_path) if file_path else task_data.get("document_text")
    
    if not doc_text:
        raise ValueError("No text provided or extracted from document.")

    # Sanitize the document to remove trailing references or bibliography sections, if present.
    doc_text = _sanitize_document(doc_text)
    original_text_length = len(doc_text)
    style = task_data.get("style", "bullet_points")

    llm = get_llm()
    document_tokens = llm.tokenize(
        doc_text.encode("utf-8"),
        add_bos=False,
        special=False,
    )
    document_token_count = len(document_tokens)

    # Hierarchical map-reduce logic
    if document_token_count > 1500:
        logger.info(f"Document length ({document_token_count} tokens) exceeds single window. Initiating Map-Reduce...")
        chunks = _chunk_text_by_tokens(doc_text, llm)
        intermediate_facts = []
        
        map_system_prompt = build_map_system_prompt()
        
        for idx, chunk in enumerate(chunks):
            logger.info(f"Processing chunk {idx + 1}/{len(chunks)} (Fact Extraction)...")
            map_user_prompt = build_map_user_prompt(chunk)
            
            # Map Stage: Extract facts
            extracted_chunk = _run_inference(map_system_prompt, map_user_prompt, 256)
            intermediate_facts.append(extracted_chunk)
            
        # Combine extracted facts
        doc_text = "\n\n---\n\n".join(intermediate_facts)
        logger.info("Map phase complete. Executing Reduce synthesis...")

    # Reduce Stage (or single pass for small documents): Final generation
    reduce_system_prompt = build_reduce_system_prompt(style)
    reduce_user_prompt = build_reduce_user_prompt(doc_text)
    final_summary = _run_inference(reduce_system_prompt, reduce_user_prompt, 1024)

    # Post-Processing for short_paragraph style
    if style == "short_paragraph":
        # Remove markdown bullets at the start of each line
        lines = final_summary.splitlines()
        cleaned_lines = [re.sub(r'^\s*[-*]\s*', '', line) for line in lines]
        
        # Replace newlines with spaces
        final_summary = " ".join(cleaned_lines)
        
        # Clean up multiple spaces
        final_summary = re.sub(r'\s+', ' ', final_summary).strip()

    return {
        "model": "Qwen_Qwen3-0.6B-Q4_K_M.gguf",
        "style": style,
        "original_length": original_text_length,
        "summary": final_summary
    }