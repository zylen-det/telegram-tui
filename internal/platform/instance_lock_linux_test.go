package platform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstanceLockRejectsLiveOwnerWithTypedAlreadyRunningError(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	first, err := AcquireInstanceLock(stateDir)
	if err != nil {
		t.Fatalf("AcquireInstanceLock(first) error = %v", err)
	}
	defer first.Close()

	second, err := AcquireInstanceLock(stateDir)
	if second != nil {
		_ = second.Close()
		t.Fatal("AcquireInstanceLock(second) returned ownership")
	}
	var alreadyRunning *AlreadyRunningError
	if !errors.As(err, &alreadyRunning) {
		t.Fatalf("AcquireInstanceLock(second) error = %T, want *AlreadyRunningError", err)
	}
	if got := err.Error(); got != "another telegram-tui instance is already running" {
		t.Fatalf("already-running error = %q", got)
	}
}

func TestInstanceLockAcquiresStaleFileAndKeepsItPrivate(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(stateDir, "instance.lock")
	if err := os.WriteFile(lockPath, []byte("stale payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	lock, err := AcquireInstanceLock(stateDir)
	if err != nil {
		t.Fatalf("AcquireInstanceLock(stale) error = %v", err)
	}
	defer lock.Close()

	for _, path := range []string{stateDir, lockPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 && path == stateDir {
			t.Fatalf("state directory mode = %o, want 700", info.Mode().Perm())
		}
		if info.Mode().Perm() != 0o600 && path == lockPath {
			t.Fatalf("lock file mode = %o, want 600", info.Mode().Perm())
		}
	}
	payload, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "stale payload" {
		t.Fatalf("stale lock payload changed to %q", payload)
	}
}

func TestInstanceLockRejectsSymlinkWithoutChangingTarget(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("unchanged"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(stateDir, "instance.lock")); err != nil {
		t.Fatal(err)
	}
	lock, err := AcquireInstanceLock(stateDir)
	if lock != nil {
		_ = lock.Close()
		t.Fatal("AcquireInstanceLock followed a symlink")
	}
	if err == nil {
		t.Fatal("AcquireInstanceLock accepted a symlink")
	}
	info, statErr := os.Stat(target)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("symlink target mode changed to %o", info.Mode().Perm())
	}
	payload, readErr := os.ReadFile(target)
	if readErr != nil || string(payload) != "unchanged" {
		t.Fatal("symlink target content changed")
	}
}

func TestInstanceLockReleasesOwnership(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	first, err := AcquireInstanceLock(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	second, err := AcquireInstanceLock(stateDir)
	if err != nil {
		t.Fatalf("AcquireInstanceLock(after release) error = %v", err)
	}
	defer second.Close()
}
