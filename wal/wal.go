package wal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type WAL struct {
	dir      string
	file     *os.File
	fileID   uint64
	fileSize int64
	mu       sync.Mutex
	closed   bool
	files    map[uint64]*os.File
}

func NewWAL(dir string) (*WAL, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	wal := &WAL{dir: dir, files: make(map[uint64]*os.File)}
	if err := wal.openNewFileLocked(); err != nil {
		return nil, err
	}
	return wal, nil
}

func (w *WAL) openNewFileLocked() error {
	files, err := os.ReadDir(w.dir)
	if err != nil {
		return err
	}

	maxID := uint64(0)
	hasExisting := false
	for _, f := range files {
		if !f.IsDir() && filepath.Ext(f.Name()) == ".log" {
			hasExisting = true
			var id uint64
			fmt.Sscanf(f.Name(), "wal_%06d.log", &id)
			if id >= maxID {
				maxID = id + 1
			}
		}
	}
	w.fileID = maxID

	path := w.filePath()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	w.file = f
	w.files[w.fileID] = f
	info, err := f.Stat()
	if err != nil {
		return err
	}
	w.fileSize = info.Size()

	if hasExisting && w.fileID == 0 {
		w.fileID = 1
		path = w.filePath()
		f, err = os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
		w.file = f
		w.files[w.fileID] = f
		w.fileSize = 0
	}

	return nil
}

func (w *WAL) filePath() string {
	return fmt.Sprintf("%s/wal_%06d.log", w.dir, w.fileID)
}

func (w *WAL) filePathFor(fileID uint64) string {
	return fmt.Sprintf("%s/wal_%06d.log", w.dir, fileID)
}

func (w *WAL) openFileLocked(fileID uint64) (*os.File, error) {
	if f, ok := w.files[fileID]; ok {
		return f, nil
	}

	path := w.filePathFor(fileID)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	w.files[fileID] = f
	return f, nil
}

func (w *WAL) Append(key, value []byte) (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, errors.New("wal is closed")
	}

	rec := &Record{Key: key, Value: value, Flag: FlagNormal}
	data := rec.Bytes()

	n, err := w.file.Write(data)
	if err != nil {
		return 0, err
	}
	if err := w.file.Sync(); err != nil {
		return 0, err
	}

	offset := w.fileSize
	w.fileSize += int64(n)
	return offset, nil
}

func (w *WAL) AppendDelete(key []byte) (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, errors.New("wal is closed")
	}

	rec := &Record{Key: key, Value: nil, Flag: FlagDelete}
	data := rec.Bytes()

	n, err := w.file.Write(data)
	if err != nil {
		return 0, err
	}
	if err := w.file.Sync(); err != nil {
		return 0, err
	}

	offset := w.fileSize
	w.fileSize += int64(n)
	return offset, nil
}

func (w *WAL) ReadAt(fileID uint64, offset int64) (*Record, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	f, err := w.openFileLocked(fileID)
	if err != nil {
		return nil, err
	}

	_, err = f.Seek(offset, 0)
	if err != nil {
		return nil, err
	}
	return ReadRecord(f)
}

func (w *WAL) Iterate(fn func(offset int64, rec *Record) error) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return errors.New("wal file not open")
	}

	_, err := w.file.Seek(0, 0)
	if err != nil {
		return err
	}

	offset := int64(0)
	for {
		pos, _ := w.file.Seek(0, 1)
		rec, err := ReadRecord(w.file)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := fn(pos, rec); err != nil {
			return err
		}
		offset += int64(rec.Size())
	}
}

func (w *WAL) Truncate(size int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Truncate(size)
}

func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	for _, f := range w.files {
		f.Close()
	}
	w.files = make(map[uint64]*os.File)
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

func (w *WAL) FileID() uint64 { return w.fileID }
func (w *WAL) FileSize() int64 { return w.fileSize }
func (w *WAL) Dir() string { return w.dir }
func (w *WAL) OpenFile() *os.File { return w.file }
