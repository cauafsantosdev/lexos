import os
from faster_whisper import WhisperModel
from utils.logger import get_logger


logger = get_logger("scriber")

logger.info("Loading Faster-Whisper model (small, int8)...")
try:
    # Enforce "cpu" and "int8" quantization
    MODEL = WhisperModel("small", device="cpu", compute_type="int8")
    logger.info("Faster-Whisper model loaded successfully.")
except Exception as e:
    logger.error(f"Failed to load Faster-Whisper: {e}")
    MODEL = None

def transcribe_audio(file_path: str) -> dict:
    """
    Processes an audio file through the local Faster-Whisper model and extracts the text.

    Args:
        file_path (str): The absolute or relative path to the audio file.

    Returns:
        dict: A dictionary containing the transcription details:
            - language (str): The detected language code (e.g., 'en', 'pt').
            - language_probability (float): Confidence score of the detected language (0.0 to 1.0).
            - text (str): The full transcribed text.

    Raises:
        RuntimeError: If the Faster-Whisper model failed to initialize on startup.
        FileNotFoundError: If the audio file does not exist at the specified path.
    """
    if not MODEL:
        raise RuntimeError("Faster-Whisper model is not initialized.")
        
    if not os.path.exists(file_path):
        raise FileNotFoundError(f"Audio file not found at {file_path}")

    logger.info(f"Starting audio transcription for {os.path.basename(file_path)}...")
    
    # beam_size=5 provides a balance between accuracy and speed.
    segments, info = MODEL.transcribe(file_path, beam_size=5)
    
    full_text = ""
    for segment in segments:
        full_text += segment.text + " "

    logger.info("Transcription completed successfully.")

    return {
        "language": info.language,
        "language_probability": info.language_probability,
        "text": full_text.strip()
    }