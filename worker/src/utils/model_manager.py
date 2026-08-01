import os
from fastembed import TextEmbedding
from fastembed.common.model_description import PoolingType, ModelSource
from huggingface_hub import hf_hub_download
from faster_whisper import WhisperModel
from tokenizers import Tokenizer
from llama_cpp import Llama
from utils.logger import get_logger

logger = get_logger("model_manager")

# Global singleton instances
_EMBEDDING_MODEL = None
_TOKENIZER = None
_LLM = None
_CUSTOM_MODEL_REGISTERED = False
_WHISPER_MODEL = None

def get_embedding_model() -> TextEmbedding:
    """
    Lazy-loads and returns a singleton instance of the FastEmbed text embedding model.
    
    This function ensures that the custom intfloat/multilingual-e5-small model 
    is registered properly with the FastEmbed runtime before instantiation. It 
    maintains a single reference to conserve memory across the worker process.

    Returns:
        TextEmbedding: An initialized FastEmbed model ready for vector generation.
    """
    global _EMBEDDING_MODEL, _CUSTOM_MODEL_REGISTERED

    if not _CUSTOM_MODEL_REGISTERED:
        logger.info("Registering custom FastEmbed model: intfloat/multilingual-e5-small")
        TextEmbedding.add_custom_model(
            model="intfloat/multilingual-e5-small",
            pooling=PoolingType.MEAN,
            normalization=True,
            sources=ModelSource(hf="intfloat/multilingual-e5-small"),
            dim=384,
            model_file="onnx/model.onnx",
        )
        _CUSTOM_MODEL_REGISTERED = True
    
    if not _EMBEDDING_MODEL:
        logger.info("Loading FastEmbed model for retrieval...")
        _EMBEDDING_MODEL = TextEmbedding(model_name="intfloat/multilingual-e5-small")
        logger.info("FastEmbed retrieval model loaded successfully.")

    return _EMBEDDING_MODEL

def get_tokenizer() -> Tokenizer:
    """
    Lazy-loads and returns a singleton instance of the native Rust HuggingFace Tokenizer.
    
    Bypasses the monolithic `transformers` library to fetch and load the raw 
    tokenizer.json directly, drastically reducing memory footprint and load time.

    Returns:
        Tokenizer: A highly optimized Rust-based tokenizer for text chunking.
    """
    global _TOKENIZER

    if not _TOKENIZER:
        logger.info("Fetching tokenizer.json via huggingface_hub...")
        tokenizer_path = hf_hub_download(
            repo_id="intfloat/multilingual-e5-small",
            filename="onnx/tokenizer.json",
            cache_dir="/models/tokenizer",
            token=os.getenv("HF_TOKEN")
        )
        _TOKENIZER = Tokenizer.from_file(tokenizer_path)
        logger.info("Native Tokenizer loaded successfully.")

    return _TOKENIZER

def get_llm() -> Llama:
    """
    Lazy-loads and returns a singleton instance of the Llama.cpp (Qwen3) model.
    
    Uses memory-mapped (mmap) GGUF files to share OS-level memory, fitting a 
    0.6B parameter model comfortably within a 2GB RAM container constraint.

    Returns:
        Llama: An initialized Llama.cpp chat completion engine.
    """
    global _LLM

    if not _LLM:
        logger.info("Loading Llama.cpp (Qwen3-0.6B) for generation...")
        model_path = hf_hub_download(
            repo_id="bartowski/Qwen_Qwen3-0.6B-GGUF",
            filename="Qwen_Qwen3-0.6B-Q4_K_M.gguf",
            cache_dir="/models/qwen3",
            token=os.getenv("HF_TOKEN")
        )
        _LLM = Llama(
            model_path=model_path,
            n_ctx=3072,
            n_threads=2,
            verbose=False
        )
        logger.info("Llama.cpp model loaded successfully.")

    return _LLM

def get_whisper_model() -> WhisperModel:
    """
    Lazy-loads and returns a singleton instance of the Faster-Whisper audio model.

    Returns:
        WhisperModel: Initialized WhisperModel instance configured for CPU/int8.
    """
    global _WHISPER_MODEL
    if not _WHISPER_MODEL:
        logger.info("Loading Faster-Whisper model (small, int8)...")
        _WHISPER_MODEL = WhisperModel("small", device="cpu", compute_type="int8")
        logger.info("Faster-Whisper model loaded successfully.")
    return _WHISPER_MODEL