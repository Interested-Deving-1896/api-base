// Package r2 stores files in Cloudflare R2 — Cloudflare's S3-like
// storage service. The main draw vs AWS: R2 has zero egress fees
// (downloads cost nothing), which adds up fast for image-heavy apps.
//
// R2 speaks the same API as S3, so this package is just a thin wrapper
// around the s3 package. It takes R2-shaped config (account_id) and
// translates it into S3-shaped config (endpoint URL) before delegating.
//
// Why a separate package at all, instead of telling users "set
// STORAGE_PROVIDER=s3 with this endpoint URL"?
//
//   1. Config stays simple. Users paste an account_id from the Cloudflare
//      dashboard; they don't have to format a URL.
//   2. STORAGE_PROVIDER=r2 tells future-you "this is R2" at a glance,
//      without having to decode an endpoint URL.
//   3. If R2 ever stops being 100% S3-compatible, the fix lives here —
//      feature code elsewhere doesn't change.
package r2

import (
	"errors"
	"fmt"

	"github.com/topboyasante/api-base/internal/providers/storage"
	"github.com/topboyasante/api-base/internal/providers/storage/s3"
)

// New builds an R2-backed Provider. The registry calls this when config
// says STORAGE_PROVIDER=r2.
//
// Options:
//
//	"account_id" — Cloudflare account ID (required, from CF dashboard)
//	"bucket"     — R2 bucket name        (required)
//	"access_key" — R2 API token key      (required)
//	"secret_key" — R2 API token secret   (required)
//	"public_url" — optional public URL for rendering in the app
func New(opts map[string]string) (storage.Provider, error) {
    accountID := opts["account_id"]
    if accountID == "" {
        return nil, errors.New("r2: account_id is required")
    }

    // R2's API endpoint format is fixed: https://<account_id>.r2.cloudflarestorage.com
    // Rather than making users paste that whole URL, we just ask for the
    // account_id and build the URL here.
    s3Opts := map[string]string{
        "bucket":     opts["bucket"],
        "region":     "auto", // R2 has no regions, but the S3 SDK requires one
        "access_key": opts["access_key"],
        "secret_key": opts["secret_key"],
        "endpoint":   fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID),
        "public_url": opts["public_url"],
    }

    provider, err := s3.New(s3Opts)
    if err != nil {
        return nil, fmt.Errorf("r2: %w", err)
    }

    // The s3 package's provider returns Name() = "s3". We want Name() = "r2"
    // so logs and FileMetadata.Provider read "r2" — accurate info about
    // where files actually live. See namedProvider below for how.
    return &namedProvider{Provider: provider, name: "r2"}, nil
}

// namedProvider is a tiny wrapper that changes ONLY the Name() method of
// an underlying Provider. Everything else (Upload, Download, Delete,
// SignedURL) falls through to the embedded Provider unchanged.
//
// This uses Go's "embedding" — by declaring `storage.Provider` as a field
// without a name, namedProvider automatically gets every method of the
// embedded interface. We then "override" just Name() by defining our own.
//
// It's like inheritance but simpler: you pick which methods to replace,
// and the rest are forwarded for free.
type namedProvider struct {
    storage.Provider
    name string
}

func (n *namedProvider) Name() string { return n.name }