package config

import (
	"os"
	"path/filepath"
	"syscall"
)

// FileLock holds an exclusive flock on a sentinel file.
type FileLock struct {
	f *os.File
}

// AcquireLock opens (or creates) lockPath and takes an exclusive flock.
// It blocks until the lock is held. Caller must Release().
func AcquireLock(lockPath string) (*FileLock, error) {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &FileLock{f: f}, nil
}

// Release unlocks and closes.
func (l *FileLock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	return l.f.Close()
}
