package storage

import "errors"

// These are "sentinel errors" — fixed error values callers can compare
// against with errors.Is. Every backend (local, s3, r2) returns these
// SAME errors for the same situations, so your code doesn't need an if
// statement per backend.
//
//	rc, err := store.Download(ctx, key)
//	if errors.Is(err, storage.ErrNotFound) {
//	    // show a 404 — works whether store is local, s3, or r2
//	    return c.Status(404).SendString("not found")
//	}
//	if err != nil {
//	    return err  // some other problem (network, disk, etc.)
//	}
var (
	// ErrNotFound means "you asked for a key that isn't there."
	//
	// Returned by Download when the file doesn't exist.
	// NOT returned by Delete — deleting a missing key is silently OK, so
	// retries are safe (see Provider.Delete docs).
	ErrNotFound = errors.New("storage: key not found")

	// ErrProviderNotRegistered means "registry.Resolve was asked for a
	// backend name nobody registered." Usually one of:
	//
	//   1. Typo in config:     STORAGE_PROVIDER=s33
	//   2. Forgot the import:  the backend's package isn't imported
	//                          anywhere, so its init()/Register call
	//                          never ran
	//
	// If you see this error at startup, check config first, then check
	// wire.go is importing the backend package.
	ErrProviderNotRegistered = errors.New("storage: provider not registered")
)