import pytest
from services.scriber_service import transcribe_audio

def test_transcribe_audio_concatenates_segments(mocker):
    # Mock os.path.exists to bypass the physical file check
    mocker.patch("os.path.exists", return_value=True)
    
    # Create fake segment objects mimicking Faster-Whisper's output
    class FakeSegment:
        def __init__(self, text):
            self.text = text

    class FakeInfo:
        language = "en"
        language_probability = 0.99
        
    mock_model = mocker.Mock()
    mock_model.transcribe.return_value = (
        [FakeSegment("This is"), FakeSegment("a test."), FakeSegment("Success.")],
        FakeInfo()
    )
    
    mocker.patch("services.scriber_service.get_whisper_model", return_value=mock_model)
    
    result = transcribe_audio("fake/path/audio.wav")
    
    assert result["language"] == "en"
    assert result["text"] == "This is a test. Success."

def test_transcribe_audio_file_not_found():
    with pytest.raises(FileNotFoundError):
        transcribe_audio("non_existent_file.wav")