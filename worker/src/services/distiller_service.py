import os
import re
from huggingface_hub import hf_hub_download
from llama_cpp import Llama
from utils.logger import get_logger
from utils.file_parser import extract_text
from utils.prompt_builder import build_map_system_prompt, build_map_user_prompt, build_reduce_system_prompt, build_reduce_user_prompt

logger = get_logger("distiller")

MODEL_REPO = "bartowski/Qwen_Qwen3-0.6B-GGUF"
MODEL_FILE = "Qwen_Qwen3-0.6B-Q4_K_M.gguf"
MODEL_CACHE_DIR = "/models"

logger.info(f"Checking for {MODEL_FILE} in cache...")
try:
    os.makedirs(MODEL_CACHE_DIR, exist_ok=True)
    model_path = hf_hub_download(
        repo_id=MODEL_REPO,
        filename=MODEL_FILE,
        cache_dir=MODEL_CACHE_DIR
    )
    
    logger.info("Initializing Llama.cpp Engine...")
    llm = Llama(
        model_path=model_path,
        n_ctx=3072,
        n_threads=2,
        n_threads_batch=4,
        n_batch=512,
        verbose=False
    )
    logger.info("Llama.cpp initialized successfully.")
except Exception as e:
    logger.error(f"Failed to load GGUF model: {e}")
    llm = None

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

def _chunk_text(text: str, chunk_size: int = 6500, overlap: int = 500) -> list[str]:
    """
    Splits large text into manageable overlapping chunks to fit within the LLM context window.

    Args:
        text (str): The full document text to be chunked.
        chunk_size (int, optional): The maximum character length of each chunk. Defaults to 6500.
        overlap (int, optional): The number of characters to overlap between chunks to preserve context. Defaults to 500.

    Returns:
        list[str]: A list of text chunks.
    """
    chunks = []
    start = 0
    while start < len(text):
        end = start + chunk_size
        chunks.append(text[start:end])
        start += chunk_size - overlap
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
    messages = [
        {"role": "system", "content": system_prompt},
        {"role": "user", "content": user_prompt}
    ]
    
    response = llm.create_chat_completion(
        messages=messages,
        max_tokens=max_tokens,
        temperature=0
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
    
    if not llm:
        raise RuntimeError("Llama.cpp model pipeline is not initialized.")

    # Hierarchical map-reduce logic
    if original_text_length > 6500:
        logger.info(f"Document length ({original_text_length} chars) exceeds single window. Initiating Map-Reduce...")
        chunks = _chunk_text(doc_text)
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
        # Replace newlines with spaces
        final_summary = " ".join(final_summary.splitlines())
        # Remove any residual markdown list tokens or inline labels if present
        final_summary = re.sub(r'^\s*[-*•]\s*', '', final_summary)
        # Clean up multiple spaces
        final_summary = re.sub(r'\s+', ' ', final_summary).strip()

    return {
        "model": MODEL_FILE,
        "style": style,
        "original_length": original_text_length,
        "summary": final_summary
    }