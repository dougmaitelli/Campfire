// Package backupstorage implements off-host archive storage using the AWS SDK.
package backupstorage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

const partSize = 16 << 20

// Config identifies one operator-managed bucket. SDK credentials stay on Campfire.
type Config struct {
	Bucket, Region, Endpoint, Prefix string
	PathStyle                        bool
}

// S3 uses immutable object keys and records version IDs for exact deletion.
type S3 struct {
	client                   *s3.Client
	bucket, prefix, identity string
}

// New returns nil when S3 is not configured, or a credential-chain-backed client.
func New(ctx context.Context, c Config) (*S3, error) {
	if c.Bucket == "" {
		if c.Endpoint != "" || c.Prefix != "" {
			return nil, errors.New("S3 endpoint or prefix requires a bucket")
		}

		return nil, nil
	}

	if c.Endpoint != "" {
		u, err := url.Parse(c.Endpoint)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, errors.New("invalid S3 endpoint")
		}
	}

	if strings.ContainsAny(c.Prefix, "\\\x00\r\n") || strings.Contains(c.Prefix, "..") {
		return nil, errors.New("invalid S3 prefix")
	}

	if c.Region == "" {
		c.Region = "us-east-1"
	}

	if c.Prefix == "" {
		c.Prefix = "campfire"
	}

	c.Prefix = strings.Trim(c.Prefix, "/") + "/"

	options, err := config.LoadDefaultConfig(ctx, config.WithRegion(c.Region))
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(options, func(o *s3.Options) {
		o.UsePathStyle = c.PathStyle
		if c.Endpoint != "" {
			o.BaseEndpoint = aws.String(strings.TrimRight(c.Endpoint, "/"))
		}
	})
	id := sha256.Sum256([]byte(c.Endpoint + "\n" + c.Bucket + "\n" + c.Prefix))

	return &S3{client: client, bucket: c.Bucket, prefix: c.Prefix, identity: hex.EncodeToString(id[:])}, nil
}

// ID prevents using changed bucket settings to operate on an earlier archive.
func (s *S3) ID() string { return s.identity }

func (s *S3) head(ctx context.Context, key string) (*s3.HeadObjectOutput, error) {
	return s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: aws.String(s.prefix + key)})
}

func missing(err error) bool {
	var response *smithyhttp.ResponseError
	return errors.As(err, &response) && response.HTTPStatusCode() == http.StatusNotFound
}

// Put verifies the supplied SHA-256, uses checksummed multipart uploads for large
// archives, and recovers an already-completed upload after a lost acknowledgement.
func (s *S3) Put(ctx context.Context, key string, size int64, digest string, input io.Reader) (string, error) {
	if size < 1 || size > 100<<30 || len(digest) != 64 {
		return "", errors.New("invalid backup upload")
	}

	if existing, err := s.head(ctx, key); err == nil {
		if aws.ToInt64(existing.ContentLength) != size || existing.Metadata["sha256"] != digest {
			return "", errors.New("existing backup object does not match archive")
		}

		return aws.ToString(existing.VersionId), nil
	} else if !missing(err) {
		return "", err
	}

	object := s.prefix + key
	metadata := map[string]string{"sha256": digest}

	if size <= partSize {
		data, err := io.ReadAll(io.LimitReader(input, size+1))
		if err != nil {
			return "", err
		}

		hash := sha256.Sum256(data)
		if int64(len(data)) != size || hex.EncodeToString(hash[:]) != digest {
			return "", errors.New("backup changed during upload")
		}

		_, err = s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: &s.bucket, Key: &object, Body: bytes.NewReader(data), ContentLength: &size, ContentType: aws.String("application/gzip"), Metadata: metadata, ChecksumSHA256: aws.String(base64.StdEncoding.EncodeToString(hash[:])), IfNoneMatch: aws.String("*")})
		if err != nil {
			return "", err
		}
	} else {
		if err := s.multipart(ctx, object, size, digest, metadata, input); err != nil {
			return "", err
		}
	}

	verified, err := s.head(ctx, key)
	if err != nil {
		return "", err
	}

	if aws.ToInt64(verified.ContentLength) != size || verified.Metadata["sha256"] != digest {
		return "", errors.New("uploaded backup verification failed")
	}

	return aws.ToString(verified.VersionId), nil
}

