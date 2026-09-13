package store

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/prateekkhurmi/kvprjt/wal"
)

const DefaultMaxFileSize = 64 * 1024 * 1024

type Store struct {
	dir     string
	wal     *wal.WAL
	index   *Index
	mu      sync.RWMutex
	maxSize int64
	closed  bool
}

func New(dir string) (*Store, error) {
	w, err := wal.NewWAL(dir)
	if err != nil {
		return nil, err
	}

	s := &Store{
		dir:    dir,
		wal:    w,
		index:  NewIndex(),
		maxSize: DefaultMaxFileSize,
	}

	if err := s.recover(); err != nil {
		w.Close()
		return nil, err
	}

	return s, nil
}

func (s *Store) Put(key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("store is closed")
	}

	offset, err := s.wal.Append([]byte(key), value)
	if err != nil {
		return err
	}

	s.index.Put(key, &EntryLoc{
		FileID: s.wal.FileID(),
		Offset: offset,
		Size:   len(key) + len(value) + wal.TotalHeader,
	})
	return nil
}

func (s *Store) Get(key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, errors.New("store is closed")
	}

	loc, ok := s.index.Get(key)
	if !ok || loc.Deleted {
		return nil, errors.New("key not found")
	}

	rec, err := s.wal.ReadAt(loc.FileID, loc.Offset)
	if err != nil {
		return nil, err
	}
	if rec.Flag == wal.FlagDelete {
		return nil, errors.New("key not found")
	}
	return rec.Value, nil
}

func (s *Store) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("store is closed")
	}

	_, err := s.wal.AppendDelete([]byte(key))
	if err != nil {
		return err
	}

	s.index.Delete(key)
	return nil
}

func (s *Store) Has(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.index.Has(key)
}

func (s *Store) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.index.Size()
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return s.wal.Close()
}

func (s *Store) WAL() *wal.WAL { return s.wal }
func (s *Store) Dir() string { return s.dir }

func (s *Store) recover() error {
	files, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".log" {
			continue
		}

		fileID, _ := parseFileID(f.Name())
		path := filepath.Join(s.dir, f.Name())
		file, err := os.Open(path)
		if err != nil {
			continue
		}

		offset := int64(0)
		lastValidOffset := int64(0)
		for {
			pos, _ := file.Seek(0, 1)
			rec, err := wal.ReadRecord(file)
			if err == io.EOF {
				if pos < getFileSize(path) {
					s.truncateFile(path, pos)
				}
				break
			}
			if err != nil {
				if err == wal.ErrChecksumMismatch {
					s.truncateFile(path, lastValidOffset)
				}
				break
			}

			if rec.Flag == wal.FlagDelete {
				s.index.Delete(string(rec.Key))
			} else {
				s.index.Put(string(rec.Key), &EntryLoc{
					FileID: fileID,
					Offset: offset,
					Size:   rec.Size(),
				})
			}
			lastValidOffset = offset + int64(rec.Size())
			offset += int64(rec.Size())
		}
		file.Close()
	}
	return nil
}

func (s *Store) truncateFile(path string, size int64) {
	f, err := os.OpenFile(path, os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Truncate(size)
}

func parseFileID(name string) (uint64, error) {
	var id uint64
	fmt.Sscanf(name, "wal_%06d.log", &id)
	return id, nil
}

func getFileSize(path string) int64 {
	info, _ := os.Stat(path)
	return info.Size()
}
