# Defines three summary styles for the final synthesis stage, each with specific formatting and content rules.
PROMPT_STYLES = {
    "bullet_points": """Output the minimum number of bullet points required to cover all major ideas (usually between 3 and 6).
Bold a unique, descriptive title at the start of each bullet based on the content (e.g., **Core Idea**, **Methodology**, **Key Results**).""",

    "short_paragraph": """Write EXACTLY ONE continuous, dense narrative prose paragraph.
- Synthesize the objective, methodology, exact quantitative metrics, and conclusions into fluid, grammatically connected sentences.
- STRICTLY FORBIDDEN: Do NOT use bullet points, dashes, numbered lists, or inline section labels (do NOT write 'Methodology:', 'Conclusion:', or 'Main objective:').
- Do NOT include any line breaks or newlines within the paragraph.""",

    "executive": """Output a structured executive summary with three distinct Markdown subheadings:
### Problem
### Solution
### Key Takeaways"""
}

def build_map_system_prompt() -> str:
    """
    Generates the system prompt for the Map stage (Fact Extraction).
    Forces the model to act as a raw data extractor, actively ignoring previous work and preserving hard numbers.

    Returns:
        str: The strict data extraction system prompt.
    """
    return """You are an expert data extraction engine.
Your task is to extract raw, structured facts from the provided text chunk.

EXTRACT ONLY:
- The main research objective or core problem.
- The proposed methodology or framework.
- The exact quantitative results and findings.
- The final conclusions.

STRICTLY IGNORE (DO NOT EXTRACT):
- Literature reviews and background information.
- Mentions of previous work (e.g., "Previous work proposed...", "Smith et al.").
- External citations, references, and bibliography.
- Generic fluff.

CRITICAL RULES:
1. PRESERVE EVERY NUMERICAL VALUE EXACTLY. Do not approximate.
2. Output a raw list of extracted facts, NOT a polished summary.
3. Never expose intermediate reasoning or thoughts."""

def build_map_user_prompt(text_chunk: str) -> str:
    """
    Generates the user prompt for the Map stage.

    Args:
        text_chunk (str): A subsection of the parsed document text.

    Returns:
        str: The formatted prompt injecting the text chunk for fact extraction.
    """
    return f"""Extract the key facts and exact metrics from this chunk of text, ignoring all citations and previous work:

```text
{text_chunk}
```"""

def build_reduce_system_prompt(style: str = "bullet_points") -> str:
    """
    Generates the system prompt for the Reduce stage (Final Synthesis).
    Instructs the model to synthesize the extracted facts while strictly adhering to the requested formatting style.

    Args:
        style (str, optional): The formatting style requested by the user 
            ('bullet_points', 'short_paragraph', 'executive'). Defaults to 'bullet_points'.

    Returns:
        str: The final synthesis system prompt.
    """
    style_instruction = PROMPT_STYLES.get(style, PROMPT_STYLES["bullet_points"])
    
    return f"""You are Lexos Distiller, an expert document summarizer.
You will receive a list of extracted facts from a larger document. Synthesize them into a cohesive final summary.

CRITICAL RULES:
1. If multiple quantitative results are available, ALWAYS preserve the 2 or 3 most important numerical findings in the final summary.
2. When numerical results, evaluation metrics, percentages, model names, or benchmark scores are present, prioritize them over qualitative descriptions.
3. Ensure the summary focuses ONLY on the primary contribution. Exclude any facts that appear to be referencing external studies or previous work.
4. Be strictly factual. Do not infer unsupported facts.
5. Avoid repeating the same information using different wording.
6. Never expose intermediate reasoning or thoughts.
7. Output valid Markdown format ONLY. Do not include conversational greetings.

# OUTPUT FORMAT
{style_instruction}"""

def build_reduce_user_prompt(extracted_facts: str) -> str:
    """
    Generates the user prompt for the Reduce stage.

    Args:
        extracted_facts (str): A consolidated string containing all facts 
            extracted during the Map stage.

    Returns:
        str: The formatted prompt injecting the facts for final summary generation.
    """
    return f"""Synthesize the following extracted facts into the final summary based on your instructions:

```text
{extracted_facts}
```"""