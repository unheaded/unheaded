package object

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/unheaded/unheaded/pkg/logger"
	"github.com/unheaded/unheaded/pkg/storage"
)

// S3Store is an S3-compatible object store.
// This is a mock implementation that provides the interface.
// In production, this would use github.com/aws/aws-sdk-go-v2/service/s3.
type S3Store struct {
	cfg     Config
	log     *logger.Logger
	client  *mockS3Client
	closed  bool
	mu      sync.RWMutex
}

// mockS3Client simulates an S3 client for testing/development.
type mockS3Client struct {
	buckets map[string]map[string]*mockObject
	mu      sync.RWMutex
}

type mockObject struct {
	data        []byte
	contentType string
	metadata    map[string]string
	etag        string
	lastModified time.Time
}

// NewS3Store creates a new S3-compatible object store.
func NewS3Store(cfg Config) (*S3Store, error) {
	log := cfg.Logger
	if log == nil {
		log = logger.New(os.Stdout)
	}

	// In production, this would initialize the AWS SDK client
	// awsCfg, err := config.LoadDefaultConfig(ctx,
	//     config.WithRegion(cfg.S3Config.Region),
	//     config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
	//         cfg.S3Config.AccessKey,
	//         cfg.S3Config.SecretKey,
	//         "",
	//     )),
	// )
	// if err != nil {
	//     return nil, fmt.Errorf("s3: load config: %w", err)
	// }
	//
	// client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
	//     if cfg.S3Config.Endpoint != "" {
	//         o.BaseEndpoint = aws.String(cfg.S3Config.Endpoint)
	//     }
	//     o.UsePathStyle = cfg.S3Config.UsePathStyle
	// })

	store := &S3Store{
		cfg: cfg,
		log: log.With().Str("backend", "s3").Logger(),
		client: &mockS3Client{
			buckets: make(map[string]map[string]*mockObject),
		},
	}

	store.log.Info().
		Str("endpoint", cfg.S3Config.Endpoint).
		Str("region", cfg.S3Config.Region).
		Msg("s3 object store initialized")

	return store, nil
}

// Put stores an object in the bucket.
func (s *S3Store) Put(ctx context.Context, bucket, key string, reader io.Reader, opts *storage.PutOptions) (*storage.ObjectInfo, error) {
	start := time.Now()

	if err := ValidateBucket(bucket); err != nil {
		recordOp("s3", "put", "error", time.Since(start).Seconds())
		return nil, err
	}
	if err := ValidateKey(key); err != nil {
		recordOp("s3", "put", "error", time.Since(start).Seconds())
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		recordOp("s3", "put", "error", time.Since(start).Seconds())
		return nil, storage.ErrClosed
	}

	// Read all data
	data, err := io.ReadAll(reader)
	if err != nil {
		recordOp("s3", "put", "error", time.Since(start).Seconds())
		return nil, fmt.Errorf("s3: read data: %w", err)
	}

	// Check size limit
	if s.cfg.MaxObjectSize > 0 && int64(len(data)) > s.cfg.MaxObjectSize {
		recordOp("s3", "put", "error", time.Since(start).Seconds())
		return nil, storage.ErrValueTooLarge
	}

	// In production:
	// input := &s3.PutObjectInput{
	//     Bucket: aws.String(bucket),
	//     Key:    aws.String(key),
	//     Body:   bytes.NewReader(data),
	// }
	// if opts != nil {
	//     if opts.ContentType != "" {
	//         input.ContentType = aws.String(opts.ContentType)
	//     }
	//     if opts.Metadata != nil {
	//         input.Metadata = opts.Metadata
	//     }
	// }
	// result, err := s.client.PutObject(ctx, input)

	// Mock implementation
	s.client.mu.Lock()
	defer s.client.mu.Unlock()

	if _, ok := s.client.buckets[bucket]; !ok {
		recordOp("s3", "put", "error", time.Since(start).Seconds())
		return nil, storage.ErrBucketNotFound
	}

	contentType := "application/octet-stream"
	var metadata map[string]string
	if opts != nil {
		if opts.ContentType != "" {
			contentType = opts.ContentType
		}
		metadata = opts.Metadata
	}

	etag := GenerateETag(data)
	now := time.Now()

	s.client.buckets[bucket][key] = &mockObject{
		data:         data,
		contentType:  contentType,
		metadata:     metadata,
		etag:         etag,
		lastModified: now,
	}

	info := &storage.ObjectInfo{
		Key:          key,
		Size:         int64(len(data)),
		ContentType:  contentType,
		ETag:         etag,
		LastModified: now,
		Metadata:     metadata,
	}

	recordOp("s3", "put", "success", time.Since(start).Seconds())
	recordTransfer("s3", "upload", int64(len(data)))
	recordSize("s3", int64(len(data)))

	s.log.Debug().
		Str("bucket", bucket).
		Str("key", key).
		Int("size", len(data)).
		Msg("object stored")

	return info, nil
}

