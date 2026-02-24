package db

import (
	"io"
	"os"
	"sync"
	"syscall"
)

// Store provides a simple read/write abstraction for persistence.
type Store interface {
	Read() ([]byte, error)
	Write([]byte) error
	Close() error
}

// lockedFileStore implements Store backed by an os.File with flock-based
// advisory locking on each read and write. The mutex serializes in-process
// goroutines; the flock serializes across processes.
type lockedFileStore struct {
	mu sync.Mutex
	f  *os.File
}

var _ Store = (*lockedFileStore)(nil)

func newLockedFileStore(filename string) *lockedFileStore {
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0o666)
	if err != nil {
		panic(err)
	}

	return &lockedFileStore{f: f}
}

func (s *lockedFileStore) Read() ([]byte, error) {
	s.lock()
	defer s.unlock()

	if _, err := s.f.Seek(0, 0); err != nil {
		return nil, err
	}

	return io.ReadAll(s.f)
}

func (s *lockedFileStore) Write(b []byte) error {
	s.lock()
	defer s.unlock()

	if err := s.f.Truncate(0); err != nil {
		return err
	}

	if _, err := s.f.Seek(0, 0); err != nil {
		return err
	}

	_, err := s.f.Write(b)
	return err
}

func (s *lockedFileStore) Close() error {
	return s.f.Close()
}

func (s *lockedFileStore) lock() {
	s.mu.Lock()

	if err := syscall.Flock(int(s.f.Fd()), syscall.LOCK_EX); err != nil {
		panic(err)
	}
}

func (s *lockedFileStore) unlock() {
	if err := syscall.Flock(int(s.f.Fd()), syscall.LOCK_UN); err != nil {
		panic(err)
	}

	s.mu.Unlock()
}
