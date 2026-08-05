import io
import json
import os
import tempfile
from minio import Minio
from minio.error import S3Error
from utils.logger import get_logger

logger = get_logger("s3_storage")

# Lazily initialized client state avoids creating network clients during module import and tests.
_S3_CLIENT = None
_BUCKET_NAME = None


def _env_value(primary: str, legacy: str | None = None, default: str = "") -> str:
    """
    Resolves an object-storage environment value with optional legacy fallback.

    Args:
        primary (str): Preferred environment variable name.
        legacy (str | None, optional): Legacy MinIO-specific variable name.
        default (str, optional): Value returned when neither variable is configured.

    Returns:
        str: Resolved configuration value.
    """
    value = os.getenv(primary, "").strip()
    if value:
        return value

    if legacy:
        value = os.getenv(legacy, "").strip()
        if value:
            return value

    return default


def _normalize_endpoint(raw_endpoint: str) -> tuple[str, bool]:
    """
    Normalizes MinIO and Cloudflare R2 endpoints for the MinIO Python SDK.

    Args:
        raw_endpoint (str): Endpoint with or without an HTTP scheme.

    Returns:
        tuple[str, bool]: Normalized endpoint and TLS flag.
    """
    endpoint = raw_endpoint.strip().rstrip("/")
    secure = _env_value("S3_USE_SSL", "MINIO_USE_SSL", "false").lower() == "true"

    if endpoint.startswith("https://"):
        endpoint = endpoint.removeprefix("https://")
        secure = True
    elif endpoint.startswith("http://"):
        endpoint = endpoint.removeprefix("http://")
        secure = False

    return endpoint, secure


def get_s3_client() -> tuple[Minio, str]:
    """
    Lazy-loads the S3-compatible storage client and configured bucket name.

    Development targets MinIO while production can target Cloudflare R2 through
    the same S3-compatible API surface.

    Returns:
        tuple[Minio, str]: Initialized client and configured bucket name.
    """
    global _S3_CLIENT, _BUCKET_NAME

    # Generic S3 variables take precedence; legacy MinIO variables remain valid in development.
    if not _S3_CLIENT:
        endpoint, secure = _normalize_endpoint(
            _env_value("S3_ENDPOINT", "MINIO_ENDPOINT", "lexos-minio:9000")
        )
        access_key = _env_value("S3_ACCESS_KEY", "MINIO_ACCESS_KEY")
        secret_key = _env_value("S3_SECRET_KEY", "MINIO_SECRET_KEY")
        region = _env_value("S3_REGION", default="") or None
        _BUCKET_NAME = _env_value("S3_BUCKET", "MINIO_BUCKET", "lexos-storage")

        _S3_CLIENT = Minio(
            endpoint=endpoint,
            access_key=access_key,
            secret_key=secret_key,
            secure=secure,
            region=region,
        )
        logger.info(f"Connected worker to S3-compatible storage at {endpoint}")

    return _S3_CLIENT, _BUCKET_NAME


def download_s3_file_to_temp(s3_key: str) -> str:
    """
    Downloads an object to temporary local storage for CPU-bound processing.

    Args:
        s3_key (str): Object key inside the configured bucket.

    Returns:
        str: Local temporary file path.
    """
    client, bucket = get_s3_client()
    ext = os.path.splitext(s3_key)[1]

    temp_file = tempfile.NamedTemporaryFile(delete=False, suffix=ext)
    temp_file.close()
    client.fget_object(bucket, s3_key, temp_file.name)

    logger.debug(f"Downloaded {s3_key} to temporary file {temp_file.name}")
    return temp_file.name


def upload_json_to_s3(s3_key: str, data: dict) -> str:
    """
    Uploads a dictionary as a JSON object to S3-compatible storage.

    Args:
        s3_key (str): Destination object key.
        data (dict): JSON-serializable payload.

    Returns:
        str: Internal S3 URI for the uploaded object.
    """
    client, bucket = get_s3_client()
    json_bytes = json.dumps(data, indent=2, ensure_ascii=False).encode("utf-8")

    client.put_object(
        bucket_name=bucket,
        object_name=s3_key,
        data=io.BytesIO(json_bytes),
        length=len(json_bytes),
        content_type="application/json",
    )

    logger.debug(f"Uploaded JSON artifact: {s3_key}")
    return f"s3://{bucket}/{s3_key}"


def upload_file_to_s3(s3_key: str, file_path: str) -> str:
    """
    Uploads a local file to S3-compatible storage.

    Args:
        s3_key (str): Destination object key.
        file_path (str): Local file path.

    Returns:
        str: Internal S3 URI for the uploaded object.
    """
    client, bucket = get_s3_client()
    client.fput_object(bucket, s3_key, file_path)
    logger.debug(f"Uploaded file artifact: {s3_key}")
    return f"s3://{bucket}/{s3_key}"


def object_exists(s3_key: str) -> bool:
    """
    Checks whether an object exists in the configured bucket.

    Args:
        s3_key (str): Object key to inspect.

    Returns:
        bool: True when the object exists, otherwise False.

    Raises:
        S3Error: If the storage backend returns an error unrelated to absence.
    """
    if not s3_key:
        return False

    client, bucket = get_s3_client()
    try:
        client.stat_object(bucket, s3_key)
        return True
    except S3Error as exc:
        if exc.code in {"NoSuchKey", "NoSuchObject", "NotFound"}:
            return False
        raise


def delete_s3_object(s3_key: str) -> None:
    """
    Removes an object from the configured bucket.

    Args:
        s3_key (str): Object key to remove.
    """
    if not s3_key:
        return

    client, bucket = get_s3_client()
    client.remove_object(bucket, s3_key)
