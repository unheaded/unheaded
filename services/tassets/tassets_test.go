package tassets

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestService creates a Service wired to a silent logger and default config.
// The busboy client is nil because Service never invokes it directly.
func newTestService(t *testing.T) *Service {
	t.Helper()
	log := logger.New(io.Discard)
	return NewService(log, nil, nil)
}

// newTestServiceWithConfig is like newTestService but accepts a custom Config.
func newTestServiceWithConfig(t *testing.T, cfg *Config) *Service {
	t.Helper()
	log := logger.New(io.Discard)
	return NewService(log, nil, cfg)
}

// mustCreateBucket is a helper that creates a bucket and fails the test on error.
func mustCreateBucket(t *testing.T, svc *Service, name string) {
	t.Helper()
	err := svc.CreateBucket(&Bucket{Name: name})
	if err != nil {
		t.Fatalf("CreateBucket(%q): unexpected error: %v", name, err)
	}
}

// mustPutObject is a helper that puts an object and fails the test on error.
func mustPutObject(t *testing.T, svc *Service, bucket, key string, data []byte) *Object {
	t.Helper()
	obj, err := svc.PutObject(bucket, key, data, nil)
	if err != nil {
		t.Fatalf("PutObject(%q, %q): unexpected error: %v", bucket, key, err)
	}
	return obj
}

// sha256Hex computes the hex-encoded SHA-256 digest of data.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// errWriter is an io.Writer that always returns an error.
type errWriter struct{ err error }

func (w *errWriter) Write([]byte) (int, error) { return 0, w.err }

// ---------------------------------------------------------------------------
// DefaultConfig tests
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DefaultStorageClass != StorageStandard {
		t.Errorf("DefaultStorageClass = %q, want %q", cfg.DefaultStorageClass, StorageStandard)
	}
	if cfg.MaxObjectSize != 5*1024*1024*1024 {
		t.Errorf("MaxObjectSize = %d, want %d", cfg.MaxObjectSize, 5*1024*1024*1024)
	}
	if cfg.DefaultRetention != 30 {
		t.Errorf("DefaultRetention = %d, want 30", cfg.DefaultRetention)
	}
	if !cfg.EnableEncryption {
		t.Error("EnableEncryption should be true by default")
	}
	if cfg.BusboyTopic != "tassets.storage" {
		t.Errorf("BusboyTopic = %q, want %q", cfg.BusboyTopic, "tassets.storage")
	}
}

// ---------------------------------------------------------------------------
// NewService tests
// ---------------------------------------------------------------------------

func TestNewService_NilConfig(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil, nil)

	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if svc.config == nil {
		t.Fatal("Service config is nil even though DefaultConfig should apply")
	}
	if svc.config.DefaultStorageClass != StorageStandard {
		t.Errorf("Expected default storage class, got %q", svc.config.DefaultStorageClass)
	}
}

func TestNewService_CustomConfig(t *testing.T) {
	cfg := &Config{
		DefaultStorageClass: StorageArchive,
		MaxObjectSize:       1024,
		DefaultRetention:    7,
		EnableEncryption:    false,
		BusboyTopic:         "custom.topic",
	}
	svc := newTestServiceWithConfig(t, cfg)

	if svc.config.DefaultStorageClass != StorageArchive {
		t.Errorf("Expected StorageArchive, got %q", svc.config.DefaultStorageClass)
	}
	if svc.config.MaxObjectSize != 1024 {
		t.Errorf("MaxObjectSize = %d, want 1024", svc.config.MaxObjectSize)
	}
	if svc.config.EnableEncryption {
		t.Error("Expected encryption disabled")
	}
}

func TestNewService_MapsInitialized(t *testing.T) {
	svc := newTestService(t)

	if svc.buckets == nil {
		t.Error("buckets map is nil")
	}
	if svc.objects == nil {
		t.Error("objects map is nil")
	}
	if svc.backups == nil {
		t.Error("backups map is nil")
	}
	if svc.pipelines == nil {
		t.Error("pipelines map is nil")
	}
	if svc.data == nil {
		t.Error("data map is nil")
	}
}

// ---------------------------------------------------------------------------
// Start / Stop tests
// ---------------------------------------------------------------------------

func TestStartStop(t *testing.T) {
	svc := newTestService(t)

	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: unexpected error: %v", err)
	}
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop: unexpected error: %v", err)
	}
}

func TestStart_CancelledContext(t *testing.T) {
	svc := newTestService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Start should still succeed (current implementation does not check ctx).
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start with cancelled context: unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Bucket tests (table-driven)
// ---------------------------------------------------------------------------

func TestCreateBucket(t *testing.T) {
	tests := []struct {
		name      string
		bucket    *Bucket
		wantErr   bool
		errSubstr string
	}{
		{
			name:   "valid bucket with defaults",
			bucket: &Bucket{Name: "my-bucket"},
		},
		{
			name:   "valid bucket with storage class",
			bucket: &Bucket{Name: "archive-bucket", StorageClass: StorageArchive},
		},
		{
			name:   "valid bucket with versioning",
			bucket: &Bucket{Name: "versioned", Versioning: true},
		},
		{
			name:   "valid bucket with tags",
			bucket: &Bucket{Name: "tagged", Tags: map[string]string{"env": "test"}},
		},
		{
			name:      "empty name",
			bucket:    &Bucket{Name: ""},
			wantErr:   true,
			errSubstr: "bucket name is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			err := svc.CreateBucket(tc.bucket)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify defaults were applied.
			if tc.bucket.StorageClass == "" {
				t.Error("StorageClass should have been set to default")
			}
			if tc.bucket.CreatedAt.IsZero() {
				t.Error("CreatedAt should have been set")
			}
		})
	}
}

func TestCreateBucket_Duplicate(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "dup")

	err := svc.CreateBucket(&Bucket{Name: "dup"})
	if err == nil {
		t.Fatal("expected error for duplicate bucket")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error %q does not mention 'already exists'", err.Error())
	}
}

func TestCreateBucket_EncryptionFromConfig(t *testing.T) {
	t.Run("encryption enabled", func(t *testing.T) {
		svc := newTestServiceWithConfig(t, &Config{EnableEncryption: true})
		b := &Bucket{Name: "enc"}
		if err := svc.CreateBucket(b); err != nil {
			t.Fatal(err)
		}
		if !b.Encryption {
			t.Error("bucket should have encryption enabled from config")
		}
	})

	t.Run("encryption disabled", func(t *testing.T) {
		svc := newTestServiceWithConfig(t, &Config{EnableEncryption: false})
		b := &Bucket{Name: "noenc"}
		if err := svc.CreateBucket(b); err != nil {
			t.Fatal(err)
		}
		if b.Encryption {
			t.Error("bucket should have encryption disabled from config")
		}
	})
}

func TestGetBucket(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "exists")

	t.Run("existing", func(t *testing.T) {
		b, ok := svc.GetBucket("exists")
		if !ok {
			t.Fatal("expected bucket to be found")
		}
		if b.Name != "exists" {
			t.Errorf("Name = %q, want %q", b.Name, "exists")
		}
	})

	t.Run("missing", func(t *testing.T) {
		_, ok := svc.GetBucket("nope")
		if ok {
			t.Error("expected bucket not to be found")
		}
	})
}

