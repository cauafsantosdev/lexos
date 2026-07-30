import os


def extract_text(file_path: str) -> str:
    """
    Extracts raw text from .txt, .pdf, or .docx files.

    Args:
        file_path (str): The absolute or relative path to the document file.

    Returns:
        str: The extracted raw text, stripped of leading and trailing whitespace.

    Raises:
        ValueError: If the file extension is not supported (only .txt, .pdf, .docx are allowed).
        FileNotFoundError: If the specified file does not exist (raised natively by open()).
    """
    ext = os.path.splitext(file_path)[1].lower()
    text = ""
    
    if ext == ".txt":
        with open(file_path, "r", encoding="utf-8") as f:
            text = f.read()
    elif ext == ".pdf":
        import pypdf
        with open(file_path, "rb") as f:
            reader = pypdf.PdfReader(f)
            text = " ".join([page.extract_text() for page in reader.pages if page.extract_text()])
    elif ext == ".docx":
        import docx
        doc = docx.Document(file_path)
        text = "\n".join([para.text for para in doc.paragraphs])
    else:
        raise ValueError(f"Unsupported file type: {ext}")
        
    return text.strip()