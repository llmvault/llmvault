package storage

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
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

type Artifact struct {
	URL         string
	Digest      string
	SizeBytes   int64
	ContentType string
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

func (s *SnapshotStore) Upload(ctx context.Context, localPath, snapshotID string) (*Artifact, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return nil, err
	}
	digest, err := FileSHA256(localPath)
	if err != nil {
		return nil, err
	}
	ext, contentType := artifactExtensionAndType(localPath)
	artifact := &Artifact{
		URL:         "file://" + localPath,
		Digest:      digest,
		SizeBytes:   info.Size(),
		ContentType: contentType,
	}
	if s == nil {
		return artifact, nil
	}
	file, err := os.Open(localPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	key := path.Join(s.prefix, snapshotID+ext)
	if _, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(contentType),
	}); err != nil {
		return nil, err
	}
	artifact.URL = fmt.Sprintf("s3://%s/%s", s.bucket, key)
	return artifact, nil
}

func (s *SnapshotStore) Download(ctx context.Context, artifactURL, localPath string) error {
	parsed, err := url.Parse(artifactURL)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return err
	}
	switch parsed.Scheme {
	case "file":
		return copyFile(parsed.Path, localPath)
	case "s3":
		if s == nil {
			return fmt.Errorf("snapshot S3 storage is not configured")
		}
		resp, err := s.client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(parsed.Host),
			Key:    aws.String(strings.TrimPrefix(parsed.Path, "/")),
		})
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		out, err := os.Create(localPath)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, resp.Body)
		return err
	default:
		return fmt.Errorf("unsupported snapshot artifact URL scheme %q", parsed.Scheme)
	}
}

func (s *SnapshotStore) Delete(ctx context.Context, artifactURL string) error {
	if artifactURL == "" {
		return nil
	}
	parsed, err := url.Parse(artifactURL)
	if err != nil {
		return err
	}
	switch parsed.Scheme {
	case "file":
		if err := os.Remove(parsed.Path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	case "s3":
		if s == nil {
			return fmt.Errorf("snapshot S3 storage is not configured")
		}
		_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(parsed.Host),
			Key:    aws.String(strings.TrimPrefix(parsed.Path, "/")),
		})
		return err
	default:
		return fmt.Errorf("unsupported snapshot artifact URL scheme %q", parsed.Scheme)
	}
}

func FileSHA256(localPath string) (string, error) {
	file, err := os.Open(localPath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), nil
}

func artifactExtensionAndType(localPath string) (string, string) {
	if strings.HasSuffix(localPath, ".tar.zst") {
		return ".tar.zst", "application/zstd"
	}
	if ext := filepath.Ext(localPath); ext != "" {
		return ext, "application/octet-stream"
	}
	return ".tar", "application/x-tar"
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
