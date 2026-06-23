import os
from faster_whisper import WhisperModel

# Load the model globally so it stays in RAM across tasks, avoiding cold starts.
print("Loading Faster-Whisper model (small, int8)...")
MODEL = WhisperModel("small", device="cpu", compute_type="int8")
print("Model loaded successfully.")

def transcribe_audio(file_path: str) -> dict:
    """
    Processes the audio file through Faster-Whisper and returns the text.
    """
    if not os.path.exists(file_path):
        raise FileNotFoundError(f"Audio file not found at {file_path}")

    segments, info = MODEL.transcribe(file_path, beam_size=5)
    
    full_text = ""
    for segment in segments:
        full_text += segment.text + " "

    return {
        "language": info.language,
        "language_probability": info.language_probability,
        "text": full_text.strip()
    }