// Package storage defines ONE way to save and load files — without caring
// where they actually live. Files might end up on disk, in AWS S3, or in
// Cloudflare R2, but code that uploads an avatar shouldn't have to know.
//
// The trick is the Provider interface below. Your feature code talks to
// the interface; the actual backend is chosen at startup from config.
//
// Example — an avatar service that works with any backend:
//
//	type AvatarService struct {
//	    store storage.Provider  // could be local.Local, s3.S3, r2.*, ...
//	}
//
//	func (s *AvatarService) Save(ctx context.Context, userID string, r io.Reader) error {
//	    _, err := s.store.Upload(ctx, storage.UploadInput{
//	        Key:         "avatars/" + userID + ".jpg",
//	        Body:        r,
//	        ContentType: "image/jpeg",
//	    })
//	    return err
//	}
//
// Notice AvatarService never imports "local" or "s3" — only "storage". In
// dev, the app wires up local disk; in prod, S3. Same feature code either way.
//
// If you add a new method to Provider, every backend (local, s3, r2) must
// implement it or the code won't compile. So keep this interface small —
// add a method only when a real feature needs it.
package storage

import (
	"context"
	"io"
	"time"
)

type Provider interface {
	// Name returns the short identifier this backend was registered with
	// ("local", "s3", "r2"). Handy in logs: "uploaded via s3" vs "uploaded
	// via local" tells you at a glance where a file actually went.
	Name() string

	// Upload writes the bytes from in.Body to the backend under in.Key,
	// and returns metadata (URL, size, etc.) you can store in your DB.
	//
	//	meta, err := p.Upload(ctx, storage.UploadInput{
	//	    Key:         "avatars/user-123.jpg",
	//	    Body:        fileReader,
	//	    ContentType: "image/jpeg",
	//	})
	//	// save meta.Key or meta.URL on the User row
	//
	// If in.Body is an *os.File or http.Request.Body, YOU close it — not
	// this method. That's standard io.Reader etiquette in Go.
	Upload(ctx context.Context, in UploadInput) (*FileMetadata, error)

	// Download returns a reader for the file at key. You must Close() the
	// returned ReadCloser when you're done, or you'll leak a file handle
	// (local) or HTTP connection (s3/r2).
	//
	//	rc, err := p.Download(ctx, "avatars/user-123.jpg")
	//	if err != nil { return err }
	//	defer rc.Close()
	//	io.Copy(w, rc)  // e.g. stream to an HTTP response
	//
	// Returns ErrNotFound (not some backend-specific error) when the key
	// doesn't exist, so callers can check it the same way everywhere:
	//
	//	if errors.Is(err, storage.ErrNotFound) { ... }
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes the file at key. Deleting a key that doesn't exist
	// is NOT an error — it just returns nil. This makes retries safe: if
	// the first delete half-succeeds and you retry, the second call won't
	// blow up just because the file is already gone.
	Delete(ctx context.Context, key string) error

	// SignedURL returns a URL the browser can hit directly to download the
	// file, valid for roughly `ttl`. Use this instead of streaming through
	// your server when files are big (videos, large images) — it saves
	// your server's bandwidth.
	//
	//	url, _ := p.SignedURL(ctx, "videos/clip.mp4", 15*time.Minute)
	//	// send url back to the client in JSON; it expires in 15 min.
	//
	// TTL is a HINT. S3 caps signed URLs at 7 days; local returns a plain
	// public URL with no real expiry. Don't rely on exact timing.
	SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
}