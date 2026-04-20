// Package local stores uploaded files in a folder on your machine.
//
// Use it when:
//   - You're running the app locally in development (no AWS keys needed)
//   - Running tests — fast and isolated, nothing to mock
//   - Tiny deployments: one server, one disk, that's it
//
// Don't use it when:
//   - You have more than one server. Uploads on server A won't exist on
//     server B, so users will see broken images at random.
//   - You care about durability. If the disk dies, the files are gone.
//     S3/R2 replicate across machines; your laptop's SSD doesn't.
//
// SignedURL note: cloud backends (s3, r2) return a URL that's temporarily
// valid — anyone with the URL can download the file, but only until the
// TTL expires. This backend can't do that; it just returns the public URL.
// Access control (if you need it) has to happen in the HTTP handler that
// serves the /files/ route.
package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/topboyasante/api-base/internal/providers/storage"
)

type Local struct {
	basePath string
	baseURL  string
}

// New builds a local-disk Provider. The registry calls this when config
// says STORAGE_PROVIDER=local.
//
// Options (all optional, have sensible defaults):
//
//	"path"     — folder on disk to store files  (default: "./uploads")
//	"base_url" — URL prefix files are served at (default: "/files")
//
// Given opts{"path": "/var/data", "base_url": "/cdn"}, a file uploaded
// with Key="avatars/x.jpg" ends up at /var/data/avatars/x.jpg on disk,
// and its public URL is "/cdn/avatars/x.jpg".
func New(opts map[string]string) (storage.Provider, error) {
	basePath := opts["path"]
	if basePath == "" {
		basePath = "./uploads"
	}
	// Create the folder (and any missing parents) if it doesn't exist.
	// 0o755 = owner can read/write/execute, everyone else can read/execute.
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, fmt.Errorf("local storage: ensure base path: %w", err)
	}

	baseURL := opts["base_url"]
	if baseURL == "" {
		// The app's HTTP router should have a handler mounted here that
		// serves files from basePath. Without that, the URLs won't work.
		baseURL = "/files"
	}

	return &Local{basePath: basePath, baseURL: baseURL}, nil
}

func (l *Local) Name() string { return "local" }

func (l *Local) Upload(ctx context.Context, in storage.UploadInput) (*storage.FileMetadata, error) {
	// filepath.Join is safer than string concat — it handles slashes,
	// trims duplicates, and works on both Unix and Windows.
	//   basePath="./uploads", Key="avatars/x.jpg"  ->  "./uploads/avatars/x.jpg"
	full := filepath.Join(l.basePath, in.Key)

	// os.Create fails if the parent folder doesn't exist, so create it first.
	// filepath.Dir("./uploads/avatars/x.jpg") -> "./uploads/avatars"
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return nil, fmt.Errorf("local upload: mkdir: %w", err)
	}

	f, err := os.Create(full)
	if err != nil {
		return nil, fmt.Errorf("local upload: create: %w", err)
	}
	defer f.Close() // runs when this function returns, no matter what

	// io.Copy streams bytes from the reader to the file without loading
	// the whole thing into memory — important for big uploads.
	n, err := io.Copy(f, in.Body)
	if err != nil {
		return nil, fmt.Errorf("local upload: write: %w", err)
	}

	return &storage.FileMetadata{
		Key:      in.Key,
		URL:      l.baseURL + "/" + in.Key,
		Size:     n, // actual bytes written, not in.Size (which might be 0)
		Provider: "local",
	}, nil
}

func (l *Local) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	// *os.File already implements io.ReadCloser, so we can return it
	// directly. The caller is responsible for calling .Close() on it.
	f, err := os.Open(filepath.Join(l.basePath, key))
	if err != nil {
		// Translate the OS-level "file doesn't exist" error into our own
		// package-level ErrNotFound, so callers don't need to know we're
		// backed by a filesystem.
		if errors.Is(err, os.ErrNotExist) {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("local download: %w", err)
	}
	return f, nil
}

func (l *Local) Delete(ctx context.Context, key string) error {
	err := os.Remove(filepath.Join(l.basePath, key))
	// If the file is already gone, that's fine — the caller wanted it
	// gone and it's gone. Only real errors (permissions, disk issues)
	// should bubble up.
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("local delete: %w", err)
	}
	return nil
}

func (l *Local) SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	// Local can't produce a truly expiring URL. We just hand back the
	// same public URL and ignore the TTL. See the package comment for why.
	return l.baseURL + "/" + key, nil
}
