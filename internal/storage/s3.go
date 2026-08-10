package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/shridarpatil/whatomate/internal/config"
)

// S3Client provides upload and presigned URL operations for call recordings.
type S3Client struct {
	client *s3.Client
	bucket string
}

// NewS3Client creates a new S3 client from the application's StorageConfig.
func NewS3Client(cfg *config.StorageConfig) (*S3Client, error) {
	if cfg.S3Bucket == "" || cfg.S3Region == "" {
		return nil, fmt.Errorf("s3_bucket and s3_region are required")
	}

	opts := s3.Options{
		Region: cfg.S3Region,
	}

	if cfg.S3Key != "" && cfg.S3Secret != "" {
		opts.Credentials = credentials.NewStaticCredentialsProvider(cfg.S3Key, cfg.S3Secret, "")
	}

	// A custom endpoint means we're talking to an S3-compatible provider other
	// than AWS (e.g. Google Cloud Storage's XML interoperability API), which
	// requires path-style addressing instead of AWS's virtual-hosted style.
	if cfg.S3Endpoint != "" {
		opts.BaseEndpoint = aws.String(cfg.S3Endpoint)
		opts.UsePathStyle = true

		// Since v1.73.0 the SDK adds CRC32 integrity headers to every eligible
		// request/response by default. GCS's XML API doesn't recognize them,
		// which breaks SigV4 verification with "SignatureDoesNotMatch" — only
		// compute/validate checksums when a caller explicitly asks for one.
		opts.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		opts.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired

		// Go's HTTP transport also adds its own Accept-Encoding header, which
		// the SDK includes in the SigV4 canonical request — but GCS strips or
		// alters that header before verifying, so the signature never matches.
		// See https://github.com/aws/aws-sdk-go-v2/issues/1816.
		opts.APIOptions = append(opts.APIOptions, dropHeaderFromSigning("Accept-Encoding"))
	}

	client := s3.New(opts)
	return &S3Client{client: client, bucket: cfg.S3Bucket}, nil
}

// Upload uploads a file to S3 at the given key.
func (s *S3Client) Upload(ctx context.Context, key string, body io.Reader, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	return err
}

// GetPresignedURL returns a time-limited download URL for the given S3 key.
func (s *S3Client) GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	presigner := s3.NewPresignClient(s.client)
	req, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// dropHeaderFromSigning removes the given header immediately before SigV4
// signing so its value (which some S3-compatible providers alter in transit)
// never becomes part of the canonical request the server verifies against.
func dropHeaderFromSigning(header string) func(*middleware.Stack) error {
	return func(stack *middleware.Stack) error {
		mw := middleware.FinalizeMiddlewareFunc("DropHeaderFromSigning", func(
			ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler,
		) (middleware.FinalizeOutput, middleware.Metadata, error) {
			if req, ok := in.Request.(*smithyhttp.Request); ok {
				req.Header.Del(header)
			}
			return next.HandleFinalize(ctx, in)
		})

		// Regular operations sign via a step named "Signing"; presign
		// operations (used for GetPresignedURL) build the request through
		// "PresignHTTPRequest" instead — there's no single anchor common to
		// both, so insert relative to whichever one this operation has.
		for _, anchor := range []string{"Signing", "PresignHTTPRequest"} {
			if _, ok := stack.Finalize.Get(anchor); ok {
				return stack.Finalize.Insert(mw, anchor, middleware.Before)
			}
		}
		return nil
	}
}
