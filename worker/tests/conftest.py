import os
import pytest

@pytest.fixture(autouse=True)
def set_env_vars():
    """Ensure tests run perfectly even without an .env file."""
    os.environ["MINIO_ENDPOINT"] = "localhost:9000"
    os.environ["MINIO_ACCESS_KEY"] = "test"
    os.environ["MINIO_SECRET_KEY"] = "test"
    os.environ["REDIS_URL"] = "redis://localhost:6379"
    os.environ["HF_TOKEN"] = "fake_test_token"