import os
import io
import json
import tempfile
from minio import Minio
from utils.logger import get_logger

logger = get_logger("s3_storage")

_MINIO_CLIENT = None
_BUCKET_NAME = None

def get_s3_client() -> tuple[Minio, str]:
    """Lazy-loads and returns the MinIO client and target bucket name."""
    global _MINIO_CLIENT, _BUCKET_NAME

    if not _MINIO_CLIENT:
        endpoint = os.getenv("MINIO_ENDPOINT", "lexos-minio:9000")
        access_key = os.getenv("MINIO_ACCESS_KEY", "lexos_admin")
        secret_key = os.getenv("MINIO_SECRET_KEY", "lexos_pass")
        _BUCKET_NAME = os.getenv("MINIO_BUCKET", "lexos-storage")
        secure = os.getenv("MINIO_USE_SSL", "false").lower() == "true"

        _MINIO_CLIENT = Minio(
            endpoint=endpoint,
            access_key=access_key,
            secret_key=secret_key,
            secure=secure
        )
        logger.info(f"Connected worker to MinIO at {endpoint}")

    return _MINIO_CLIENT, _BUCKET_NAME

def download_s3_file_to_temp(s3_key: str) -> str:
    """Downloads an object from MinIO to a temporary local file for processing."""
    client, bucket = get_s3_client()
    ext = os.path.splitext(s3_key)[1]
    
    temp_file = tempfile.NamedTemporaryFile(delete=False, suffix=ext)
    client.fget_object(bucket, s3_key, temp_file.name)
    temp_file.close()
    
    logger.debug(f"Downloaded {s3_key} from MinIO to temporary file {temp_file.name}")
    return temp_file.name

def upload_json_to_s3(s3_key: str, data: dict) -> str:
    """Uploads a Python dictionary as a JSON object directly to MinIO."""
    client, bucket = get_s3_client()
    json_bytes = json.dumps(data, indent=2, ensure_ascii=False).encode("utf-8")
    
    client.put_object(
        bucket_name=bucket,
        object_name=s3_key,
        data=io.BytesIO(json_bytes),
        length=len(json_bytes),
        content_type="application/json"
    )
    
    logger.debug(f"Uploaded result JSON to MinIO: {s3_key}")
    return f"s3://{bucket}/{s3_key}"

def upload_file_to_s3(s3_key: str, file_path: str) -> str:
    """Uploads a physical file from disk to MinIO."""
    client, bucket = get_s3_client()
    client.fput_object(bucket, s3_key, file_path)
    return f"s3://{bucket}/{s3_key}"