package lock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"cloud.google.com/go/storage"
	"github.com/cloud-mirror/state-sync-engine/pkg/interfaces"
	"google.golang.org/api/googleapi"
)

var (
	// ErrNotLockHolder is returned when a renew or release is attempted by a node
	// that does not currently hold the lock.
	ErrNotLockHolder = errors.New("not the lock holder")

	// ErrLockNotFound is returned when an operation targets a lock that does not exist.
	ErrLockNotFound = errors.New("lock not found")
)

// lockMetadata is the JSON payload stored inside the GCS lock object.
type lockMetadata struct {
	Holder    string `json:"holder"`
	Acquired  string `json:"acquired"`
	ExpiresAt string `json:"expires_at"`
}

// GCSLock implements interfaces.DistributedLock using a GCS object as an
// atomic compare-and-create / metadata-update mechanism.
type GCSLock struct {
	bucket *storage.BucketHandle
	nodeID string
	now    func() time.Time // injectable clock for testing
}

var _ interfaces.DistributedLock = (*GCSLock)(nil)

// NewGCSLock creates a distributed lock backed by the given GCS bucket.
// nodeID uniquely identifies the current process/node.
func NewGCSLock(client *storage.Client, bucketName string, nodeID string) *GCSLock {
	return &GCSLock{
		bucket: client.Bucket(bucketName),
		nodeID: nodeID,
		now:    time.Now,
	}
}

// Acquire attempts to create the lock object in GCS. It returns true if the
// lock was acquired, false if the lock is already held by another node (and
// not expired). An expired lock is cleaned up before retrying acquisition.
func (l *GCSLock) Acquire(ctx context.Context, lockKey string, ttl time.Duration) (bool, error) {
	obj := l.bucket.Object(lockKey)

	// Try to create the object with DoesNotExist precondition.
	acquired, err := l.tryCreate(ctx, obj, ttl)
	if err != nil {
		return false, err
	}
	if acquired {
		return true, nil
	}

	// Object exists — check whether the existing lock has expired.
	meta, err := l.readMeta(ctx, obj)
	if err != nil {
		// If the object disappeared between our create attempt and read, retry.
		if errors.Is(err, storage.ErrObjectNotExist) {
			return l.tryCreate(ctx, obj, ttl)
		}
		return false, fmt.Errorf("reading existing lock: %w", err)
	}

	expiresAt, parseErr := time.Parse(time.RFC3339, meta.ExpiresAt)
	if parseErr != nil || l.now().Before(expiresAt) {
		// Lock is still valid (or unparseable expiry — treat as held).
		return false, nil
	}

	// Lock expired — delete and re-acquire.
	if err := obj.Delete(ctx); err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		return false, fmt.Errorf("deleting expired lock: %w", err)
	}

	return l.tryCreate(ctx, obj, ttl)
}

// Renew extends the lock lease by updating the GCS object metadata.
// Returns ErrNotLockHolder if the caller does not own the lock.
func (l *GCSLock) Renew(ctx context.Context, lockKey string, ttl time.Duration) error {
	obj := l.bucket.Object(lockKey)

	meta, err := l.readMeta(ctx, obj)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return ErrLockNotFound
		}
		return fmt.Errorf("reading lock for renewal: %w", err)
	}

	if meta.Holder != l.nodeID {
		return ErrNotLockHolder
	}

	newMeta := lockMetadata{
		Holder:    l.nodeID,
		Acquired:  meta.Acquired,
		ExpiresAt: l.now().Add(ttl).Format(time.RFC3339),
	}
	if err := l.writeMeta(ctx, obj, newMeta); err != nil {
		return fmt.Errorf("renewing lock: %w", err)
	}

	return nil
}

// Release deletes the lock object in GCS.
// Returns ErrNotLockHolder if the caller does not own the lock.
// If the lock already expired/deleted, the release succeeds silently.
func (l *GCSLock) Release(ctx context.Context, lockKey string) error {
	obj := l.bucket.Object(lockKey)

	meta, err := l.readMeta(ctx, obj)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil // already released / expired
		}
		return fmt.Errorf("reading lock for release: %w", err)
	}

	if meta.Holder != l.nodeID {
		return ErrNotLockHolder
	}

	if err := obj.Delete(ctx); err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		return fmt.Errorf("deleting lock object: %w", err)
	}

	return nil
}

// GetHolder reads the lock object and returns the holder identifier.
// Returns ErrLockNotFound if the lock does not exist.
func (l *GCSLock) GetHolder(ctx context.Context, lockKey string) (string, error) {
	obj := l.bucket.Object(lockKey)

	meta, err := l.readMeta(ctx, obj)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return "", ErrLockNotFound
		}
		return "", fmt.Errorf("reading lock holder: %w", err)
	}

	return meta.Holder, nil
}

// tryCreate attempts to create a new GCS object with DoesNotExist precondition.
// Returns (true, nil) on success, (false, nil) if the object already exists.
func (l *GCSLock) tryCreate(ctx context.Context, obj *storage.ObjectHandle, ttl time.Duration) (bool, error) {
	now := l.now()
	meta := lockMetadata{
		Holder:    l.nodeID,
		Acquired:  now.Format(time.RFC3339),
		ExpiresAt: now.Add(ttl).Format(time.RFC3339),
	}

	data, err := json.Marshal(meta)
	if err != nil {
		return false, fmt.Errorf("marshaling lock metadata: %w", err)
	}

	w := obj.If(storage.Conditions{DoesNotExist: true}).NewWriter(ctx)
	w.ContentType = "application/json"

	if _, err := w.Write(data); err != nil {
		// Write errors are not authoritative before Close.
		_ = w.Close()
		return false, fmt.Errorf("writing lock object: %w", err)
	}

	if err := w.Close(); err != nil {
		// HTTP 412 Precondition Failed means the object already exists.
		var gerr *googleapi.Error
		if errors.As(err, &gerr) && gerr.Code == 412 {
			return false, nil
		}
		return false, fmt.Errorf("closing lock writer: %w", err)
	}

	return true, nil
}

// readMeta fetches and parses the lock object's JSON body.
func (l *GCSLock) readMeta(ctx context.Context, obj *storage.ObjectHandle) (*lockMetadata, error) {
	r, err := obj.NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading lock data: %w", err)
	}

	var meta lockMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing lock metadata: %w", err)
	}

	return &meta, nil
}

// writeMeta replaces the lock object content with updated metadata.
func (l *GCSLock) writeMeta(ctx context.Context, obj *storage.ObjectHandle, meta lockMetadata) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshaling lock metadata: %w", err)
	}

	w := obj.NewWriter(ctx)
	w.ContentType = "application/json"

	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return err
	}

	return w.Close()
}