func TestListBuckets(t *testing.T) {
	svc := newTestService(t)

	t.Run("empty", func(t *testing.T) {
		buckets := svc.ListBuckets()
		if len(buckets) != 0 {
			t.Errorf("expected 0 buckets, got %d", len(buckets))
		}
	})

	t.Run("multiple", func(t *testing.T) {
		mustCreateBucket(t, svc, "a")
		mustCreateBucket(t, svc, "b")
		mustCreateBucket(t, svc, "c")

		buckets := svc.ListBuckets()
		if len(buckets) != 3 {
			t.Errorf("expected 3 buckets, got %d", len(buckets))
		}

		names := make(map[string]bool)
		for _, b := range buckets {
			names[b.Name] = true
		}
		for _, name := range []string{"a", "b", "c"} {
			if !names[name] {
				t.Errorf("bucket %q not found in list", name)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Object storage tests (table-driven)
// ---------------------------------------------------------------------------

func TestPutObject(t *testing.T) {
	tests := []struct {
		name      string
		bucket    string
		key       string
		data      []byte
		metadata  map[string]string
		wantErr   bool
		errSubstr string
	}{
		{
			name:   "basic put",
			bucket: "data",
			key:    "file.txt",
			data:   []byte("hello world"),
		},
		{
			name:     "with metadata",
			bucket:   "data",
			key:      "meta.txt",
			data:     []byte("with meta"),
			metadata: map[string]string{"author": "test"},
		},
		{
			name:   "empty data",
			bucket: "data",
			key:    "empty.txt",
			data:   []byte{},
		},
		{
			name:      "nonexistent bucket",
			bucket:    "no-such-bucket",
			key:       "file.txt",
			data:      []byte("data"),
			wantErr:   true,
			errSubstr: "bucket not found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			mustCreateBucket(t, svc, "data")

			obj, err := svc.PutObject(tc.bucket, tc.key, tc.data, tc.metadata)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if obj.Bucket != tc.bucket {
				t.Errorf("Bucket = %q, want %q", obj.Bucket, tc.bucket)
			}
			if obj.Key != tc.key {
				t.Errorf("Key = %q, want %q", obj.Key, tc.key)
			}
			if obj.Size != int64(len(tc.data)) {
				t.Errorf("Size = %d, want %d", obj.Size, len(tc.data))
			}
			if obj.Checksum != sha256Hex(tc.data) {
				t.Errorf("Checksum mismatch")
			}
			if obj.Version != 1 {
				t.Errorf("Version = %d, want 1", obj.Version)
			}
			if obj.ID != fmt.Sprintf("%s/%s", tc.bucket, tc.key) {
				t.Errorf("ID = %q, want %q", obj.ID, fmt.Sprintf("%s/%s", tc.bucket, tc.key))
			}
			if obj.CreatedAt.IsZero() {
				t.Error("CreatedAt should not be zero")
			}
			if obj.UpdatedAt.IsZero() {
				t.Error("UpdatedAt should not be zero")
			}
		})
	}
}

func TestPutObject_SizeLimit(t *testing.T) {
	cfg := &Config{MaxObjectSize: 10}
	svc := newTestServiceWithConfig(t, cfg)
	mustCreateBucket(t, svc, "small")

	// Exactly at limit should succeed.
	_, err := svc.PutObject("small", "ok.txt", make([]byte, 10), nil)
	if err != nil {
		t.Fatalf("object at exactly max size should succeed: %v", err)
	}

	// Over limit should fail.
	_, err = svc.PutObject("small", "big.txt", make([]byte, 11), nil)
	if err == nil {
		t.Fatal("expected error for oversized object")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("error %q does not mention exceeds maximum", err.Error())
	}
}

func TestPutObject_Versioning(t *testing.T) {
	svc := newTestService(t)
	err := svc.CreateBucket(&Bucket{Name: "ver", Versioning: true})
	if err != nil {
		t.Fatal(err)
	}

	obj1 := mustPutObject(t, svc, "ver", "doc.txt", []byte("v1"))
	if obj1.Version != 1 {
		t.Errorf("first version = %d, want 1", obj1.Version)
	}

	obj2 := mustPutObject(t, svc, "ver", "doc.txt", []byte("v2"))
	if obj2.Version != 2 {
		t.Errorf("second version = %d, want 2", obj2.Version)
	}

	obj3 := mustPutObject(t, svc, "ver", "doc.txt", []byte("v3"))
	if obj3.Version != 3 {
		t.Errorf("third version = %d, want 3", obj3.Version)
	}
}

func TestPutObject_NoVersioning(t *testing.T) {
	svc := newTestService(t)
	err := svc.CreateBucket(&Bucket{Name: "novers", Versioning: false})
	if err != nil {
		t.Fatal(err)
	}

	mustPutObject(t, svc, "novers", "doc.txt", []byte("v1"))
	obj2 := mustPutObject(t, svc, "novers", "doc.txt", []byte("v2"))

	// Without versioning, version stays at 1 on overwrite.
	if obj2.Version != 1 {
		t.Errorf("version without versioning = %d, want 1", obj2.Version)
	}
}

func TestPutObject_UpdatesBucketStats(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "stats")

	mustPutObject(t, svc, "stats", "a.txt", []byte("hello"))
	mustPutObject(t, svc, "stats", "b.txt", []byte("world!"))

	b, _ := svc.GetBucket("stats")
	if b.ObjectCount != 2 {
		t.Errorf("ObjectCount = %d, want 2", b.ObjectCount)
	}
	expectedSize := int64(len("hello") + len("world!"))
	if b.TotalSize != expectedSize {
		t.Errorf("TotalSize = %d, want %d", b.TotalSize, expectedSize)
	}
}

func TestPutObject_MetadataPreserved(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "meta")

	meta := map[string]string{"content-type": "text/plain", "author": "tester"}
	obj := mustPutObject(t, svc, "meta", "file.txt", []byte("data"))
	_ = obj

	obj2, err := svc.PutObject("meta", "file2.txt", []byte("data2"), meta)
	if err != nil {
		t.Fatal(err)
	}
	if obj2.Metadata["author"] != "tester" {
		t.Error("metadata was not preserved")
	}
}

// ---------------------------------------------------------------------------
// GetObject tests
// ---------------------------------------------------------------------------

func TestGetObject(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "read")

	content := []byte("important data")
	mustPutObject(t, svc, "read", "doc.txt", content)

	tests := []struct {
		name      string
		bucket    string
		key       string
		wantErr   bool
		errSubstr string
	}{
		{
			name:   "existing object",
			bucket: "read",
			key:    "doc.txt",
		},
		{
			name:      "missing bucket",
			bucket:    "nope",
			key:       "doc.txt",
			wantErr:   true,
			errSubstr: "bucket not found",
		},
		{
			name:      "missing key",
			bucket:    "read",
			key:       "nope.txt",
			wantErr:   true,
			errSubstr: "object not found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, obj, err := svc.GetObject(tc.bucket, tc.key)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !bytes.Equal(data, content) {
				t.Errorf("data = %q, want %q", data, content)
			}
			if obj.Key != tc.key {
				t.Errorf("Key = %q, want %q", obj.Key, tc.key)
			}
		})
	}
}

func TestGetObject_ReturnsCorrectChecksum(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "cs")

	content := []byte("checksum me")
	mustPutObject(t, svc, "cs", "file.bin", content)

	_, obj, err := svc.GetObject("cs", "file.bin")
	if err != nil {
		t.Fatal(err)
	}
	if obj.Checksum != sha256Hex(content) {
		t.Errorf("checksum mismatch: got %q, want %q", obj.Checksum, sha256Hex(content))
	}
}

// ---------------------------------------------------------------------------
// DeleteObject tests
// ---------------------------------------------------------------------------

func TestDeleteObject(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*Service)
		bucket    string
		key       string
		wantErr   bool
		errSubstr string
	}{
		{
			name: "delete existing",
			setup: func(svc *Service) {
				mustCreateBucket(t, svc, "del")
				mustPutObject(t, svc, "del", "file.txt", []byte("data"))
			},
			bucket: "del",
			key:    "file.txt",
		},
		{
			name:      "missing bucket",
			setup:     func(svc *Service) {},
			bucket:    "nope",
			key:       "file.txt",
			wantErr:   true,
			errSubstr: "bucket not found",
		},
		{
			name: "missing key",
			setup: func(svc *Service) {
				mustCreateBucket(t, svc, "del2")
			},
			bucket:    "del2",
			key:       "nope.txt",
			wantErr:   true,
			errSubstr: "object not found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			tc.setup(svc)

			err := svc.DeleteObject(tc.bucket, tc.key)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify object is gone.
			_, _, err = svc.GetObject(tc.bucket, tc.key)
			if err == nil {
				t.Error("expected error when getting deleted object")
			}
		})
	}
}

