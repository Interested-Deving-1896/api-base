package storage

import "io"

// UploadInput bundles everything Upload needs into one struct.
//
// Why a struct instead of just passing (key, body, contentType, size) as
// separate arguments? Because if we later need one more field (say, tags
// or checksum), we'd have to change every Upload call site in the codebase.
// With a struct, existing callers keep working — they just don't set the
// new field.
//
//	in := storage.UploadInput{
//	    Key:         "avatars/user-123.jpg",
//	    Body:        fileFromForm,
//	    ContentType: "image/jpeg",
//	    Size:        fileSize,
//	}
type UploadInput struct {
	// Key is the "path" the file is stored under — think of it like a
	// filename the backend uses to find the file later.
	//
	// Use slash-separated prefixes to keep different features apart:
	//   "avatars/user-123.jpg"
	//   "invoices/2026/04/inv-998.pdf"
	//   "exports/report-abcd.csv"
	//
	// Without prefixes, an avatar named "cat.jpg" could collide with a
	// blog post's "cat.jpg" and overwrite it. Prefixes prevent that.
	Key string

	// Body is where the bytes come from — anything that satisfies io.Reader:
	// an *os.File, an http.Request.Body, a bytes.Buffer, etc. Upload reads
	// until EOF (end of file / end of stream).
	Body io.Reader

	// ContentType tells browsers and CDNs how to handle the file. Without
	// it, a browser might download "image.jpg" as a generic file instead
	// of showing it inline.
	//
	// Common values: "image/jpeg", "image/png", "application/pdf",
	// "video/mp4", "text/plain". Leave empty if you don't know.
	ContentType string

	// Size is the total number of bytes, if you know it ahead of time.
	// S3 can upload faster with this info (one request vs many). If you
	// don't know — e.g. streaming an unknown-length source — leave it 0.
	Size int64
}

// FileMetadata is what Upload hands back on success. Store Key (or URL)
// in your database so you can find the file again later.
//
//	meta, _ := store.Upload(ctx, in)
//	db.Exec("UPDATE users SET avatar_key=? WHERE id=?", meta.Key, userID)
//	// later: render `<img src={user.avatar_url}>` using meta.URL
type FileMetadata struct {
	// Key is the same key you uploaded under. Save it in the DB — it's
	// the stable identifier you pass back to Download/Delete later.
	Key string

	// URL is a direct link you can render to users (e.g. in an <img> tag).
	// Format depends on the backend:
	//   local: "/files/avatars/user-123.jpg"  (served by your app)
	//   s3:    "https://my-bucket.s3.amazonaws.com/avatars/user-123.jpg"
	URL string

	// Size is the number of bytes actually written. Useful for showing
	// "2.3 MB" in a UI or enforcing quotas.
	Size int64

	// Provider is the backend that stored this file ("local", "s3", "r2").
	// Helpful when debugging — e.g. a file uploaded in dev (local) won't
	// exist in prod (s3), and this field tells you immediately.
	Provider string
}
