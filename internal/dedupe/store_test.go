package dedupe_test

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qiz029/vibe-pushover/internal/dedupe"
)

func TestStoreReservesFingerprintOnceAcrossConcurrentCallers(t *testing.T) {
	t.Parallel()

	store := dedupe.Store{Path: filepath.Join(t.TempDir(), "cache", "dedupe.json")}
	var acquired atomic.Int32
	var pending atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := store.Reserve("same-notification")
			if err != nil {
				t.Errorf("Reserve() error = %v", err)
				return
			}
			switch result.Status {
			case dedupe.StatusAcquired:
				acquired.Add(1)
			case dedupe.StatusPending:
				pending.Add(1)
			default:
				t.Errorf("Reserve() status = %v, want acquired or pending", result.Status)
			}
		}()
	}
	wg.Wait()
	if acquired.Load() != 1 || pending.Load() != 11 {
		t.Fatalf("acquired=%d pending=%d, want 1 and 11", acquired.Load(), pending.Load())
	}
}

func TestStoreOldOwnerCannotReleaseNewReservation(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	store := dedupe.Store{Path: filepath.Join(t.TempDir(), "dedupe.json"), Now: func() time.Time { return now }}
	first, err := store.Reserve("notification")
	if err != nil || first.Status != dedupe.StatusAcquired {
		t.Fatalf("first Reserve() = %#v, %v", first, err)
	}
	now = now.Add(4 * time.Second)
	second, err := store.Reserve("notification")
	if err != nil || second.Status != dedupe.StatusAcquired {
		t.Fatalf("second Reserve() = %#v, %v", second, err)
	}
	if err := store.Release(first.Reservation); err != nil {
		t.Fatalf("Release(old owner) error = %v", err)
	}
	got, err := store.Reserve("notification")
	if err != nil {
		t.Fatalf("third Reserve() error = %v", err)
	}
	if got.Status != dedupe.StatusPending {
		t.Fatalf("third Reserve() status = %v, want pending newer reservation", got.Status)
	}
}

func TestStoreDoesNotTakeOverExistingLock(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "dedupe.json")
	if err := os.Mkdir(path+".lock", 0o700); err != nil {
		t.Fatalf("Mkdir(lock) error = %v", err)
	}
	store := dedupe.Store{Path: path}
	if _, err := store.Reserve("notification"); err == nil {
		t.Fatal("Reserve() error = nil, want fail-open lock timeout")
	}
	if _, err := os.Stat(path + ".lock"); err != nil {
		t.Fatalf("existing lock was removed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("state file exists despite unavailable lock: %v", err)
	}
}

func TestStoreCreatesPrivateState(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "cache")
	path := filepath.Join(dir, "dedupe.json")
	store := dedupe.Store{Path: path}
	if _, err := store.Reserve("notification"); err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(state) error = %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("state permissions = %o, want 600", got)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat(dir) error = %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory permissions = %o, want 700", got)
	}
}
