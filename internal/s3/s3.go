package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type (
	// S3Config is the subconfig for the S3 storage type
	S3Config struct {
		Endpoint        string `json:"endpoint,omitempty"`
		Region          string `json:"region"`
		AccessKeyID     string `json:"access_key_id"`
		SecretAccessKey string `json:"secret_access_key"`
		Bucket          string `json:"bucket"`
	}

	// S3Client is a client for uploading data to S3.
	S3Client struct {
		endpoint  string
		region    string
		accessKey string
		secretKey string
		bucket    string

		// These fields are used for testing via dependency injection.
		uploader   uploader
		downloader downloader
	}
)

// NewS3Client returns an instance of an S3Client.
func NewS3Client(endpoint, region, accessKey, secretKey, bucket string) *S3Client {
	return &S3Client{
		endpoint:  endpoint,
		region:    region,
		accessKey: accessKey,
		secretKey: secretKey,
		bucket:    bucket,
	}
}

// String returns a string representation of the S3Client.
func (s *S3Client) String(key string) string {
	return fmt.Sprintf("s3://%s/%s", s.bucket, key)
}

// GetFileLink returns a presigned URL for a file in AWS S3.
func (s *S3Client) GetFileLink(key string) (string, error) {
	// Create a new S3 client from the configuration.
	sess, err := s.createSession()
	if err != nil {
		return "", err
	}

	presignClient := s3.New(sess)
	req, _ := presignClient.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	// Set 7 day expire, TODO: configs
	httpURL, err := req.Presign(time.Duration(7*24) * time.Hour)
	if err != nil {
		return "", err
	}
	return httpURL, nil
}

// Upload uploads data to S3.
func (s *S3Client) Upload(ctx context.Context, key string, reader io.Reader) error {
	sess, err := s.createSession()
	if err != nil {
		return err
	}

	// If an uploader was not provided, use a real S3 uploader.
	var uploader uploader
	if s.uploader == nil {
		uploader = s3manager.NewUploader(sess)
	} else {
		uploader = s.uploader
	}

	_, err = uploader.UploadWithContext(ctx, &s3manager.UploadInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   reader,
	})
	if err != nil {
		return fmt.Errorf("failed to upload to %v: %w", s, err)
	}
	return nil
}

// UploadString uploads string to S3.
func (s *S3Client) UploadString(ctx context.Context, key string, buffer []byte) error {
	return s.Upload(ctx, key, bytes.NewReader(buffer))
}

// Download downloads data from S3.
func (s *S3Client) Download(ctx context.Context, key string, writer io.WriterAt) error {
	sess, err := s.createSession()
	if err != nil {
		return err
	}

	// If a downloader was not provided, use a real S3 downloader.
	var downloader downloader
	if s.downloader == nil {
		downloader = s3manager.NewDownloader(sess)
	} else {
		downloader = s.downloader
	}

	_, err = downloader.DownloadWithContext(ctx, writer, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to download from %v: %w", s, err)
	}
	return nil
}

// DownloadString downloads string to S3.
func (s *S3Client) DownloadString(ctx context.Context, key string) ([]byte, error) {
	sess, err := s.createSession()
	if err != nil {
		return nil, err
	}

	s3Client := s3.New(sess)
	req := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	resp, err := s3Client.GetObject(req)
	if err != nil {
		return nil, err
	}

	return io.ReadAll(resp.Body)
}

func (s *S3Client) createSession() (*session.Session, error) {
	disableSSL := false
	sess, err := session.NewSession(&aws.Config{
		Endpoint:    aws.String(s.endpoint),
		Region:      aws.String(s.region),
		Credentials: credentials.NewStaticCredentials(s.accessKey, s.secretKey, ""),
		DisableSSL:  &disableSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 session: %w", err)
	}
	return sess, nil
}

type uploader interface {
	UploadWithContext(ctx aws.Context, input *s3manager.UploadInput, opts ...func(*s3manager.Uploader)) (*s3manager.UploadOutput, error)
}

type downloader interface {
	DownloadWithContext(ctx aws.Context, w io.WriterAt, input *s3.GetObjectInput, opts ...func(*s3manager.Downloader)) (n int64, err error)
}