func TestDeleteObject_UpdatesBucketStats(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "dstats")
	mustPutObject(t, svc, "dstats", "a.txt", []byte("hello"))
	mustPutObject(t, svc, "dstats", "b.txt", []byte("world"))

	if err := svc.DeleteObject("dstats", "a.txt"); err != nil {
		t.Fatal(err)
	}

	b, _ := svc.GetBucket("dstats")
	if b.ObjectCount != 1 {
		t.Errorf("ObjectCount after delete = %d, want 1", b.ObjectCount)
	}
	if b.TotalSize != int64(len("world")) {
		t.Errorf("TotalSize after delete = %d, want %d", b.TotalSize, len("world"))
	}
}

func TestDeleteObject_DataRemovedFromStore(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "clean")
	mustPutObject(t, svc, "clean", "secret.dat", []byte("sensitive"))

	if err := svc.DeleteObject("clean", "secret.dat"); err != nil {
		t.Fatal(err)
	}

	// Verify the data bytes are removed from in-memory storage.
	svc.mu.RLock()
	_, exists := svc.data["clean/secret.dat"]
	svc.mu.RUnlock()
	if exists {
		t.Error("data bytes still in store after delete")
	}
}

// ---------------------------------------------------------------------------
// ListObjects tests
// ---------------------------------------------------------------------------

func TestListObjects(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "list")
	mustPutObject(t, svc, "list", "a.txt", []byte("a"))
	mustPutObject(t, svc, "list", "b.txt", []byte("b"))
	mustPutObject(t, svc, "list", "dir/c.txt", []byte("c"))
	mustPutObject(t, svc, "list", "dir/d.txt", []byte("d"))
	mustPutObject(t, svc, "list", "other/e.txt", []byte("e"))

	tests := []struct {
		name     string
		bucket   string
		prefix   string
		limit    int
		wantMin  int // minimum expected count (map iteration order is non-deterministic)
		wantMax  int // maximum expected count
		wantErr  bool
	}{
		{
			name:    "all objects no prefix",
			bucket:  "list",
			prefix:  "",
			limit:   0,
			wantMin: 5,
			wantMax: 5,
		},
		{
			name:    "prefix dir/",
			bucket:  "list",
			prefix:  "dir/",
			limit:   0,
			wantMin: 2,
			wantMax: 2,
		},
		{
			name:    "prefix other/",
			bucket:  "list",
			prefix:  "other/",
			limit:   0,
			wantMin: 1,
			wantMax: 1,
		},
		{
			name:    "with limit",
			bucket:  "list",
			prefix:  "",
			limit:   2,
			wantMin: 2,
			wantMax: 2,
		},
		{
			name:    "nonexistent prefix",
			bucket:  "list",
			prefix:  "zzz/",
			limit:   0,
			wantMin: 0,
			wantMax: 0,
		},
		{
			name:    "missing bucket",
			bucket:  "nope",
			prefix:  "",
			limit:   0,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			objs, err := svc.ListObjects(tc.bucket, tc.prefix, tc.limit)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(objs) < tc.wantMin || len(objs) > tc.wantMax {
				t.Errorf("got %d objects, want between %d and %d", len(objs), tc.wantMin, tc.wantMax)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CopyObject tests
// ---------------------------------------------------------------------------

func TestCopyObject(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "src")
	mustCreateBucket(t, svc, "dst")

	content := []byte("copy me")
	meta := map[string]string{"origin": "src"}
	if _, err := svc.PutObject("src", "original.txt", content, meta); err != nil {
		t.Fatal(err)
	}

	t.Run("same bucket", func(t *testing.T) {
		obj, err := svc.CopyObject("src", "original.txt", "src", "copy.txt")
		if err != nil {
			t.Fatal(err)
		}
		if obj.Key != "copy.txt" {
			t.Errorf("Key = %q, want %q", obj.Key, "copy.txt")
		}

		data, _, err := svc.GetObject("src", "copy.txt")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, content) {
			t.Error("copied data does not match original")
		}
	})

	t.Run("cross bucket", func(t *testing.T) {
		obj, err := svc.CopyObject("src", "original.txt", "dst", "migrated.txt")
		if err != nil {
			t.Fatal(err)
		}
		if obj.Bucket != "dst" {
			t.Errorf("Bucket = %q, want %q", obj.Bucket, "dst")
		}

		data, _, err := svc.GetObject("dst", "migrated.txt")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, content) {
			t.Error("cross-bucket copy data mismatch")
		}
	})

	t.Run("metadata preserved", func(t *testing.T) {
		_, err := svc.CopyObject("src", "original.txt", "dst", "meta-copy.txt")
		if err != nil {
			t.Fatal(err)
		}
		_, obj, err := svc.GetObject("dst", "meta-copy.txt")
		if err != nil {
			t.Fatal(err)
		}
		if obj.Metadata["origin"] != "src" {
			t.Error("metadata not preserved in copy")
		}
	})

	t.Run("source not found", func(t *testing.T) {
		_, err := svc.CopyObject("src", "nope.txt", "dst", "fail.txt")
		if err == nil {
			t.Fatal("expected error when source does not exist")
		}
	})

	t.Run("destination bucket missing", func(t *testing.T) {
		_, err := svc.CopyObject("src", "original.txt", "no-bucket", "fail.txt")
		if err == nil {
			t.Fatal("expected error when destination bucket missing")
		}
	})
}

// ---------------------------------------------------------------------------
// StreamObject tests
// ---------------------------------------------------------------------------

func TestStreamObject(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "stream")

	content := []byte("streaming data from the kingdom")
	mustPutObject(t, svc, "stream", "file.dat", content)

	t.Run("successful stream", func(t *testing.T) {
		var buf bytes.Buffer
		err := svc.StreamObject("stream", "file.dat", &buf)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(buf.Bytes(), content) {
			t.Errorf("streamed data = %q, want %q", buf.String(), content)
		}
	})

	t.Run("bucket not found", func(t *testing.T) {
		var buf bytes.Buffer
		err := svc.StreamObject("nope", "file.dat", &buf)
		if err == nil {
			t.Fatal("expected error for missing bucket")
		}
	})

	t.Run("key not found", func(t *testing.T) {
		var buf bytes.Buffer
		err := svc.StreamObject("stream", "nope.dat", &buf)
		if err == nil {
			t.Fatal("expected error for missing key")
		}
	})

	t.Run("writer error", func(t *testing.T) {
		w := &errWriter{err: fmt.Errorf("disk full")}
		err := svc.StreamObject("stream", "file.dat", w)
		if err == nil {
			t.Fatal("expected error from failing writer")
		}
		if !strings.Contains(err.Error(), "disk full") {
			t.Errorf("error %q should mention disk full", err.Error())
		}
	})
}

// ---------------------------------------------------------------------------
// Backup tests
// ---------------------------------------------------------------------------

