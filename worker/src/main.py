import os
import json
import redis
from models.scriber import transcribe_audio


def main():
    print("Lexos Worker initialized!")
    
    # Connect to Redis
    redis_url = os.getenv("REDIS_URL", "redis:6379")
    host, port = redis_url.split(":")
    
    try:
        r = redis.Redis(host=host, port=int(port), decode_responses=True)
        r.ping()
        print("Connected to Redis successfully.")
    except redis.ConnectionError as e:
        print(f"Fatal: Could not connect to Redis: {e}")
        return

    print("Listening for tasks on 'lexos:queue:transcription'...")

    # The Event Loop
    while True:
        try:
            # BLPOP blocks until a message arrives.
            queue_name, message = r.blpop("lexos:queue:transcription", timeout=0)
            
            # Parse the JSON payload sent by Go
            task_data = json.loads(message)
            task_id = task_data.get("task_id")
            file_path = task_data.get("file_path")
            
            print(f"\nReceived Task: {task_id} | Processing file: {file_path}")
            
            # Execute AI Inference
            result = transcribe_audio(file_path)
            
            # Save the result
            result_path = file_path.replace(os.path.splitext(file_path)[1], ".json")
            with open(result_path, "w", encoding="utf-8") as f:
                json.dump(result, f, indent=4, ensure_ascii=False)
                
            print(f"Task {task_id} complete. Result saved to {result_path}")

        except json.JSONDecodeError:
            print("Error: Received malformed JSON message.")
        except Exception as e:
            print(f"Error processing task: {e}")

        finally:
            # Garbage collector
            if file_path and os.path.exists(file_path):
                try:
                    os.remove(file_path)
                    print(f"Cleaned up raw file: {file_path}")
                except Exception as cleanup_error:
                    print(f"Failed to delete file {file_path}: {cleanup_error}")

if __name__ == "__main__":
    main()