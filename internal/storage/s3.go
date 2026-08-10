package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/logging"
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

		// The SDK unconditionally adds "Accept-Encoding: identity" (and signs
		// it) before handing the request to the configured signer. GCS's XML
		// API alters that header in transit before verifying, so the
		// signature never matches. Wrapping the signer — rather than trying
		// to anchor a Finalize middleware on a step name that differs between
		// regular and presign operations — strips it right at the one place
		// both code paths actually go through. See
		// https://github.com/aws/aws-sdk-go-v2/issues/1816.
		opts.HTTPSignerV4 = &headerStrippingSigner{
			wrapped: v4.NewSigner(),
			headers: []string{"Accept-Encoding"},
		}

		// TEMPORARY: this has failed with SignatureDoesNotMatch in production
		// while passing locally against the same bucket/credentials. Log the
		// exact canonical request/headers Cloud Run actually sends so the next
		// real failure is diagnosable instead of guessed at again. Remove once
		// root-caused.
		if os.Getenv("WHATOMATE_DEBUG_S3_SIGNING") != "" {
			opts.ClientLogMode = aws.LogSigning | aws.LogRequest
			opts.Logger = logging.NewStandardLogger(os.Stdout)
		}
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

// headerStrippingSigner deletes the given headers from the request
// immediately before delegating to the real SigV4 signer, so their values
// (which some S3-compatible providers alter in transit) never become part of
// the canonical request the server verifies against.
type headerStrippingSigner struct {
	wrapped s3.HTTPSignerV4
	headers []string
}

func (s *headerStrippingSigner) SignHTTP(
	ctx context.Context, credentials aws.Credentials, r *http.Request, payloadHash, service, region string,
	signingTime time.Time, optFns ...func(*v4.SignerOptions),
) error {
	for _, h := range s.headers {
		r.Header.Del(h)
	}
	return s.wrapped.SignHTTP(ctx, credentials, r, payloadHash, service, region, signingTime, optFns...)
}