func TestCreateBackup(t *testing.T) {
	tests := []struct {
		name   string
		backup *Backup
	}{
		{
			name: "full backup with ID",
			backup: &Backup{
				ID:     "backup-001",
				Name:   "nightly",
				Source: "prod-data",
				Type:   BackupFull,
			},
		},
		{
			name: "incremental backup auto-ID",
			backup: &Backup{
				Name:   "hourly",
				Source: "hot-data",
				Type:   BackupIncremental,
			},
		},
		{
			name: "differential backup",
			backup: &Backup{
				Name:   "weekly",
				Source: "cold-data",
				Type:   BackupDifferential,
			},
		},
		{
			name: "with custom retention",
			backup: &Backup{
				Name:          "long-term",
				Source:        "archive",
				RetentionDays: 365,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			origID := tc.backup.ID

			err := svc.CreateBackup(tc.backup)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Auto-ID should have been assigned if empty.
			if origID == "" && tc.backup.ID == "" {
				t.Error("expected auto-generated ID")
			}

			// Status should be pending.
			if tc.backup.Status != BackupPending {
				t.Errorf("Status = %q, want %q", tc.backup.Status, BackupPending)
			}

			// Default retention should be applied if zero.
			if tc.backup.RetentionDays == 0 {
				t.Error("RetentionDays should not be zero (default should apply)")
			}

			// Should be retrievable.
			b, ok := svc.GetBackup(tc.backup.ID)
			if !ok {
				t.Fatal("backup not found after creation")
			}
			if b.Name != tc.backup.Name {
				t.Errorf("Name = %q, want %q", b.Name, tc.backup.Name)
			}
		})
	}
}

func TestGetBackup_NotFound(t *testing.T) {
	svc := newTestService(t)
	_, ok := svc.GetBackup("nonexistent")
	if ok {
		t.Error("expected backup not to be found")
	}
}

func TestCreateBackup_DefaultRetention(t *testing.T) {
	cfg := &Config{DefaultRetention: 90}
	svc := newTestServiceWithConfig(t, cfg)

	b := &Backup{ID: "ret-test", Name: "test"}
	if err := svc.CreateBackup(b); err != nil {
		t.Fatal(err)
	}
	if b.RetentionDays != 90 {
		t.Errorf("RetentionDays = %d, want 90 (from config)", b.RetentionDays)
	}
}

func TestCreateBackup_CustomRetentionNotOverridden(t *testing.T) {
	cfg := &Config{DefaultRetention: 90}
	svc := newTestServiceWithConfig(t, cfg)

	b := &Backup{ID: "ret-custom", Name: "test", RetentionDays: 7}
	if err := svc.CreateBackup(b); err != nil {
		t.Fatal(err)
	}
	if b.RetentionDays != 7 {
		t.Errorf("RetentionDays = %d, want 7 (custom value should be kept)", b.RetentionDays)
	}
}

func TestBackupTypes(t *testing.T) {
	// Verify the constants are distinct.
	types := []BackupType{BackupFull, BackupIncremental, BackupDifferential}
	seen := make(map[BackupType]bool)
	for _, bt := range types {
		if seen[bt] {
			t.Errorf("duplicate BackupType: %q", bt)
		}
		seen[bt] = true
	}
}

func TestBackupStatuses(t *testing.T) {
	statuses := []BackupStatus{BackupPending, BackupRunning, BackupCompleted, BackupFailed}
	seen := make(map[BackupStatus]bool)
	for _, s := range statuses {
		if seen[s] {
			t.Errorf("duplicate BackupStatus: %q", s)
		}
		seen[s] = true
	}
}

// ---------------------------------------------------------------------------
// Pipeline tests
// ---------------------------------------------------------------------------