// Get retrieves an object from the bucket.
func (s *S3Store) Get(ctx context.Context, bucket, key string) (io.ReadCloser, *storage.ObjectInfo, error) {
	start := time.Now()

	if err := ValidateBucket(bucket); err != nil {
		recordOp("s3", "get", "error", time.Since(start).Seconds())
		return nil, nil, err
	}
	if err := ValidateKey(key); err != nil {
		recordOp("s3", "get", "error", time.Since(start).Seconds())
		return nil, nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		recordOp("s3", "get", "error", time.Since(start).Seconds())
		return nil, nil, storage.ErrClosed
	}

	// In production:
	// result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
	//     Bucket: aws.String(bucket),
	//     Key:    aws.String(key),
	// })
	// if err != nil {
	//     var nsk *types.NoSuchKey
	//     if errors.As(err, &nsk) {
	//         return nil, nil, storage.ErrObjectNotFound
	//     }
	//     return nil, nil, fmt.Errorf("s3: get object: %w", err)
	// }

	// Mock implementation
	s.client.mu.RLock()
	defer s.client.mu.RUnlock()

	bucketData, ok := s.client.buckets[bucket]
	if !ok {
		recordOp("s3", "get", "not_found", time.Since(start).Seconds())
		return nil, nil, storage.ErrBucketNotFound
	}

	obj, ok := bucketData[key]
	if !ok {
		recordOp("s3", "get", "not_found", time.Since(start).Seconds())
		return nil, nil, storage.ErrObjectNotFound
	}

	info := &storage.ObjectInfo{
		Key:          key,
		Size:         int64(len(obj.data)),
		ContentType:  obj.contentType,
		ETag:         obj.etag,
		LastModified: obj.lastModified,
		Metadata:     obj.metadata,
	}

	recordOp("s3", "get", "success", time.Since(start).Seconds())
	recordTransfer("s3", "download", int64(len(obj.data)))

	// Return a copy of the data
	dataCopy := make([]byte, len(obj.data))
	copy(dataCopy, obj.data)

	return io.NopCloser(bytes.NewReader(dataCopy)), info, nil
}

// Delete removes an object from the bucket.
func (s *S3Store) Delete(ctx context.Context, bucket, key string) error {
	start := time.Now()

	if err := ValidateBucket(bucket); err != nil {
		recordOp("s3", "delete", "error", time.Since(start).Seconds())
		return err
	}
	if err := ValidateKey(key); err != nil {
		recordOp("s3", "delete", "error", time.Since(start).Seconds())
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		recordOp("s3", "delete", "error", time.Since(start).Seconds())
		return storage.ErrClosed
	}

	// In production:
	// _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
	//     Bucket: aws.String(bucket),
	//     Key:    aws.String(key),
	// })

	// Mock implementation
	s.client.mu.Lock()
	defer s.client.mu.Unlock()

	bucketData, ok := s.client.buckets[bucket]
	if !ok {
		recordOp("s3", "delete", "not_found", time.Since(start).Seconds())
		return storage.ErrBucketNotFound
	}

	if _, ok := bucketData[key]; !ok {
		recordOp("s3", "delete", "not_found", time.Since(start).Seconds())
		return storage.ErrObjectNotFound
	}

	delete(bucketData, key)

	recordOp("s3", "delete", "success", time.Since(start).Seconds())
	s.log.Debug().Str("bucket", bucket).Str("key", key).Msg("object deleted")
	return nil
}