func (s *S3) multipart(ctx context.Context, key string, size int64, digest string, metadata map[string]string, input io.Reader) error {
	created, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: &s.bucket, Key: &key, Metadata: metadata, ContentType: aws.String("application/gzip"), ChecksumAlgorithm: types.ChecksumAlgorithmSha256})
	if err != nil {
		return err
	}

	complete := false
	defer func() {
		if !complete {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()

			_, _ = s.client.AbortMultipartUpload(cleanup, &s3.AbortMultipartUploadInput{Bucket: &s.bucket, Key: &key, UploadId: created.UploadId})
		}
	}()

	hash := sha256.New()

	var parts []types.CompletedPart

	for remaining := size; remaining > 0; {
		data := make([]byte, min(int64(partSize), remaining))
		if _, err := io.ReadFull(input, data); err != nil {
			return err
		}

		_, _ = hash.Write(data)
		partHash := sha256.Sum256(data)
		checksum := base64.StdEncoding.EncodeToString(partHash[:])
		number := int32(len(parts) + 1)

		result, err := s.client.UploadPart(ctx, &s3.UploadPartInput{Bucket: &s.bucket, Key: &key, UploadId: created.UploadId, PartNumber: &number, Body: bytes.NewReader(data), ChecksumSHA256: &checksum})
		if err != nil {
			return err
		}

		if result.ChecksumSHA256 == nil || *result.ChecksumSHA256 != checksum {
			return errors.New("S3 multipart checksum mismatch")
		}

		parts = append(parts, types.CompletedPart{PartNumber: &number, ETag: result.ETag, ChecksumSHA256: &checksum})
		remaining -= int64(len(data))
	}

	if hex.EncodeToString(hash.Sum(nil)) != digest {
		return errors.New("backup changed during upload")
	}

	_, err = s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{Bucket: &s.bucket, Key: &key, UploadId: created.UploadId, MultipartUpload: &types.CompletedMultipartUpload{Parts: parts}, IfNoneMatch: aws.String("*")})
	complete = err == nil

	return err
}

// Read returns a bounded, version-pinned range without staging on panel disk.
func (s *S3) Read(ctx context.Context, key, version string, offset int64, count int) ([]byte, error) {
	request := &s3.GetObjectInput{Bucket: &s.bucket, Key: aws.String(s.prefix + key), Range: aws.String(fmt.Sprintf("bytes=%d-%d", offset, offset+int64(count)-1))}
	if version != "" {
		request.VersionId = &version
	}

	result, err := s.client.GetObject(ctx, request)
	if err != nil {
		return nil, err
	}

	defer func() { _ = result.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(result.Body, int64(count)+1))
	if err != nil {
		return nil, err
	}

	if len(data) != count {
		return nil, errors.New("unexpected backup object range")
	}

	return data, nil
}

// RestoreURL grants temporary read access to one exact object version.
func (s *S3) RestoreURL(ctx context.Context, key, version string) (string, error) {
	input := &s3.GetObjectInput{Bucket: &s.bucket, Key: aws.String(s.prefix + key)}
	if version != "" {
		input.VersionId = &version
	}

	result, err := s3.NewPresignClient(s.client).PresignGetObject(ctx, input, s3.WithPresignExpires(45*time.Minute))
	if err != nil {
		return "", err
	}

	return result.URL, nil
}

// Delete removes the owned object version, including an upload whose final
// acknowledgement was lost before its version ID reached the catalog.
func (s *S3) Delete(ctx context.Context, key, version string) error {
	if version == "" {
		existing, err := s.head(ctx, key)
		if missing(err) {
			return nil
		}

		if err != nil {
			return err
		}

		version = aws.ToString(existing.VersionId)
	}

	input := &s3.DeleteObjectInput{Bucket: &s.bucket, Key: aws.String(s.prefix + key)}
	if version != "" {
		input.VersionId = &version
	}

	_, err := s.client.DeleteObject(ctx, input)

	return err
}