func TestCreatePipeline(t *testing.T) {
	tests := []struct {
		name     string
		pipeline *Pipeline
	}{
		{
			name: "basic pipeline",
			pipeline: &Pipeline{
				Name: "etl",
				Source: DataSource{
					Type:       "s3",
					Connection: "s3://bucket/input",
				},
				Destination: DataSource{
					Type:       "db",
					Connection: "postgres://localhost/data",
				},
			},
		},
		{
			name: "pipeline with transforms",
			pipeline: &Pipeline{
				Name: "transform-pipe",
				Source: DataSource{
					Type:       "local",
					Connection: "/data/input",
				},
				Destination: DataSource{
					Type:       "s3",
					Connection: "s3://out",
				},
				Transforms: []Transform{
					{Name: "filter", Type: "filter", Config: map[string]string{"field": "status", "value": "active"}},
					{Name: "aggregate", Type: "aggregate", Config: map[string]string{"group_by": "region"}},
				},
			},
		},
		{
			name: "pipeline with schedule",
			pipeline: &Pipeline{
				ID:       "pipe-sched",
				Name:     "scheduled",
				Schedule: "0 * * * *",
				Source:   DataSource{Type: "gcs", Connection: "gs://bucket"},
				Destination: DataSource{Type: "local", Connection: "/output"},
			},
		},
		{
			name: "pipeline with config maps",
			pipeline: &Pipeline{
				Name: "configured",
				Source: DataSource{
					Type:       "db",
					Connection: "mysql://host/db",
					Config:     map[string]string{"charset": "utf8", "batch_size": "1000"},
				},
				Destination: DataSource{
					Type:       "s3",
					Connection: "s3://dest",
					Config:     map[string]string{"compression": "gzip"},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			origID := tc.pipeline.ID

			err := svc.CreatePipeline(tc.pipeline)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Auto-ID should have been assigned if empty.
			if origID == "" && tc.pipeline.ID == "" {
				t.Error("expected auto-generated ID")
			}

			// Status should be active.
			if tc.pipeline.Status != PipelineActive {
				t.Errorf("Status = %q, want %q", tc.pipeline.Status, PipelineActive)
			}

			// Should be retrievable.
			p, ok := svc.GetPipeline(tc.pipeline.ID)
			if !ok {
				t.Fatal("pipeline not found after creation")
			}
			if p.Name != tc.pipeline.Name {
				t.Errorf("Name = %q, want %q", p.Name, tc.pipeline.Name)
			}
		})
	}
}

func TestGetPipeline_NotFound(t *testing.T) {
	svc := newTestService(t)
	_, ok := svc.GetPipeline("nonexistent")
	if ok {
		t.Error("expected pipeline not to be found")
	}
}

func TestPipelineStatuses(t *testing.T) {
	statuses := []PipelineStatus{PipelineActive, PipelinePaused, PipelineDisabled}
	seen := make(map[PipelineStatus]bool)
	for _, s := range statuses {
		if seen[s] {
			t.Errorf("duplicate PipelineStatus: %q", s)
		}
		seen[s] = true
	}
}

// ---------------------------------------------------------------------------
// Stats tests
// ---------------------------------------------------------------------------

func TestStats_Empty(t *testing.T) {
	svc := newTestService(t)
	stats := svc.Stats()

	assertStatValue(t, stats, "total_buckets", 0)
	assertStatValue(t, stats, "total_objects", 0)
	assertStatValue(t, stats, "total_backups", 0)
	assertStatValue(t, stats, "total_pipelines", 0)

	if size, ok := stats["total_size"].(int64); !ok || size != 0 {
		t.Errorf("total_size = %v, want 0", stats["total_size"])
	}
}

func TestStats_Populated(t *testing.T) {
	svc := newTestService(t)

	// Buckets + objects.
	mustCreateBucket(t, svc, "b1")
	mustCreateBucket(t, svc, "b2")
	mustPutObject(t, svc, "b1", "file1.txt", []byte("hello"))
	mustPutObject(t, svc, "b1", "file2.txt", []byte("world"))
	mustPutObject(t, svc, "b2", "file3.txt", []byte("!"))

	// Backups.
	_ = svc.CreateBackup(&Backup{ID: "bk1", Name: "bk1"})
	_ = svc.CreateBackup(&Backup{ID: "bk2", Name: "bk2"})

	// Pipelines.
	_ = svc.CreatePipeline(&Pipeline{ID: "p1", Name: "p1"})

	stats := svc.Stats()

	assertStatValue(t, stats, "total_buckets", 2)
	assertStatValue(t, stats, "total_objects", 3)
	assertStatValue(t, stats, "total_backups", 2)
	assertStatValue(t, stats, "total_pipelines", 1)

	totalSize := stats["total_size"].(int64)
	expectedSize := int64(len("hello") + len("world") + len("!"))
	if totalSize != expectedSize {
		t.Errorf("total_size = %d, want %d", totalSize, expectedSize)
	}
}

func assertStatValue(t *testing.T, stats map[string]interface{}, key string, want int) {
	t.Helper()
	got, ok := stats[key].(int)
	if !ok {
		t.Errorf("stats[%q] has unexpected type %T", key, stats[key])
		return
	}
	if got != want {
		t.Errorf("stats[%q] = %d, want %d", key, got, want)
	}
}

// ---------------------------------------------------------------------------
// Storage class tests
// ---------------------------------------------------------------------------

func TestStorageClasses(t *testing.T) {
	classes := []struct {
		sc   StorageClass
		want string
	}{
		{StorageStandard, "standard"},
		{StorageInfrequent, "infrequent"},
		{StorageArchive, "archive"},
		{StorageGlacier, "glacier"},
	}

	seen := make(map[StorageClass]bool)
	for _, tc := range classes {
		t.Run(string(tc.sc), func(t *testing.T) {
			if string(tc.sc) != tc.want {
				t.Errorf("StorageClass = %q, want %q", tc.sc, tc.want)
			}
			if seen[tc.sc] {
				t.Errorf("duplicate storage class: %q", tc.sc)
			}
			seen[tc.sc] = true
		})
	}
}

func TestCreateBucket_StorageClassDefault(t *testing.T) {
	svc := newTestServiceWithConfig(t, &Config{DefaultStorageClass: StorageGlacier})
	b := &Bucket{Name: "glacial"}
	if err := svc.CreateBucket(b); err != nil {
		t.Fatal(err)
	}
	if b.StorageClass != StorageGlacier {
		t.Errorf("StorageClass = %q, want %q", b.StorageClass, StorageGlacier)
	}
}

func TestCreateBucket_StorageClassExplicit(t *testing.T) {
	svc := newTestServiceWithConfig(t, &Config{DefaultStorageClass: StorageStandard})
	b := &Bucket{Name: "archive-explicit", StorageClass: StorageArchive}
	if err := svc.CreateBucket(b); err != nil {
		t.Fatal(err)
	}
	if b.StorageClass != StorageArchive {
		t.Errorf("StorageClass = %q, want %q (explicit should not be overridden)", b.StorageClass, StorageArchive)
	}
}

// ---------------------------------------------------------------------------
// Encryption at rest tests
// ---------------------------------------------------------------------------

func TestEncryptionAtRest_BucketFlag(t *testing.T) {
	t.Run("encryption enabled by default", func(t *testing.T) {
		svc := newTestService(t) // Default config has EnableEncryption: true
		b := &Bucket{Name: "secure"}
		if err := svc.CreateBucket(b); err != nil {
			t.Fatal(err)
		}
		if !b.Encryption {
			t.Error("bucket should have encryption enabled when config enables it")
		}
	})

	t.Run("encryption disabled in config", func(t *testing.T) {
		svc := newTestServiceWithConfig(t, &Config{EnableEncryption: false})
		b := &Bucket{Name: "open"}
		if err := svc.CreateBucket(b); err != nil {
			t.Fatal(err)
		}
		if b.Encryption {
			t.Error("bucket should not have encryption when config disables it")
		}
	})

	t.Run("all buckets inherit encryption setting", func(t *testing.T) {
		svc := newTestServiceWithConfig(t, &Config{EnableEncryption: true})
		for i := 0; i < 5; i++ {
			b := &Bucket{Name: fmt.Sprintf("enc-%d", i)}
			if err := svc.CreateBucket(b); err != nil {
				t.Fatal(err)
			}
			if !b.Encryption {
				t.Errorf("bucket %q missing encryption flag", b.Name)
			}
		}
	})
}

func TestEncryptionAtRest_DataIntegrity(t *testing.T) {
	// Verify that data stored and retrieved is identical regardless of encryption flag.
	for _, encEnabled := range []bool{true, false} {
		name := "encrypted"
		if !encEnabled {
			name = "unencrypted"
		}
		t.Run(name, func(t *testing.T) {
			svc := newTestServiceWithConfig(t, &Config{EnableEncryption: encEnabled, MaxObjectSize: 1 << 20})
			mustCreateBucket(t, svc, "data")

			content := []byte("sensitive financial data 1234-5678")
			mustPutObject(t, svc, "data", "pii.dat", content)

			retrieved, obj, err := svc.GetObject("data", "pii.dat")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(retrieved, content) {
				t.Error("data integrity violated")
			}
			if obj.Checksum != sha256Hex(content) {
				t.Error("checksum integrity violated")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Backup / Restore round-trip tests
// ---------------------------------------------------------------------------

func TestBackupRestore_RoundTrip(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "prod")

	// Populate production data.
	files := map[string][]byte{
		"config.yaml":   []byte("port: 8080\nenv: prod"),
		"data.json":     []byte(`{"users": 42}`),
		"readme.md":     []byte("# Production Data"),
		"bin/app":       []byte{0x7f, 0x45, 0x4c, 0x46}, // ELF header bytes
	}
	for key, data := range files {
		mustPutObject(t, svc, "prod", key, data)
	}

	// Create backup record.
	backup := &Backup{
		ID:          "backup-roundtrip",
		Name:        "production-snapshot",
		Source:      "prod",
		Destination: "backup-store",
		Type:        BackupFull,
	}
	if err := svc.CreateBackup(backup); err != nil {
		t.Fatal(err)
	}

	// Simulate restore: copy all objects to a new bucket.
	mustCreateBucket(t, svc, "restored")
	for key := range files {
		_, err := svc.CopyObject("prod", key, "restored", key)
		if err != nil {
			t.Fatalf("failed to copy %q during restore: %v", key, err)
		}
	}

	// Verify restored data matches original.
	for key, original := range files {
		data, _, err := svc.GetObject("restored", key)
		if err != nil {
			t.Fatalf("restored object %q not found: %v", key, err)
		}
		if !bytes.Equal(data, original) {
			t.Errorf("restored data for %q does not match original", key)
		}
	}

	// Verify backup metadata.
	b, ok := svc.GetBackup("backup-roundtrip")
	if !ok {
		t.Fatal("backup record not found")
	}
	if b.Status != BackupPending {
		t.Errorf("backup status = %q, want %q", b.Status, BackupPending)
	}
	if b.Type != BackupFull {
		t.Errorf("backup type = %q, want %q", b.Type, BackupFull)
	}
}

// ---------------------------------------------------------------------------
// Data pipeline operations tests
// ---------------------------------------------------------------------------

func TestDataPipeline_FullLifecycle(t *testing.T) {
	svc := newTestService(t)

	// Create source and destination buckets.
	mustCreateBucket(t, svc, "raw-input")
	mustCreateBucket(t, svc, "processed-output")

	// Populate source.
	mustPutObject(t, svc, "raw-input", "events/2026-01-01.json", []byte(`[{"event": "click"}]`))
	mustPutObject(t, svc, "raw-input", "events/2026-01-02.json", []byte(`[{"event": "purchase"}]`))

	// Create pipeline.
	pipe := &Pipeline{
		Name: "event-processor",
		Source: DataSource{
			Type:       "s3",
			Connection: "raw-input",
			Config:     map[string]string{"format": "json"},
		},
		Destination: DataSource{
			Type:       "s3",
			Connection: "processed-output",
			Config:     map[string]string{"format": "parquet"},
		},
		Transforms: []Transform{
			{Name: "parse-json", Type: "map"},
			{Name: "filter-clicks", Type: "filter", Config: map[string]string{"event": "click"}},
			{Name: "count-by-day", Type: "aggregate", Config: map[string]string{"group_by": "date"}},
		},
		Schedule: "0 */6 * * *",
	}
	if err := svc.CreatePipeline(pipe); err != nil {
		t.Fatal(err)
	}

	// Verify pipeline was stored.
	retrieved, ok := svc.GetPipeline(pipe.ID)
	if !ok {
		t.Fatal("pipeline not found")
	}
	if retrieved.Status != PipelineActive {
		t.Errorf("pipeline status = %q, want %q", retrieved.Status, PipelineActive)
	}
	if len(retrieved.Transforms) != 3 {
		t.Errorf("transforms count = %d, want 3", len(retrieved.Transforms))
	}
	if retrieved.Schedule != "0 */6 * * *" {
		t.Errorf("schedule = %q, want %q", retrieved.Schedule, "0 */6 * * *")
	}

	// Simulate pipeline execution: copy from source to destination.
	srcObjects, err := svc.ListObjects("raw-input", "events/", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, obj := range srcObjects {
		_, err := svc.CopyObject("raw-input", obj.Key, "processed-output", "output/"+obj.Key)
		if err != nil {
			t.Fatalf("pipeline copy %q failed: %v", obj.Key, err)
		}
	}

	// Verify output.
	outputObjects, err := svc.ListObjects("processed-output", "output/", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(outputObjects) != len(srcObjects) {
		t.Errorf("output count = %d, want %d", len(outputObjects), len(srcObjects))
	}
}

func TestDataPipeline_MultipleTransforms(t *testing.T) {
	svc := newTestService(t)

	transforms := []Transform{
		{Name: "step1", Type: "filter"},
		{Name: "step2", Type: "map"},
		{Name: "step3", Type: "aggregate"},
		{Name: "step4", Type: "filter"},
		{Name: "step5", Type: "map"},
	}

	pipe := &Pipeline{
		Name:        "multi-transform",
		Source:      DataSource{Type: "db", Connection: "postgres://localhost/src"},
		Destination: DataSource{Type: "s3", Connection: "s3://output"},
		Transforms:  transforms,
	}

	if err := svc.CreatePipeline(pipe); err != nil {
		t.Fatal(err)
	}

	p, ok := svc.GetPipeline(pipe.ID)
	if !ok {
		t.Fatal("pipeline not found")
	}
	if len(p.Transforms) != 5 {
		t.Errorf("transforms = %d, want 5", len(p.Transforms))
	}
	for i, tr := range p.Transforms {
		expected := fmt.Sprintf("step%d", i+1)
		if tr.Name != expected {
			t.Errorf("transform[%d].Name = %q, want %q", i, tr.Name, expected)
		}
	}
}

// ---------------------------------------------------------------------------
// Storage abstraction tests
// ---------------------------------------------------------------------------

func TestStorageAbstraction_MultipleBuckets(t *testing.T) {
	svc := newTestService(t)

	bucketNames := []string{"users", "logs", "config", "cache", "temp"}
	for _, name := range bucketNames {
		mustCreateBucket(t, svc, name)
	}

	// Each bucket gets its own data.
	for _, name := range bucketNames {
		data := []byte("data for " + name)
		mustPutObject(t, svc, name, "default.dat", data)
	}

	// Verify isolation: each bucket has its own independent object.
	for _, name := range bucketNames {
		data, _, err := svc.GetObject(name, "default.dat")
		if err != nil {
			t.Fatalf("failed to get object from bucket %q: %v", name, err)
		}
		expected := []byte("data for " + name)
		if !bytes.Equal(data, expected) {
			t.Errorf("bucket %q returned %q, want %q", name, data, expected)
		}
	}
}

func TestStorageAbstraction_BucketIsolation(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "alpha")
	mustCreateBucket(t, svc, "beta")

	mustPutObject(t, svc, "alpha", "shared-key.txt", []byte("from alpha"))
	mustPutObject(t, svc, "beta", "shared-key.txt", []byte("from beta"))

	dataA, _, _ := svc.GetObject("alpha", "shared-key.txt")
	dataB, _, _ := svc.GetObject("beta", "shared-key.txt")

	if bytes.Equal(dataA, dataB) {
		t.Error("objects with same key in different buckets should have different data")
	}
	if string(dataA) != "from alpha" {
		t.Errorf("alpha data = %q, want %q", dataA, "from alpha")
	}
	if string(dataB) != "from beta" {
		t.Errorf("beta data = %q, want %q", dataB, "from beta")
	}
}

func TestStorageAbstraction_ObjectOverwrite(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "overwrite")

	mustPutObject(t, svc, "overwrite", "file.txt", []byte("version 1"))
	mustPutObject(t, svc, "overwrite", "file.txt", []byte("version 2"))

	data, _, err := svc.GetObject("overwrite", "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "version 2" {
		t.Errorf("overwritten data = %q, want %q", data, "version 2")
	}
}

// ---------------------------------------------------------------------------
// Edge cases and error paths
// ---------------------------------------------------------------------------

func TestEdgeCase_EmptyBucketName(t *testing.T) {
	svc := newTestService(t)
	err := svc.CreateBucket(&Bucket{Name: ""})
	if err == nil {
		t.Fatal("expected error for empty bucket name")
	}
}

func TestEdgeCase_ZeroSizeObject(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "zero")

	obj, err := svc.PutObject("zero", "empty.bin", []byte{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if obj.Size != 0 {
		t.Errorf("Size = %d, want 0", obj.Size)
	}
	if obj.Checksum != sha256Hex([]byte{}) {
		t.Error("checksum mismatch for empty data")
	}
}

func TestEdgeCase_LargeKey(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "keys")

	longKey := strings.Repeat("a/", 500) + "file.txt"
	obj := mustPutObject(t, svc, "keys", longKey, []byte("data"))
	if obj.Key != longKey {
		t.Error("long key was not preserved")
	}

	data, _, err := svc.GetObject("keys", longKey)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "data" {
		t.Error("data mismatch for long key")
	}
}

func TestEdgeCase_SpecialCharactersInKey(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "special")

	keys := []string{
		"file with spaces.txt",
		"file\twith\ttabs",
		"file-with-dashes",
		"file_with_underscores",
		"file.multiple.dots.txt",
		"unicode-\u00e9\u00e0\u00fc.txt",
		"path/to/deeply/nested/file.dat",
	}

	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			mustPutObject(t, svc, "special", key, []byte("content"))
			data, _, err := svc.GetObject("special", key)
			if err != nil {
				t.Fatalf("failed to get object with key %q: %v", key, err)
			}
			if string(data) != "content" {
				t.Errorf("data mismatch for key %q", key)
			}
		})
	}
}

func TestEdgeCase_BinaryData(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "bin")

	// Include null bytes and non-UTF8 sequences.
	data := []byte{0x00, 0x01, 0xff, 0xfe, 0x80, 0x7f, 0x00, 0x00}
	mustPutObject(t, svc, "bin", "binary.dat", data)

	retrieved, _, err := svc.GetObject("bin", "binary.dat")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(retrieved, data) {
		t.Error("binary data round-trip failed")
	}
}