// List lists objects in the bucket with the given prefix.
func (s *S3Store) List(ctx context.Context, bucket, prefix string) ([]*storage.ObjectInfo, error) {
	start := time.Now()

	if err := ValidateBucket(bucket); err != nil {
		recordOp("s3", "list", "error", time.Since(start).Seconds())
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		recordOp("s3", "list", "error", time.Since(start).Seconds())
		return nil, storage.ErrClosed
	}

	// In production:
	// paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
	//     Bucket: aws.String(bucket),
	//     Prefix: aws.String(prefix),
	// })

	// Mock implementation
	s.client.mu.RLock()
	defer s.client.mu.RUnlock()

	bucketData, ok := s.client.buckets[bucket]
	if !ok {
		recordOp("s3", "list", "not_found", time.Since(start).Seconds())
		return nil, storage.ErrBucketNotFound
	}

	var objects []*storage.ObjectInfo
	for key, obj := range bucketData {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if prefix != "" && len(key) < len(prefix) {
			continue
		}
		if prefix != "" && key[:len(prefix)] != prefix {
			continue
		}

		objects = append(objects, &storage.ObjectInfo{
			Key:          key,
			Size:         int64(len(obj.data)),
			ContentType:  obj.contentType,
			ETag:         obj.etag,
			LastModified: obj.lastModified,
			Metadata:     obj.metadata,
		})
	}

	// Sort by key
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].Key < objects[j].Key
	})

	recordOp("s3", "list", "success", time.Since(start).Seconds())
	return objects, nil
}

// Stat returns object metadata without downloading.
func (s *S3Store) Stat(ctx context.Context, bucket, key string) (*storage.ObjectInfo, error) {
	start := time.Now()

	if err := ValidateBucket(bucket); err != nil {
		recordOp("s3", "stat", "error", time.Since(start).Seconds())
		return nil, err
	}
	if err := ValidateKey(key); err != nil {
		recordOp("s3", "stat", "error", time.Since(start).Seconds())
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		recordOp("s3", "stat", "error", time.Since(start).Seconds())
		return nil, storage.ErrClosed
	}

	// In production:
	// result, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
	//     Bucket: aws.String(bucket),
	//     Key:    aws.String(key),
	// })

	// Mock implementation
	s.client.mu.RLock()
	defer s.client.mu.RUnlock()

	bucketData, ok := s.client.buckets[bucket]
	if !ok {
		recordOp("s3", "stat", "not_found", time.Since(start).Seconds())
		return nil, storage.ErrBucketNotFound
	}

	obj, ok := bucketData[key]
	if !ok {
		recordOp("s3", "stat", "not_found", time.Since(start).Seconds())
		return nil, storage.ErrObjectNotFound
	}

	info := &storage.ObjectInfo{
		Key:          key,
		Size:         int64(len(obj.data)),
		ContentType:  obj.contentType,
		ETag:         obj.etag,
		LastModified: obj.lastModified,
		Metadata:     obj.metadata,
	}

	recordOp("s3", "stat", "success", time.Since(start).Seconds())
	return info, nil
}

// CreateBucket creates a new bucket.
func (s *S3Store) CreateBucket(ctx context.Context, bucket string) error {
	start := time.Now()

	if err := ValidateBucket(bucket); err != nil {
		recordOp("s3", "create_bucket", "error", time.Since(start).Seconds())
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		recordOp("s3", "create_bucket", "error", time.Since(start).Seconds())
		return storage.ErrClosed
	}

	// In production:
	// _, err := s.client.CreateBucket(ctx, &s3.CreateBucketInput{
	//     Bucket: aws.String(bucket),
	// })

	// Mock implementation
	s.client.mu.Lock()
	defer s.client.mu.Unlock()

	if _, ok := s.client.buckets[bucket]; ok {
		recordOp("s3", "create_bucket", "error", time.Since(start).Seconds())
		return fmt.Errorf("s3: bucket already exists")
	}

	s.client.buckets[bucket] = make(map[string]*mockObject)

	recordOp("s3", "create_bucket", "success", time.Since(start).Seconds())
	s.log.Info().Str("bucket", bucket).Msg("bucket created")
	return nil
}

