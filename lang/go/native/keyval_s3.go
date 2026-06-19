//go:build !js

package native

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3KVStore is a KeyValBackend backed by an S3-compatible object store
// (AWS S3, MinIO, GCS interop). Each key is an object; values are stored
// as JSON bytes (shared codec). Opened with an s3:// URL:
//
//	s3://ACCESS_KEY:SECRET_KEY@endpoint/bucket?region=us-east-1&ssl=true
//
// endpoint is the host[:port] of the service (e.g. s3.amazonaws.com or a
// MinIO host); bucket is the first path segment.
type S3KVStore struct {
	client *minio.Client
	bucket string
}

var _ KeyValBackend = (*S3KVStore)(nil)

const kvS3Timeout = 30 * time.Second

// NewS3KVStore connects to an S3-compatible endpoint and verifies the
// bucket exists.
func NewS3KVStore(dsn string) (*S3KVStore, error) {
	cfg, err := parseS3DSN(dsn)
	if err != nil {
		return nil, err
	}
	client, err := minio.New(cfg.endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.accessKey, cfg.secretKey, ""),
		Secure: cfg.secure,
		Region: cfg.region,
	})
	if err != nil {
		return nil, fmt.Errorf("s3 client: %w", redactError(err, dsn))
	}
	ctx, cancel := context.WithTimeout(context.Background(), kvS3Timeout)
	defer cancel()
	exists, err := client.BucketExists(ctx, cfg.bucket)
	if err != nil {
		return nil, fmt.Errorf("s3 connect: %w", redactError(err, dsn))
	}
	if !exists {
		return nil, fmt.Errorf("s3: bucket %q does not exist", cfg.bucket)
	}
	return &S3KVStore{client: client, bucket: cfg.bucket}, nil
}

type s3Config struct {
	endpoint, bucket, region, accessKey, secretKey string
	secure                                         bool
}

// parseS3DSN parses an s3:// connection URL.
func parseS3DSN(dsn string) (s3Config, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return s3Config{}, fmt.Errorf("s3 url: %w", redactError(err, dsn))
	}
	cfg := s3Config{
		endpoint: u.Host,
		bucket:   strings.Trim(u.Path, "/"),
		region:   u.Query().Get("region"),
		secure:   u.Query().Get("ssl") != "false", // default to TLS
	}
	if u.User != nil {
		cfg.accessKey = u.User.Username()
		cfg.secretKey, _ = u.User.Password()
	}
	if cfg.endpoint == "" {
		return s3Config{}, errors.New("s3 url must include an endpoint host")
	}
	if cfg.bucket == "" {
		return s3Config{}, errors.New("s3 url must include a bucket path, e.g. s3://endpoint/mybucket")
	}
	if cfg.region == "" {
		cfg.region = "us-east-1"
	}
	return cfg, nil
}

func (s *S3KVStore) Get(key string) (Value, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), kvS3Timeout)
	defer cancel()
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return Value{}, false, err
	}
	defer obj.Close()
	b, err := io.ReadAll(obj)
	if err != nil {
		if isS3NotFound(err) {
			return Value{}, false, nil
		}
		return Value{}, false, err
	}
	v, err := kvBytesToValue(b)
	if err != nil {
		return Value{}, false, err
	}
	return v, true, nil
}

func (s *S3KVStore) Set(key string, val Value) error {
	b, err := valueToKVBytes(val)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), kvS3Timeout)
	defer cancel()
	_, err = s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(b), int64(len(b)),
		minio.PutObjectOptions{ContentType: "application/json"})
	return err
}

func (s *S3KVStore) Delete(key string) (bool, error) {
	existed, err := s.Has(key)
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), kvS3Timeout)
	defer cancel()
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return false, err
	}
	return existed, nil
}

func (s *S3KVStore) Has(key string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), kvS3Timeout)
	defer cancel()
	_, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if isS3NotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *S3KVStore) Keys(prefix string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), kvS3Timeout)
	defer cancel()
	var out []string
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		out = append(out, obj.Key)
	}
	sort.Strings(out)
	return out, nil
}

func (s *S3KVStore) Kind() string { return "s3" }

func (s *S3KVStore) Close() error { return nil }

// isS3NotFound reports whether err is an S3 "no such key" error.
func isS3NotFound(err error) bool {
	resp := minio.ToErrorResponse(err)
	return resp.Code == "NoSuchKey" || resp.StatusCode == 404
}