func TestEdgeCase_NilMetadata(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "nilmeta")

	obj, err := svc.PutObject("nilmeta", "file.txt", []byte("data"), nil)
	if err != nil {
		t.Fatal(err)
	}
	// nil metadata should be accepted.
	if obj.Metadata != nil {
		t.Errorf("expected nil metadata, got %v", obj.Metadata)
	}
}

func TestEdgeCase_MultipleDeleteSameObject(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "multidel")
	mustPutObject(t, svc, "multidel", "file.txt", []byte("data"))

	// First delete succeeds.
	if err := svc.DeleteObject("multidel", "file.txt"); err != nil {
		t.Fatal(err)
	}

	// Second delete should fail (object already gone).
	err := svc.DeleteObject("multidel", "file.txt")
	if err == nil {
		t.Fatal("expected error deleting already-deleted object")
	}
}

func TestEdgeCase_PutAfterDelete(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "reput")
	mustPutObject(t, svc, "reput", "file.txt", []byte("original"))
	if err := svc.DeleteObject("reput", "file.txt"); err != nil {
		t.Fatal(err)
	}

	// Re-create should succeed.
	mustPutObject(t, svc, "reput", "file.txt", []byte("recreated"))
	data, _, err := svc.GetObject("reput", "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "recreated" {
		t.Errorf("data after re-put = %q, want %q", data, "recreated")
	}
}