// DeleteBucket deletes a bucket. Must be empty.
func (s *S3Store) DeleteBucket(ctx context.Context, bucket string) error {
	start := time.Now()

	if err := ValidateBucket(bucket); err != nil {
		recordOp("s3", "delete_bucket", "error", time.Since(start).Seconds())
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		recordOp("s3", "delete_bucket", "error", time.Since(start).Seconds())
		return storage.ErrClosed
	}

	// In production:
	// _, err := s.client.DeleteBucket(ctx, &s3.DeleteBucketInput{
	//     Bucket: aws.String(bucket),
	// })

	// Mock implementation
	s.client.mu.Lock()
	defer s.client.mu.Unlock()

	bucketData, ok := s.client.buckets[bucket]
	if !ok {
		recordOp("s3", "delete_bucket", "not_found", time.Since(start).Seconds())
		return storage.ErrBucketNotFound
	}

	if len(bucketData) > 0 {
		recordOp("s3", "delete_bucket", "error", time.Since(start).Seconds())
		return fmt.Errorf("s3: bucket not empty")
	}

	delete(s.client.buckets, bucket)

	recordOp("s3", "delete_bucket", "success", time.Since(start).Seconds())
	s.log.Info().Str("bucket", bucket).Msg("bucket deleted")
	return nil
}

// ListBuckets returns all buckets.
func (s *S3Store) ListBuckets(ctx context.Context) ([]string, error) {
	start := time.Now()

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		recordOp("s3", "list_buckets", "error", time.Since(start).Seconds())
		return nil, storage.ErrClosed
	}

	// In production:
	// result, err := s.client.ListBuckets(ctx, &s3.ListBucketsInput{})

	// Mock implementation
	s.client.mu.RLock()
	defer s.client.mu.RUnlock()

	var buckets []string
	for name := range s.client.buckets {
		buckets = append(buckets, name)
	}

	sort.Strings(buckets)
	recordOp("s3", "list_buckets", "success", time.Since(start).Seconds())
	return buckets, nil
}

// Close releases resources.
func (s *S3Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	s.log.Info().Msg("s3 object store closed")
	return nil
}

// PresignGet generates a presigned URL for downloading.
func (s *S3Store) PresignGet(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	// In production:
	// presigner := s3.NewPresignClient(s.client)
	// req, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
	//     Bucket: aws.String(bucket),
	//     Key:    aws.String(key),
	// }, s3.WithPresignExpires(expiry))
	// if err != nil {
	//     return "", err
	// }
	// return req.URL, nil

	// Mock: return a fake URL
	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s?X-Amz-Expires=%d",
		bucket, key, int(expiry.Seconds())), nil
}

// PresignPut generates a presigned URL for uploading.
func (s *S3Store) PresignPut(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	// In production:
	// presigner := s3.NewPresignClient(s.client)
	// req, err := presigner.PresignPutObject(ctx, &s3.PutObjectInput{
	//     Bucket: aws.String(bucket),
	//     Key:    aws.String(key),
	// }, s3.WithPresignExpires(expiry))
	// if err != nil {
	//     return "", err
	// }
	// return req.URL, nil

	// Mock: return a fake URL
	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s?X-Amz-Expires=%d",
		bucket, key, int(expiry.Seconds())), nil
}

// Copy copies an object within the store.
func (s *S3Store) Copy(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) (*storage.ObjectInfo, error) {
	start := time.Now()

	// In production:
	// _, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
	//     Bucket:     aws.String(dstBucket),
	//     Key:        aws.String(dstKey),
	//     CopySource: aws.String(url.PathEscape(srcBucket + "/" + srcKey)),
	// })

	// Mock: Get and Put
	reader, info, err := s.Get(ctx, srcBucket, srcKey)
	if err != nil {
		recordOp("s3", "copy", "error", time.Since(start).Seconds())
		return nil, err
	}
	defer reader.Close()

	opts := &storage.PutOptions{
		ContentType: info.ContentType,
		Metadata:    info.Metadata,
	}

	result, err := s.Put(ctx, dstBucket, dstKey, reader, opts)
	if err != nil {
		recordOp("s3", "copy", "error", time.Since(start).Seconds())
		return nil, err
	}

	recordOp("s3", "copy", "success", time.Since(start).Seconds())
	return result, nil
}
