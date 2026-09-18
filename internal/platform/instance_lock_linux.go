package platform

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

const instanceLockName = "instance.lock"

// AlreadyRunningError reports that another process currently owns the application lock.
type AlreadyRunningError struct{}

func (*AlreadyRunningError) Error() string {
	return "another telegram-tui instance is already running"
}

// InstanceLock holds exclusive process ownership until Close is called.
type InstanceLock struct {
	file *os.File
	once sync.Once
	err  error
}

// AcquireInstanceLock acquires exclusive ownership without waiting for another process.
func AcquireInstanceLock(stateDir string) (*InstanceLock, error) {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(stateDir, 0o700); err != nil {
		return nil, err
	}

	lockPath := filepath.Join(stateDir, instanceLockName)
	fd, err := unix.Open(lockPath, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), lockPath)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("open instance lock")
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, errors.New("instance lock is not a regular file")
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, &AlreadyRunningError{}
		}
		return nil, err
	}
	return &InstanceLock{file: file}, nil
}

// Close releases ownership. It is safe to call more than once.
func (lock *InstanceLock) Close() error {
	if lock == nil {
		return nil
	}
	lock.once.Do(func() {
		unlockErr := unix.Flock(int(lock.file.Fd()), unix.LOCK_UN)
		closeErr := lock.file.Close()
		lock.err = errors.Join(unlockErr, closeErr)
	})
	return lock.err
}