func TestEdgeCase_ListObjectsWithLimitZero(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "limzero")
	mustPutObject(t, svc, "limzero", "a.txt", []byte("a"))
	mustPutObject(t, svc, "limzero", "b.txt", []byte("b"))

	// limit=0 means no limit, should return all.
	objs, err := svc.ListObjects("limzero", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 2 {
		t.Errorf("expected 2 objects with limit=0, got %d", len(objs))
	}
}

func TestEdgeCase_ListObjectsWithLimitOne(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "limone")
	mustPutObject(t, svc, "limone", "a.txt", []byte("a"))
	mustPutObject(t, svc, "limone", "b.txt", []byte("b"))
	mustPutObject(t, svc, "limone", "c.txt", []byte("c"))

	objs, err := svc.ListObjects("limone", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 1 {
		t.Errorf("expected 1 object with limit=1, got %d", len(objs))
	}
}

// ---------------------------------------------------------------------------
// Concurrent / race-safe tests
// ---------------------------------------------------------------------------

func TestConcurrent_BucketCreation(t *testing.T) {
	svc := newTestService(t)

	var wg sync.WaitGroup
	errs := make([]error, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = svc.CreateBucket(&Bucket{Name: fmt.Sprintf("bucket-%d", idx)})
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("bucket-%d creation failed: %v", i, err)
		}
	}

	buckets := svc.ListBuckets()
	if len(buckets) != 100 {
		t.Errorf("expected 100 buckets, got %d", len(buckets))
	}
}

func TestConcurrent_PutAndGet(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "concurrent")

	const numWorkers = 50
	var wg sync.WaitGroup

	// Concurrent puts.
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("file-%d.txt", idx)
			data := []byte(fmt.Sprintf("data-%d", idx))
			_, err := svc.PutObject("concurrent", key, data, nil)
			if err != nil {
				t.Errorf("PutObject(%q): %v", key, err)
			}
		}(i)
	}
	wg.Wait()

	// Concurrent gets.
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("file-%d.txt", idx)
			data, _, err := svc.GetObject("concurrent", key)
			if err != nil {
				t.Errorf("GetObject(%q): %v", key, err)
				return
			}
			expected := fmt.Sprintf("data-%d", idx)
			if string(data) != expected {
				t.Errorf("data for %q = %q, want %q", key, data, expected)
			}
		}(i)
	}
	wg.Wait()
}

func TestConcurrent_MixedOperations(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "mixed")

	// Pre-populate some objects.
	for i := 0; i < 20; i++ {
		mustPutObject(t, svc, "mixed", fmt.Sprintf("file-%d.txt", i), []byte(fmt.Sprintf("data-%d", i)))
	}

	var wg sync.WaitGroup
	const workers = 30

	// Mix of reads, writes, deletes, lists, stats.
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			switch idx % 5 {
			case 0: // Write
				key := fmt.Sprintf("new-file-%d.txt", idx)
				_, _ = svc.PutObject("mixed", key, []byte("new"), nil)
			case 1: // Read
				key := fmt.Sprintf("file-%d.txt", idx%20)
				_, _, _ = svc.GetObject("mixed", key)
			case 2: // List
				_, _ = svc.ListObjects("mixed", "", 10)
			case 3: // Stats
				_ = svc.Stats()
			case 4: // GetBucket
				_, _ = svc.GetBucket("mixed")
			}
		}(i)
	}
	wg.Wait()
}

func TestConcurrent_DeleteWhileReading(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "delread")
	mustPutObject(t, svc, "delread", "target.txt", []byte("ephemeral"))

	var wg sync.WaitGroup
	const readers = 20

	// Start readers.
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Just exercise the read path; errors are acceptable due to concurrent delete.
			_, _, _ = svc.GetObject("delread", "target.txt")
		}()
	}

	// Delete in the middle.
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = svc.DeleteObject("delread", "target.txt")
	}()

	wg.Wait()
}

func TestConcurrent_MultipleBackupCreation(t *testing.T) {
	svc := newTestService(t)

	var wg sync.WaitGroup
	const numBackups = 50

	for i := 0; i < numBackups; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			b := &Backup{
				ID:   fmt.Sprintf("backup-concurrent-%d", idx),
				Name: fmt.Sprintf("backup-%d", idx),
			}
			if err := svc.CreateBackup(b); err != nil {
				t.Errorf("CreateBackup(%d): %v", idx, err)
			}
		}(i)
	}
	wg.Wait()

	stats := svc.Stats()
	if stats["total_backups"].(int) != numBackups {
		t.Errorf("total_backups = %d, want %d", stats["total_backups"], numBackups)
	}
}

func TestConcurrent_MultiplePipelineCreation(t *testing.T) {
	svc := newTestService(t)

	var wg sync.WaitGroup
	const numPipelines = 50

	for i := 0; i < numPipelines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			p := &Pipeline{
				ID:   fmt.Sprintf("pipeline-concurrent-%d", idx),
				Name: fmt.Sprintf("pipeline-%d", idx),
			}
			if err := svc.CreatePipeline(p); err != nil {
				t.Errorf("CreatePipeline(%d): %v", idx, err)
			}
		}(i)
	}
	wg.Wait()

	stats := svc.Stats()
	if stats["total_pipelines"].(int) != numPipelines {
		t.Errorf("total_pipelines = %d, want %d", stats["total_pipelines"], numPipelines)
	}
}

func TestConcurrent_StatsUnderLoad(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "loadtest")

	var wg sync.WaitGroup

	// Writers.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _ = svc.PutObject("loadtest", fmt.Sprintf("obj-%d", idx), []byte("data"), nil)
		}(i)
	}

	// Stats readers.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stats := svc.Stats()
			// Should not panic; values may be intermediate but consistent.
			_ = stats["total_buckets"]
		}()
	}

	wg.Wait()
}

func TestConcurrent_CopyObject(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "src-copy")
	mustCreateBucket(t, svc, "dst-copy")
	mustPutObject(t, svc, "src-copy", "template.txt", []byte("template data"))

	var wg sync.WaitGroup
	const copies = 30

	for i := 0; i < copies; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			dstKey := fmt.Sprintf("copy-%d.txt", idx)
			_, err := svc.CopyObject("src-copy", "template.txt", "dst-copy", dstKey)
			if err != nil {
				t.Errorf("CopyObject(%d): %v", idx, err)
			}
		}(i)
	}
	wg.Wait()

	objs, err := svc.ListObjects("dst-copy", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != copies {
		t.Errorf("expected %d copied objects, got %d", copies, len(objs))
	}
}

// ---------------------------------------------------------------------------
// Object struct field tests
// ---------------------------------------------------------------------------

