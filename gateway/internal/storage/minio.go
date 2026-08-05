package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var (
	MinioClient *minio.Client
	BucketName  string
	Ctx         = context.Background()
)

// envValue resolves generic S3 configuration first and falls back to legacy
// MinIO-specific variables for local backwards compatibility.
func envValue(primary string, legacy string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(primary)); value != "" {
		return value
	}
	if legacy != "" {
		if value := strings.TrimSpace(os.Getenv(legacy)); value != "" {
			return value
		}
	}
	return fallback
}

// normalizeEndpoint removes URL schemes because the MinIO SDK accepts host-only
// endpoints and a separate TLS flag.
func normalizeEndpoint(raw string) (string, bool) {
	endpoint := strings.TrimSpace(raw)
	secure := strings.EqualFold(envValue("S3_USE_SSL", "MINIO_USE_SSL", "false"), "true")

	if strings.HasPrefix(endpoint, "https://") {
		secure = true
		endpoint = strings.TrimPrefix(endpoint, "https://")
	} else if strings.HasPrefix(endpoint, "http://") {
		secure = false
		endpoint = strings.TrimPrefix(endpoint, "http://")
	}

	return strings.TrimRight(endpoint, "/"), secure
}

// InitObjectStorage initializes the S3-compatible object storage client.
func InitObjectStorage() error {
	endpoint, secure := normalizeEndpoint(envValue("S3_ENDPOINT", "MINIO_ENDPOINT", "lexos-minio:9000"))
	accessKey := envValue("S3_ACCESS_KEY", "MINIO_ACCESS_KEY", "")
	secretKey := envValue("S3_SECRET_KEY", "MINIO_SECRET_KEY", "")
	region := envValue("S3_REGION", "", "")
	BucketName = envValue("S3_BUCKET", "MINIO_BUCKET", "lexos-storage")
	autoCreate := strings.EqualFold(envValue("S3_AUTO_CREATE_BUCKET", "", "false"), "true")

	if endpoint == "" || accessKey == "" || secretKey == "" || BucketName == "" {
		return fmt.Errorf("incomplete S3-compatible storage configuration")
	}

	var err error
	MinioClient, err = minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
		Region: region,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize S3-compatible storage client: %w", err)
	}

	// Development can create the MinIO bucket automatically; production R2 buckets must exist.
	exists, err := MinioClient.BucketExists(Ctx, BucketName)
	if err != nil {
		return fmt.Errorf("failed to check object storage bucket: %w", err)
	}
	if !exists {
		if !autoCreate {
			return fmt.Errorf("object storage bucket %q does not exist", BucketName)
		}

		if err = MinioClient.MakeBucket(Ctx, BucketName, minio.MakeBucketOptions{Region: region}); err != nil {
			return fmt.Errorf("failed to create bucket %s: %w", BucketName, err)
		}
		log.Printf("Object storage bucket '%s' created successfully", BucketName)
	}

	log.Printf("Connected to S3-compatible object storage at %s", endpoint)
	return nil
}

// InitMinIO preserves the previous initialization entry point for compatibility.
func InitMinIO() error {
	return InitObjectStorage()
}

// MinioWrapper adapts the MinIO SDK client to the gateway StorageService interface.
type MinioWrapper struct {
	Client *minio.Client
}

// BucketName returns the configured object storage bucket.
func (m *MinioWrapper) BucketName() string {
	return BucketName
}

// UploadStream streams an incoming file directly to S3-compatible storage.
func (m *MinioWrapper) UploadStream(key string, reader io.Reader, size int64, contentType string) (string, error) {
	info, err := m.Client.PutObject(context.Background(), BucketName, key, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", err
	}
	return info.Key, nil
}

// StatObject checks whether an object exists in the configured storage backend.
func (m *MinioWrapper) StatObject(ctx context.Context, bucket string, key string) (bool, error) {
	_, err := m.Client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}

	response := minio.ToErrorResponse(err)
	switch response.Code {
	case "NoSuchKey", "NoSuchObject", "NotFound", "XMinioInvalidObjectName":
		return false, nil
	default:
		return false, err
	}
}

// GetObject retrieves an object from S3-compatible storage.
func (m *MinioWrapper) GetObject(ctx context.Context, bucket string, key string) (io.ReadCloser, error) {
	object, err := m.Client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	return object, err
}

// RemoveObject removes an object from S3-compatible storage.
func (m *MinioWrapper) RemoveObject(ctx context.Context, bucket string, key string) error {
	return m.Client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
}