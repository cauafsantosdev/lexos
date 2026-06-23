package utils

import (
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
)

// SaveUploadedFile reads a multipart file and saves it to the shared /uploads volume
func SaveUploadedFile(file *multipart.FileHeader, taskID string) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	// Extract extension and create destination path
	fileExt := filepath.Ext(file.Filename)
	dstPath := filepath.Join("/uploads", taskID+fileExt)

	// Create physical file
	dst, err := os.Create(dstPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	// Stream contents to disk
	if _, err = io.Copy(dst, src); err != nil {
		return "", err
	}

	return dstPath, nil
}