func TestObject_FieldPopulation(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "fields")

	meta := map[string]string{"type": "test", "env": "ci"}
	data := []byte("field test data")
	obj := mustPutObject(t, svc, "fields", "test.dat", data)

	// Put again with metadata.
	obj, err := svc.PutObject("fields", "meta.dat", data, meta)
	if err != nil {
		t.Fatal(err)
	}

	if obj.ID != "fields/meta.dat" {
		t.Errorf("ID = %q, want %q", obj.ID, "fields/meta.dat")
	}
	if obj.Bucket != "fields" {
		t.Errorf("Bucket = %q, want %q", obj.Bucket, "fields")
	}
	if obj.Key != "meta.dat" {
		t.Errorf("Key = %q, want %q", obj.Key, "meta.dat")
	}
	if obj.Size != int64(len(data)) {
		t.Errorf("Size = %d, want %d", obj.Size, len(data))
	}
	if obj.ContentType != "" {
		// ContentType is not set by PutObject; it defaults to zero value.
		t.Errorf("ContentType should be empty, got %q", obj.ContentType)
	}
	if obj.Metadata["type"] != "test" {
		t.Error("metadata 'type' missing")
	}
	if obj.Metadata["env"] != "ci" {
		t.Error("metadata 'env' missing")
	}
	if obj.Version != 1 {
		t.Errorf("Version = %d, want 1", obj.Version)
	}
	if time.Since(obj.CreatedAt) > 5*time.Second {
		t.Error("CreatedAt too old")
	}
	if time.Since(obj.UpdatedAt) > 5*time.Second {
		t.Error("UpdatedAt too old")
	}
}

// ---------------------------------------------------------------------------
// Bucket struct field tests
// ---------------------------------------------------------------------------

func TestBucket_Fields(t *testing.T) {
	svc := newTestService(t)
	b := &Bucket{
		Name:       "detailed",
		Region:     "us-west-2",
		Versioning: true,
		Tags:       map[string]string{"team": "platform"},
	}
	if err := svc.CreateBucket(b); err != nil {
		t.Fatal(err)
	}

	got, ok := svc.GetBucket("detailed")
	if !ok {
		t.Fatal("bucket not found")
	}
	if got.Region != "us-west-2" {
		t.Errorf("Region = %q, want %q", got.Region, "us-west-2")
	}
	if !got.Versioning {
		t.Error("Versioning should be true")
	}
	if got.Tags["team"] != "platform" {
		t.Error("tags not preserved")
	}
	if got.ObjectCount != 0 {
		t.Errorf("ObjectCount = %d, want 0 (new bucket)", got.ObjectCount)
	}
	if got.TotalSize != 0 {
		t.Errorf("TotalSize = %d, want 0 (new bucket)", got.TotalSize)
	}
}

// ---------------------------------------------------------------------------
// Backup struct tests
// ---------------------------------------------------------------------------

func TestBackup_Fields(t *testing.T) {
	svc := newTestService(t)
	now := time.Now()
	completed := now.Add(10 * time.Minute)

	b := &Backup{
		ID:            "bk-fields",
		Name:          "full-nightly",
		Source:        "production",
		Destination:   "dr-site",
		Type:          BackupFull,
		Size:          1024 * 1024,
		ObjectCount:   42,
		StartedAt:     &now,
		CompletedAt:   &completed,
		Error:         "",
		RetentionDays: 90,
	}
	if err := svc.CreateBackup(b); err != nil {
		t.Fatal(err)
	}

	got, ok := svc.GetBackup("bk-fields")
	if !ok {
		t.Fatal("backup not found")
	}
	if got.Source != "production" {
		t.Errorf("Source = %q", got.Source)
	}
	if got.Destination != "dr-site" {
		t.Errorf("Destination = %q", got.Destination)
	}
	if got.Size != 1024*1024 {
		t.Errorf("Size = %d", got.Size)
	}
	if got.ObjectCount != 42 {
		t.Errorf("ObjectCount = %d", got.ObjectCount)
	}
	if got.StartedAt == nil || got.StartedAt.IsZero() {
		t.Error("StartedAt should be set")
	}
	if got.CompletedAt == nil || got.CompletedAt.IsZero() {
		t.Error("CompletedAt should be set")
	}
	if got.RetentionDays != 90 {
		t.Errorf("RetentionDays = %d, want 90", got.RetentionDays)
	}
}

// ---------------------------------------------------------------------------
// Stream with large data test
// ---------------------------------------------------------------------------

func TestStreamObject_LargeData(t *testing.T) {
	cfg := &Config{MaxObjectSize: 10 * 1024 * 1024}
	svc := newTestServiceWithConfig(t, cfg)
	mustCreateBucket(t, svc, "large")

	// Create 1MB of data.
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	mustPutObject(t, svc, "large", "big.bin", data)

	var buf bytes.Buffer
	if err := svc.StreamObject("large", "big.bin", &buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Error("large data stream mismatch")
	}
}

// ---------------------------------------------------------------------------
// Multiple services independence test
// ---------------------------------------------------------------------------

func TestMultipleServices_Independent(t *testing.T) {
	svc1 := newTestService(t)
	svc2 := newTestService(t)

	mustCreateBucket(t, svc1, "shared-name")
	mustCreateBucket(t, svc2, "shared-name")

	mustPutObject(t, svc1, "shared-name", "key", []byte("service1"))
	mustPutObject(t, svc2, "shared-name", "key", []byte("service2"))

	data1, _, _ := svc1.GetObject("shared-name", "key")
	data2, _, _ := svc2.GetObject("shared-name", "key")

	if string(data1) != "service1" {
		t.Errorf("svc1 data = %q, want %q", data1, "service1")
	}
	if string(data2) != "service2" {
		t.Errorf("svc2 data = %q, want %q", data2, "service2")
	}
}

// ---------------------------------------------------------------------------
// Full CRUD lifecycle test
// ---------------------------------------------------------------------------

func TestFullCRUDLifecycle(t *testing.T) {
	svc := newTestService(t)

	// Create bucket.
	mustCreateBucket(t, svc, "lifecycle")

	// Create objects.
	mustPutObject(t, svc, "lifecycle", "doc.txt", []byte("v1"))
	mustPutObject(t, svc, "lifecycle", "img.png", []byte("png-data"))

	// Read objects.
	data, _, err := svc.GetObject("lifecycle", "doc.txt")
	if err != nil || string(data) != "v1" {
		t.Fatal("read failed after create")
	}

	// Update object.
	mustPutObject(t, svc, "lifecycle", "doc.txt", []byte("v2"))
	data, _, err = svc.GetObject("lifecycle", "doc.txt")
	if err != nil || string(data) != "v2" {
		t.Fatal("read failed after update")
	}

	// Delete object.
	if err := svc.DeleteObject("lifecycle", "img.png"); err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.GetObject("lifecycle", "img.png")
	if err == nil {
		t.Fatal("expected error after delete")
	}

	// List should show only remaining object.
	objs, err := svc.ListObjects("lifecycle", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 1 {
		t.Errorf("expected 1 remaining object, got %d", len(objs))
	}

	// Stats should reflect the state.
	stats := svc.Stats()
	if stats["total_buckets"].(int) != 1 {
		t.Error("stats buckets mismatch")
	}
}

// ---------------------------------------------------------------------------
// Stress test for prefix filtering
// ---------------------------------------------------------------------------

func TestListObjects_PrefixStress(t *testing.T) {
	svc := newTestService(t)
	mustCreateBucket(t, svc, "prefix-stress")

	// Create objects with different prefixes.
	prefixes := []string{"logs/", "data/", "config/", "temp/"}
	for _, prefix := range prefixes {
		for i := 0; i < 10; i++ {
			key := fmt.Sprintf("%sfile-%d.txt", prefix, i)
			mustPutObject(t, svc, "prefix-stress", key, []byte("content"))
		}
	}

	// Each prefix should return exactly 10.
	for _, prefix := range prefixes {
		objs, err := svc.ListObjects("prefix-stress", prefix, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(objs) != 10 {
			t.Errorf("prefix %q: got %d objects, want 10", prefix, len(objs))
		}
	}

	// No prefix should return all 40.
	all, err := svc.ListObjects("prefix-stress", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 40 {
		t.Errorf("all objects: got %d, want 40", len(all))
	}
}
