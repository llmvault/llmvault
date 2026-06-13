package storage

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/usehivy/hivy/internal/microsandbox/config"
)

type SnapshotStore struct {
	bucket string
	prefix string
	client *s3.Client
}

func NewSnapshotStore(ctx context.Context, cfg config.Config) (*SnapshotStore, error) {
	if cfg.SnapshotS3Bucket == "" {
		return nil, nil
	}
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.SnapshotS3Region),
	}
	if cfg.SnapshotS3AccessKeyID != "" || cfg.SnapshotS3SecretAccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.SnapshotS3AccessKeyID,
			cfg.SnapshotS3SecretAccessKey,
			"",
		)))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.SnapshotS3PathStyle
		if cfg.SnapshotS3Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.SnapshotS3Endpoint)
		}
	})
	return &SnapshotStore{bucket: cfg.SnapshotS3Bucket, prefix: strings.Trim(cfg.SnapshotS3Prefix, "/"), client: client}, nil
}

func (s *SnapshotStore) Upload(ctx context.Context, localPath, snapshotID string) (string, error) {
	if s == nil {
		return "file://" + localPath, nil
	}
	file, err := os.Open(localPath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	key := path.Join(s.prefix, snapshotID+".tar")
	if _, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String("application/x-tar"),
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf("s3://%s/%s", s.bucket, key), nil
}
