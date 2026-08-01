import os
import json
import redis
from datetime import datetime, timezone
from services.scriber_service import transcribe_audio
from services.distiller_service import process_summarization_task
from services.gleaner_indexer_service import process_indexing_task
from services.gleaner_qa_service import process_qa_task
from utils.s3 import download_s3_file_to_temp, upload_json_to_s3
from utils.logger import get_logger


# Initialize logger for this specific file
logger = get_logger("consumer")

def start_worker():
    """
    Initializes the Redis connection and starts an infinite blocking loop to listen for incoming AI tasks on specified queues.

    The worker handles task routing, executes the corresponding AI service, and manages state updates in Redis. 
    It also handles temporary file cleanup and error logging.

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
        temp_local_file = None
        try:
            # Block indefinitely until a message arrives in any of the target queues
            popped = r.blpop(queues, timeout=5)
            
            # If nothing was in the queue after 5 seconds, it returns None. Loop again.
            if not popped:
                continue
                
            queue_name, message = popped
            
            # Decode the incoming JSON payload to extract task metadata
            queue_name, message = popped
            task_data = json.loads(message)
            task_id = task_data.get("task_id")
            s3_key = task_data.get("s3_key")
            
            task_hash = f"task:{task_id}"
            logger.info(f"Received Task: {task_id} from {queue_name}")

            # Update State -> Processing
            r.hset(task_hash, mapping={
                "status": "processing",
                "updated_at": datetime.now(timezone.utc).isoformat()
            })

            # Download source file from MinIO if required
            if s3_key:
                temp_local_file = download_s3_file_to_temp(s3_key)
                task_data["file_path"] = temp_local_file

            result = {}

            # Route the payload to the appropriate AI service based on the source queue
            if queue_name == "lexos:queue:transcription": # Scriber
                result = transcribe_audio(temp_local_file)
                
            elif queue_name == "lexos:queue:summarization": # Distiller
                result = process_summarization_task(task_data)

            elif queue_name == "lexos:queue:gleaner:index": # Gleaner Indexer
                result = process_indexing_task(task_data)

            elif queue_name == "lexos:queue:gleaner:ask": # Gleaner QA
                result = process_qa_task(task_data)            

            # Upload JSON result to MinIO (Skip for streaming QA)
            if queue_name != "lexos:queue:gleaner:ask":
                result_s3_key = f"results/{task_id}.json"
                result_url = upload_json_to_s3(result_s3_key, result)

                # Update State -> Completed
                r.hset(task_hash, mapping={
                    "status": "completed",
                    "result_s3_key": result_s3_key,
                    "result_url": result_url,
                    "updated_at": datetime.now(timezone.utc).isoformat()
                })
                logger.info(f"Task {task_id} completed successfully. Result stored at {result_url}")

        except Exception as e:
            # Catch and log any exceptions to ensure the worker loop remains active for subsequent tasks
            logger.error(f"Error processing task: {str(e)}", exc_info=True)
            if 'task_hash' in locals():
                r.hset(task_hash, mapping={
                    "status": "failed",
                    "error": str(e),
                    "updated_at": datetime.now(timezone.utc).isoformat()
                })
        finally:
            # Guarantee cleanup of the temporary uploaded file to prevent disk space exhaustion
            if temp_local_file and os.path.exists(temp_local_file):
                os.remove(temp_local_file)