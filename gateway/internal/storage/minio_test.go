package storage

import "testing"

func TestNormalizeEndpointInfersTLSFromHTTPS(t *testing.T) {
	t.Setenv("S3_USE_SSL", "false")

	endpoint, secure := normalizeEndpoint("https://account.r2.cloudflarestorage.com/")

	if endpoint != "account.r2.cloudflarestorage.com" {
		t.Fatalf("unexpected normalized endpoint: %s", endpoint)
	}
	if !secure {
		t.Fatal("HTTPS endpoint must enable TLS")
	}
}

func TestNormalizeEndpointUsesConfiguredTLSWithoutScheme(t *testing.T) {
	t.Setenv("S3_USE_SSL", "true")

	endpoint, secure := normalizeEndpoint("storage.example.com")

	if endpoint != "storage.example.com" {
		t.Fatalf("unexpected normalized endpoint: %s", endpoint)
	}
	if !secure {
		t.Fatal("S3_USE_SSL=true must enable TLS")
	}
}

func TestEnvValueFallsBackToLegacyConfiguration(t *testing.T) {
	t.Setenv("S3_BUCKET", "")
	t.Setenv("MINIO_BUCKET", "legacy-bucket")

	value := envValue("S3_BUCKET", "MINIO_BUCKET", "fallback")
	if value != "legacy-bucket" {
		t.Fatalf("expected legacy fallback, received %s", value)
	}
}
