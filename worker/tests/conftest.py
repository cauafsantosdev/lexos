import os
import pytest


@pytest.fixture(autouse=True)
def set_env_vars():
    """Provides deterministic infrastructure configuration for isolated tests."""
    os.environ["S3_ENDPOINT"] = "localhost:9000"
    os.environ["S3_ACCESS_KEY"] = "test"
    os.environ["S3_SECRET_KEY"] = "test"
    os.environ["S3_BUCKET"] = "lexos-storage"
    os.environ["S3_USE_SSL"] = "false"
    os.environ["REDIS_URL"] = "redis://localhost:6379"
    os.environ["HF_TOKEN"] = "fake_test_token"
    os.environ["CACHE_TTL_SECONDS"] = "604800"
    os.environ["TASK_TTL_SECONDS"] = "86400"
