import os
import json
import redis
from services.scriber_service import transcribe_audio
from services.distiller_service import process_summarization_task
from services.gleaner_indexer_service import process_indexing_task
from services.gleaner_qa_service import process_qa_task
from utils.logger import get_logger


# Initialize logger for this specific file
logger = get_logger("consumer")

def start_worker():
    """
    Initializes the Redis connection and starts an infinite blocking loop to listen for incoming AI tasks on specified queues.

    The worker handles task routing, executes the corresponding AI service, saves the result to a JSON file, 
    and automatically cleans up the original uploaded file to prevent storage bloat.

    Queues monitored:
        - 'lexos:queue:transcription': Tasks for Faster-Whisper audio processing.
        - 'lexos:queue:summarization': Tasks for Llama.cpp / Qwen3 document summarization.
        - 'lexos:queue:gleaner:index': Tasks for Gleaner document indexing (FAISS + E5 embeddings).
        - 'lexos:queue:gleaner:ask': Tasks for Gleaner question answering (FAISS retrieval + Llama.cpp / Qwen3 reasoning).

    Args:
        None

    Returns:
        None. This function runs in an infinite loop until the process is terminated.

    Raises:
        redis.exceptions.ConnectionError: If the Redis server is unreachable on startup.
    """
    # Establish Redis connection
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
    
    # Define the target queues for AI processing tasks
    queues = [
        "lexos:queue:transcription", 
        "lexos:queue:summarization", 
        "lexos:queue:gleaner:index",
        "lexos:queue:gleaner:ask"
    ]
    logger.info(f"Listening for tasks on: {', '.join(queues)}...")

    # Enter a continuous polling loop to process incoming background tasks
    while True:
        file_path = None
        try:
            # Block indefinitely until a message arrives in any of the target queues
            popped = r.blpop(queues, timeout=5)
            
            # If nothing was in the queue after 5 seconds, it returns None. Loop again.
            if not popped:
                continue
                
            queue_name, message = popped
            
            # Decode the incoming JSON payload to extract task metadata
            task_data = json.loads(message)
            task_id = task_data.get("task_id")
            
            logger.info(f"Received Task: {task_id} from {queue_name}")
            result = {}

            # Route the payload to the appropriate AI service based on the source queue
            if queue_name == "lexos:queue:transcription": # Scriber
                file_path = task_data.get("file_path")
                result = transcribe_audio(file_path)
                
            elif queue_name == "lexos:queue:summarization": # Distiller
                file_path = task_data.get("file_path")
                result = process_summarization_task(task_data)

            elif queue_name == "lexos:queue:gleaner:index": # Gleaner Indexer
                file_path = task_data.get("file_path")
                result = process_indexing_task(task_data)

            elif queue_name == "lexos:queue:gleaner:ask": # Gleaner QA
                result = process_qa_task(task_data)            

            # Serialize the processing result and write it to the shared volume for the gateway to retrieve
            result_path = os.path.join("/uploads", f"{task_id}.json")
            with open(result_path, "w", encoding="utf-8") as f:
                json.dump(result, f, indent=4, ensure_ascii=False)

            logger.info(f"Task {task_id} complete. Result saved to {result_path}")

        except Exception as e:
            # Catch and log any exceptions to ensure the worker loop remains active for subsequent tasks
            logger.error(f"Error processing task: {str(e)}", exc_info=True)
        finally:
            # Guarantee cleanup of the temporary uploaded file to prevent disk space exhaustion
            if file_path and os.path.exists(file_path):
                try:
                    os.remove(file_path)
                    logger.debug(f"Cleaned up raw file: {file_path}")
                except Exception as cleanup_error:
                    logger.warning(f"Failed to delete file {file_path}: {cleanup_error}")