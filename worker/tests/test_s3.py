from utils.s3 import _env_value, _normalize_endpoint


def test_normalize_endpoint_enables_tls_for_https(monkeypatch):
    """HTTPS endpoints force TLS even when the environment default is disabled."""
    # Configure deterministic inputs and dependency state.
    monkeypatch.setenv("S3_USE_SSL", "false")

    endpoint, secure = _normalize_endpoint("https://account.r2.cloudflarestorage.com/")

    assert endpoint == "account.r2.cloudflarestorage.com"
    assert secure is True


def test_normalize_endpoint_uses_configured_tls_without_scheme(monkeypatch):
    """Scheme-less endpoints use the explicit S3_USE_SSL setting."""
    # Configure deterministic inputs and dependency state.
    monkeypatch.setenv("S3_USE_SSL", "true")

    endpoint, secure = _normalize_endpoint("storage.example.com")

    assert endpoint == "storage.example.com"
    assert secure is True


def test_env_value_supports_legacy_minio_names(monkeypatch):
    """Legacy MinIO variables remain valid during configuration migration."""
    # Configure deterministic inputs and dependency state.
    monkeypatch.delenv("S3_BUCKET", raising=False)
    monkeypatch.setenv("MINIO_BUCKET", "legacy-bucket")

    assert _env_value("S3_BUCKET", "MINIO_BUCKET", "fallback") == "legacy-bucket"
