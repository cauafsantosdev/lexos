package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var (
	MinioClient *minio.Client
	BucketName  string
	Ctx         = context.Background()
)

// InitMinIO initializes the S3 client and creates the default bucket if it doesn't exist.
func InitMinIO() error {
	endpoint := os.Getenv("MINIO_ENDPOINT")
	accessKey := os.Getenv("MINIO_ACCESS_KEY")
	secretKey := os.Getenv("MINIO_SECRET_KEY")
	BucketName = os.Getenv("MINIO_BUCKET")
	useSSL := os.Getenv("MINIO_USE_SSL") == "true"

	var err error
	MinioClient, err = minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to MinIO: %w", err)
	}

	// Ensure the bucket exists
	exists, err := MinioClient.BucketExists(Ctx, BucketName)
	if err != nil {
		return fmt.Errorf("error checking bucket existence: %w", err)
	}
	if !exists {
		err = MinioClient.MakeBucket(Ctx, BucketName, minio.MakeBucketOptions{})
		if err != nil {
			return fmt.Errorf("failed to create bucket %s: %w", BucketName, err)
		}
		log.Printf("MinIO bucket '%s' created successfully", BucketName)
	}

	log.Printf("Connected to MinIO at %s", endpoint)
	return nil
}

// MinioWrapper is a wrapper around the MinIO client to implement the StorageService interface.
type MinioWrapper struct {
	Client *minio.Client
}

// UploadStream streams an incoming file directly to MinIO without saving to local disk.
func (m *MinioWrapper) UploadStream(key string, reader io.Reader, size int64, contentType string) (string, error) {
	info, err := m.Client.PutObject(context.Background(), BucketName, key, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", err
	}
	return info.Key, nil
}

// StatObject checks if an object exists in MinIO.
func (m *MinioWrapper) StatObject(ctx context.Context, bucket string, key string) (bool, error) {
	_, err := m.Client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetObject retrieves an object from MinIO.
func (m *MinioWrapper) GetObject(ctx context.Context, bucket string, key string) (io.ReadCloser, error) {
	object, err := m.Client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	return object, err
}