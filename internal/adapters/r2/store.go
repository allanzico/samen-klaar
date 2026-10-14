package r2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Store struct {
	client *s3.Client
	bucket string
}

func New(ctx context.Context) (*Store, error) {
	accessKey := strings.TrimSpace(os.Getenv("CLOUDFLARE_R2_ACCESS_KEY_ID"))
	secretKey := strings.TrimSpace(os.Getenv("CLOUDFLARE_R2_SECRET_ACCESS_KEY"))
	bucket := strings.TrimSpace(os.Getenv("CLOUDFLARE_R2_BUCKET"))
	endpoint := strings.TrimSpace(os.Getenv("CLOUDFLARE_R2_ENDPOINT"))
	if accessKey == "" || secretKey == "" || bucket == "" {
		return nil, errors.New("Cloudflare R2 access key, secret key, and bucket are required when EVIDENCE_PROVIDER=r2")
	}
	if endpoint == "" {
		accountID := strings.TrimSpace(os.Getenv("CLOUDFLARE_R2_ACCOUNT_ID"))
		if accountID == "" {
			return nil, errors.New("CLOUDFLARE_R2_ACCOUNT_ID or CLOUDFLARE_R2_ENDPOINT is required")
		}
		endpoint = "https://" + accountID + ".r2.cloudflarestorage.com"
	}
	region := strings.TrimSpace(os.Getenv("CLOUDFLARE_R2_REGION"))
	if region == "" {
		region = "auto"
	}
	pathStyle := strings.EqualFold(strings.TrimSpace(os.Getenv("CLOUDFLARE_R2_USE_PATH_STYLE_ENDPOINT")), "true")
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("configure R2 client: %w", err)
	}
	client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = pathStyle
	})
	return &Store{client: client, bucket: bucket}, nil
}

func (r *Store) objectKey(jobID, assetID string) string {
	return "evidence/" + url.PathEscape(jobID) + "/" + url.PathEscape(assetID)
}

func (r *Store) Put(ctx context.Context, jobID, assetID, contentType string, content []byte) error {
	_, err := r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(r.bucket), Key: aws.String(r.objectKey(jobID, assetID)),
		Body: bytes.NewReader(content), ContentType: aws.String(contentType),
	})
	if err != nil {
		return errors.New("Cloudflare R2 upload failed")
	}
	return nil
}

func (r *Store) Get(ctx context.Context, jobID, assetID string) (io.ReadCloser, string, error) {
	result, err := r.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(r.objectKey(jobID, assetID))})
	if err != nil {
		return nil, "", errors.New("evidence file is unavailable")
	}
	contentType := "application/octet-stream"
	if result.ContentType != nil {
		contentType = *result.ContentType
	}
	return result.Body, contentType, nil
}

func (r *Store) Delete(ctx context.Context, jobID, assetID string) {
	_, _ = r.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(r.objectKey(jobID, assetID))})
}
