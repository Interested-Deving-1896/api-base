// Package s3 stores files in AWS S3 — Amazon's cloud file storage.
//
// It also works with services that copied S3's API:
//   - Backblaze B2        (cheap cold storage)
//   - DigitalOcean Spaces (bundled with Droplets)
//   - MinIO               (self-hosted, great for tests)
//   - Cloudflare R2       (used via the r2 package, which wraps this one)
//
// You set it via config options, passed through the registry:
//
//	"bucket"     — the bucket name       (required, e.g. "my-app-uploads")
//	"region"     — the AWS region        (required, e.g. "us-east-1")
//	"access_key" — AWS access key ID     (required unless running on AWS)
//	"secret_key" — AWS secret access key (required unless running on AWS)
//	"endpoint"   — override API URL      (only for non-AWS services)
//
// About credentials: if your app runs on AWS itself (EC2, ECS, Lambda),
// leave access_key and secret_key BLANK. AWS gives those environments a
// built-in identity called an "IAM role", and the SDK picks it up
// automatically. That's safer than checking keys into config — no keys
// to leak, no keys to rotate.
package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/topboyasante/api-base/internal/providers/storage"
)

type S3 struct {
	client  *awss3.Client
	presign *awss3.PresignClient
	bucket  string
}

// New builds an S3-backed Provider. The registry calls this when config
// says STORAGE_PROVIDER=s3. See the package comment for the full list
// of supported opts.
func New(opts map[string]string) (storage.Provider, error) {
	bucket := opts["bucket"]
	region := opts["region"]
	if bucket == "" || region == "" {
		// Bail out loudly at startup rather than failing mysteriously
		// on the first upload. Missing config should never silently "work."
		return nil, errors.New("s3 storage: bucket and region are required")
	}

	// loadOpts is a list of "tweaks" we hand to the AWS SDK. We start
	// with the region, then conditionally add credentials below.
	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
	}

	// Two paths here:
	//
	// 1. Config provides access_key + secret_key (typical in dev/staging,
	//    or on non-AWS infra) — use them directly.
	//
	// 2. Neither provided — let the SDK look elsewhere. It checks, in order:
	//    env vars (AWS_ACCESS_KEY_ID), the ~/.aws/credentials file, and
	//    finally IAM role metadata if running on AWS.
	if opts["access_key"] != "" && opts["secret_key"] != "" {
		loadOpts = append(loadOpts,
			awsconfig.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(opts["access_key"], opts["secret_key"], ""),
			),
		)
	}

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("s3 storage: load aws config: %w", err)
	}

	// If "endpoint" is set, we're talking to something that speaks the S3
	// API but isn't actually AWS (MinIO, DigitalOcean Spaces, etc.).
	// Those services need two tweaks:
	//   BaseEndpoint — point the client at their URL instead of aws.com
	//   UsePathStyle — use "host.com/bucket/key" instead of AWS's
	//                  "bucket.host.com/key" format
	var client *awss3.Client
	if endpoint := opts["endpoint"]; endpoint != "" {
		client = awss3.NewFromConfig(cfg, func(o *awss3.Options) {
			o.BaseEndpoint = &endpoint
			o.UsePathStyle = true
		})
	} else {
		client = awss3.NewFromConfig(cfg)
	}

	return &S3{
		client:  client,
		// presign is a separate client dedicated to generating signed URLs.
		// It shares the same config/credentials as the main client.
		presign: awss3.NewPresignClient(client),
		bucket:  bucket,
	}, nil
}

func (s *S3) Name() string { return "s3" }

func (s *S3) Upload(ctx context.Context, in storage.UploadInput) (*storage.FileMetadata, error) {
	// PutObject is the simplest way to upload — one request, whole file.
	// Fine for typical app uploads (images, PDFs up to ~hundreds of MB).
	//
	// For files >5GB or streams of unknown size, you'd switch to
	// s3manager.Uploader, which splits the upload into parts and
	// uploads them in parallel. Out of scope for this provider.
	input := &awss3.PutObjectInput{
		// The AWS SDK wants *string (pointer) instead of string. That's
		// so it can tell "not set" (nil) apart from "set to empty" ("").
		Bucket: &s.bucket,
		Key:    &in.Key,
		Body:   in.Body,
	}
	if in.ContentType != "" {
		input.ContentType = &in.ContentType
	}

	if _, err := s.client.PutObject(ctx, input); err != nil {
		return nil, fmt.Errorf("s3 upload: %w", err)
	}

	return &storage.FileMetadata{
		Key: in.Key,
		// Standard public URL format for AWS S3. If your bucket is
		// private (most are), this URL won't actually load in a browser —
		// you'd use SignedURL to hand out time-limited access instead.
		URL:      fmt.Sprintf("https://%s.s3.amazonaws.com/%s", s.bucket, in.Key),
		Size:     in.Size,
		Provider: "s3",
	}, nil
}

func (s *S3) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
	})
	if err != nil {
		// S3 returns a typed error (*s3types.NoSuchKey) when the key is
		// missing. errors.As unwraps the error chain looking for that
		// specific type — if found, we translate to our generic ErrNotFound
		// so callers can handle it the same way regardless of backend.
		var notFound *s3types.NoSuchKey
		if errors.As(err, &notFound) {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("s3 download: %w", err)
	}
	// out.Body is an io.ReadCloser over the HTTP response body. The caller
	// must Close() it when done or the connection leaks.
	return out.Body, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
	})
	if err != nil {
		return fmt.Errorf("s3 delete: %w", err)
	}
	// Note: S3 DeleteObject already treats missing keys as a success, so
	// we don't need the os.ErrNotExist check the local backend has.
	return nil
}

func (s *S3) SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	// "Presigning" = ask AWS to produce a URL that acts like it was signed
	// by the bucket owner. Anyone with the URL can download the file, but
	// only until it expires.
	//
	// Note: AWS caps signed URLs at 7 days. If you pass ttl=30*24*time.Hour,
	// AWS will reject it at signing time.
	req, err := s.presign.PresignGetObject(ctx, &awss3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
	}, awss3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("s3 signed url: %w", err)
	}
	return req.URL, nil
}
