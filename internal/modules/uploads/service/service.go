// Package uploads handles file upload requests.
//
// The service takes a storage.Provider as a dependency. It has no idea
// whether files are going to local disk, S3, Cloudinary, or anywhere
// else. That's the whole point: the business logic of "take a file,
// give it a unique key, store it" is the same regardless of backend.
package service

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/topboyasante/api-base/internal/providers/storage"
)

type Service struct {
	storage storage.Provider
}

func NewService(s storage.Provider) *Service {
	return &Service{storage: s}
}

// Upload stores the file and returns the metadata the client needs to
// reference it later.
func (s *Service) Upload(ctx context.Context, filename string, body io.Reader, size int64, contentType string) (*storage.UploadResult, error) {
	// Generate a unique key so user-provided filenames don't collide.
	// We preserve the extension for convenience — makes URLs readable
	// and lets browsers serve them correctly.
	//
	// No "uploads/" prefix here: the storage backend already scopes files
	// (local uses basePath="./uploads", s3 uses a bucket). Prefixing the
	// key would double up — ./uploads/uploads/<uuid>.ext on local disk.
	ext := filepath.Ext(filename)
	key := uuid.NewString() + ext

	meta, err := s.storage.Upload(ctx, storage.UploadInput{
		Key:         key,
		Body:        body,
		ContentType: contentType,
		Size:        size,
	})
	if err != nil {
		return nil, fmt.Errorf("uploads service: %w", err)
	}

	return &storage.UploadResult{
		Key:      meta.Key,
		URL:      meta.URL,
		Size:     meta.Size,
		Provider: meta.Provider,
		Uploaded: time.Now().UTC(),
	}, nil
}
