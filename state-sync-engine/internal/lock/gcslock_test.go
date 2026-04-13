package lock

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

// fakeGCS is a minimal in-process GCS server that supports:
//   - PUT (upload/overwrite)
//   - GET (download)
//   - DELETE
//   - x-goog-if-generation-match: 0 precondition (DoesNotExist)
type fakeGCS struct {
	mu      sync.Mutex
	objects map[string][]byte // key = "/bucket/object"
}

func newFakeGCS() *fakeGCS {
	return &fakeGCS{objects: make(map[string][]byte)}
}

func (f *fakeGCS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	path := r.URL.Path

	// JSON API upload: POST /upload/storage/v1/b/{bucket}/o
	if r.Method == http.MethodPost && strings.HasPrefix(path, "/upload/storage/v1/b/") {
		bucket := extractSegment(path, "/upload/storage/v1/b/", "/o")
		name := r.URL.Query().Get("name")
		key := "/" + bucket + "/" + name

		// Check DoesNotExist precondition (ifGenerationMatch=0).
		if r.URL.Query().Get("ifGenerationMatch") == "0" {
			if _, exists := f.objects[key]; exists {
				w.WriteHeader(http.StatusPreconditionFailed)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":    412,
						"message": "Precondition Failed",
					},
				})
				return
			}
		}

		data := extractContent(r)
		f.objects[key] = data
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":   name,
			"bucket": bucket,
		})
		return
	}

	// JSON API object endpoint: GET /storage/v1/b/{bucket}/o/{object}
	// With ?alt=media it downloads the content; without it returns metadata.
	if r.Method == http.MethodGet && strings.HasPrefix(path, "/storage/v1/b/") && strings.Contains(path, "/o/") {
		bucket, object := extractBucketObject(path)
		key := "/" + bucket + "/" + object
		data, exists := f.objects[key]
		if !exists {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if r.URL.Query().Get("alt") == "media" {
			// Download the object content.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(data)
			return
		}

		// Return metadata.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":   object,
			"bucket": bucket,
			"size":   len(data),
		})
		return
	}

	// XML API GET (download): GET /{bucket}/{object}
	if r.Method == http.MethodGet {
		key := path
		data, exists := f.objects[key]
		if !exists {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
		return
	}

	// JSON API DELETE: DELETE /storage/v1/b/{bucket}/o/{object} or /b/{bucket}/o/{object}
	if r.Method == http.MethodDelete {
		deletePath := path
		deletePath = strings.TrimPrefix(deletePath, "/storage/v1")
		// Now path is /b/{bucket}/o/{object}
		if strings.HasPrefix(deletePath, "/b/") {
			trimmed := strings.TrimPrefix(deletePath, "/b/")
			parts := strings.SplitN(trimmed, "/o/", 2)
			if len(parts) == 2 {
				key := "/" + parts[0] + "/" + parts[1]
				if _, exists := f.objects[key]; !exists {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				delete(f.objects, key)
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
	}

	w.WriteHeader(http.StatusNotFound)
}

// extractContent reads the object content from an upload request body.
// For multipart uploads the body is a MIME multipart message where the first
// part is JSON metadata and the second part is the actual content.
func extractContent(r *http.Request) []byte {
	ct := r.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(ct)
	if err == nil && strings.HasPrefix(mediaType, "multipart/") {
		mr := multipart.NewReader(r.Body, params["boundary"])
		partIndex := 0
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			data, _ := io.ReadAll(part)
			if partIndex == 1 {
				// Second part is the actual content.
				return data
			}
			partIndex++
		}
	}
	data, _ := io.ReadAll(r.Body)
	return data
}

func extractSegment(path, prefix, suffix string) string {
	after := strings.TrimPrefix(path, prefix)
	idx := strings.Index(after, suffix)
	if idx < 0 {
		return after
	}
	return after[:idx]
}

func extractBucketObject(path string) (string, string) {
	// /storage/v1/b/{bucket}/o/{object}
	trimmed := strings.TrimPrefix(path, "/storage/v1/b/")
	parts := strings.SplitN(trimmed, "/o/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", ""
}

func setupTest(t *testing.T) (*GCSLock, *fakeGCS) {
	t.Helper()

	fake := newFakeGCS()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	client, err := storage.NewClient(
		context.Background(),
		option.WithEndpoint(server.URL),
		option.WithHTTPClient(server.Client()),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("failed to create storage client: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	lock := NewGCSLock(client, "test-bucket", "node-1")
	return lock, fake
}

func setupTestWithNodeID(t *testing.T, nodeID string) (*GCSLock, *fakeGCS) {
	t.Helper()

	fake := newFakeGCS()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	client, err := storage.NewClient(
		context.Background(),
		option.WithEndpoint(server.URL),
		option.WithHTTPClient(server.Client()),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("failed to create storage client: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	lock := NewGCSLock(client, "test-bucket", nodeID)
	return lock, fake
}

func setupTwoNodes(t *testing.T) (*GCSLock, *GCSLock) {
	t.Helper()

	fake := newFakeGCS()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	makeClient := func() *storage.Client {
		c, err := storage.NewClient(
			context.Background(),
			option.WithEndpoint(server.URL),
			option.WithHTTPClient(server.Client()),
			option.WithoutAuthentication(),
		)
		if err != nil {
			t.Fatalf("failed to create storage client: %v", err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}

	lock1 := NewGCSLock(makeClient(), "test-bucket", "node-1")
	lock2 := NewGCSLock(makeClient(), "test-bucket", "node-2")
	return lock1, lock2
}

func TestAcquire_Success(t *testing.T) {
	lock, _ := setupTest(t)
	ctx := context.Background()

	acquired, err := lock.Acquire(ctx, "test-lock", 30*time.Second)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	if !acquired {
		t.Fatal("expected lock to be acquired")
	}
}

func TestAcquire_AlreadyHeld(t *testing.T) {
	lock1, lock2 := setupTwoNodes(t)
	ctx := context.Background()

	acquired, err := lock1.Acquire(ctx, "test-lock", 30*time.Second)
	if err != nil {
		t.Fatalf("Acquire by node-1 failed: %v", err)
	}
	if !acquired {
		t.Fatal("expected node-1 to acquire lock")
	}

	acquired, err = lock2.Acquire(ctx, "test-lock", 30*time.Second)
	if err != nil {
		t.Fatalf("Acquire by node-2 failed: %v", err)
	}
	if acquired {
		t.Fatal("expected node-2 to fail acquiring lock held by node-1")
	}
}

func TestAcquire_ExpiredLockReacquired(t *testing.T) {
	lock1, lock2 := setupTwoNodes(t)
	ctx := context.Background()

	// Node-1 acquires with a clock that produces an already-expired TTL.
	pastTime := time.Now().Add(-1 * time.Hour)
	lock1.now = func() time.Time { return pastTime }
	acquired, err := lock1.Acquire(ctx, "test-lock", 30*time.Second)
	if err != nil {
		t.Fatalf("Acquire by node-1 failed: %v", err)
	}
	if !acquired {
		t.Fatal("expected node-1 to acquire lock")
	}

	// Node-2 with current time should be able to acquire since lock is expired.
	acquired, err = lock2.Acquire(ctx, "test-lock", 30*time.Second)
	if err != nil {
		t.Fatalf("Acquire by node-2 failed: %v", err)
	}
	if !acquired {
		t.Fatal("expected node-2 to acquire expired lock")
	}
}

func TestRenew_Success(t *testing.T) {
	lock, _ := setupTest(t)
	ctx := context.Background()

	acquired, err := lock.Acquire(ctx, "test-lock", 30*time.Second)
	if err != nil || !acquired {
		t.Fatalf("Acquire failed: acquired=%v err=%v", acquired, err)
	}

	if err := lock.Renew(ctx, "test-lock", 60*time.Second); err != nil {
		t.Fatalf("Renew failed: %v", err)
	}
}

func TestRenew_NotHolder(t *testing.T) {
	lock1, lock2 := setupTwoNodes(t)
	ctx := context.Background()

	acquired, _ := lock1.Acquire(ctx, "test-lock", 30*time.Second)
	if !acquired {
		t.Fatal("expected node-1 to acquire lock")
	}

	err := lock2.Renew(ctx, "test-lock", 60*time.Second)
	if err != ErrNotLockHolder {
		t.Fatalf("expected ErrNotLockHolder, got: %v", err)
	}
}

func TestRenew_LockNotFound(t *testing.T) {
	lock, _ := setupTest(t)
	ctx := context.Background()

	err := lock.Renew(ctx, "nonexistent-lock", 30*time.Second)
	if err != ErrLockNotFound {
		t.Fatalf("expected ErrLockNotFound, got: %v", err)
	}
}

func TestRelease_Success(t *testing.T) {
	lock, _ := setupTest(t)
	ctx := context.Background()

	acquired, _ := lock.Acquire(ctx, "test-lock", 30*time.Second)
	if !acquired {
		t.Fatal("expected lock to be acquired")
	}

	if err := lock.Release(ctx, "test-lock"); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	// After release, another acquire should succeed.
	acquired, err := lock.Acquire(ctx, "test-lock", 30*time.Second)
	if err != nil {
		t.Fatalf("Re-acquire failed: %v", err)
	}
	if !acquired {
		t.Fatal("expected lock to be re-acquirable after release")
	}
}

func TestRelease_NotHolder(t *testing.T) {
	lock1, lock2 := setupTwoNodes(t)
	ctx := context.Background()

	acquired, _ := lock1.Acquire(ctx, "test-lock", 30*time.Second)
	if !acquired {
		t.Fatal("expected node-1 to acquire lock")
	}

	err := lock2.Release(ctx, "test-lock")
	if err != ErrNotLockHolder {
		t.Fatalf("expected ErrNotLockHolder, got: %v", err)
	}
}

func TestRelease_AlreadyReleased(t *testing.T) {
	lock, _ := setupTest(t)
	ctx := context.Background()

	// Releasing a nonexistent lock should not error.
	if err := lock.Release(ctx, "nonexistent-lock"); err != nil {
		t.Fatalf("Release of nonexistent lock should succeed silently, got: %v", err)
	}
}

func TestGetHolder_Success(t *testing.T) {
	lock, _ := setupTest(t)
	ctx := context.Background()

	acquired, _ := lock.Acquire(ctx, "test-lock", 30*time.Second)
	if !acquired {
		t.Fatal("expected lock to be acquired")
	}

	holder, err := lock.GetHolder(ctx, "test-lock")
	if err != nil {
		t.Fatalf("GetHolder failed: %v", err)
	}
	if holder != "node-1" {
		t.Fatalf("expected holder node-1, got %s", holder)
	}
}

func TestGetHolder_NotFound(t *testing.T) {
	lock, _ := setupTest(t)
	ctx := context.Background()

	_, err := lock.GetHolder(ctx, "nonexistent-lock")
	if err != ErrLockNotFound {
		t.Fatalf("expected ErrLockNotFound, got: %v", err)
	}
}

func TestAcquireRenewReleaseCycle(t *testing.T) {
	lock, _ := setupTest(t)
	ctx := context.Background()

	// Acquire
	acquired, err := lock.Acquire(ctx, "test-lock", 30*time.Second)
	if err != nil || !acquired {
		t.Fatalf("Acquire failed: acquired=%v err=%v", acquired, err)
	}

	// Verify holder
	holder, err := lock.GetHolder(ctx, "test-lock")
	if err != nil {
		t.Fatalf("GetHolder failed: %v", err)
	}
	if holder != "node-1" {
		t.Fatalf("expected holder node-1, got %s", holder)
	}

	// Renew
	if err := lock.Renew(ctx, "test-lock", 60*time.Second); err != nil {
		t.Fatalf("Renew failed: %v", err)
	}

	// Holder should still be the same
	holder, err = lock.GetHolder(ctx, "test-lock")
	if err != nil {
		t.Fatalf("GetHolder after renew failed: %v", err)
	}
	if holder != "node-1" {
		t.Fatalf("expected holder node-1 after renew, got %s", holder)
	}

	// Release
	if err := lock.Release(ctx, "test-lock"); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	// Holder should not be found
	_, err = lock.GetHolder(ctx, "test-lock")
	if err != ErrLockNotFound {
		t.Fatalf("expected ErrLockNotFound after release, got: %v", err)
	}
}

func TestConcurrentAcquire(t *testing.T) {
	lock1, lock2 := setupTwoNodes(t)
	ctx := context.Background()

	var (
		wg          sync.WaitGroup
		acquired1   bool
		acquired2   bool
		err1, err2  error
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		acquired1, err1 = lock1.Acquire(ctx, "test-lock", 30*time.Second)
	}()
	go func() {
		defer wg.Done()
		acquired2, err2 = lock2.Acquire(ctx, "test-lock", 30*time.Second)
	}()
	wg.Wait()

	if err1 != nil {
		t.Fatalf("node-1 Acquire error: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("node-2 Acquire error: %v", err2)
	}

	// Exactly one should have acquired.
	if acquired1 == acquired2 {
		t.Fatalf("expected exactly one node to acquire lock: node-1=%v, node-2=%v", acquired1, acquired2)
	}
}
