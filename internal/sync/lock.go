package sync

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// syncLock is a flock-style advisory lock at the store root (.sync.lock) that
// serializes concurrent [GitSync.Sync] calls on one store. A second caller
// blocks for at most Config.LockTimeout rather than corrupting the working tree
// or git index (which are not safe for concurrent writers).
type syncLock struct {
	fl  *flock.Flock
	dir string
}

func newSyncLock(storeDir string) *syncLock {
	return &syncLock{
		fl:  flock.New(filepath.Join(storeDir, ".sync.lock")),
		dir: storeDir,
	}
}

// acquire blocks until the lock is held or ctx expires.
func (l *syncLock) acquire(ctx context.Context) error {
	locked, err := l.fl.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return fmt.Errorf("sync lock contention on %s: %w", l.dir, err)
	}
	if !locked {
		return fmt.Errorf("sync lock contention on %s: could not acquire lock (timed out)", l.dir)
	}
	return nil
}

func (l *syncLock) release() error {
	return l.fl.Unlock()
}

func defaultLockTimeout() time.Duration { return 30 * time.Second }
