package s3

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Storage handles serverless offloading of mmap indices into cloud object stores deeply natively
type Storage struct {
	client *s3.Client
	bucket string
}

// NewStorage configures the explicit AWS V2 native credentials from local or IAM environments
func NewStorage(ctx context.Context, region string, bucketName string) (*Storage, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS SDK config: %w", err)
	}

	return &Storage{
		client: s3.NewFromConfig(cfg),
		bucket: bucketName,
	}, nil
}

// UploadFile streams the heavily bound local NVMe vectors/shards directly onto S3 natively saving IOPS
func (s *Storage) UploadFile(ctx context.Context, localPath string, s3Key string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("unable to open local file %s: %w", localPath, err)
	}
	defer file.Close()

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key),
		Body:   file,
	})

	if err != nil {
		return fmt.Errorf("failed to stream mmap into S3 bucket %s: %w", s.bucket, err)
	}
	return nil
}

// DownloadFile stream retrieves the serverless chunks natively securely onto disk blocks internally
func (s *Storage) DownloadFile(ctx context.Context, s3Key string, destPath string) error {
	resp, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key),
	})
	if err != nil {
		return fmt.Errorf("failed to retrieve object from S3: %w", err)
	}
	defer resp.Body.Close()

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to allocate dest disk block: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write streams to internal volume disks natively: %w", err)
	}
	return nil
}
