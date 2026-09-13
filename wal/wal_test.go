package wal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendRead(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWAL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	offset, err := w.Append([]byte("hello"), []byte("world"))
	if err != nil {
		t.Fatal(err)
	}

	rec, err := w.ReadAt(0, offset)
	if err != nil {
		t.Fatal(err)
	}
	if string(rec.Key) != "hello" || string(rec.Value) != "world" {
		t.Fatalf("got key=%s val=%s, want hello/world", rec.Key, rec.Value)
	}
}

func TestChecksumCatchesCorruption(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWAL(dir)
	if err != nil {
		t.Fatal(err)
	}

	_, err = w.Append([]byte("key"), []byte("value"))
	if err != nil {
		t.Fatal(err)
	}
	w.Close()

	path := filepath.Join(dir, "wal_000000.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	data[0] ^= 0xFF
	data[1] ^= 0xFF
	data[2] ^= 0xFF
	data[3] ^= 0xFF
	os.WriteFile(path, data, 0644)

	w2, err := NewWAL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Close()

	_, err = w.ReadAt(0, 0)
	if err != ErrChecksumMismatch {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestDeleteRecord(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWAL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	_, err = w.Append([]byte("key"), []byte("value"))
	if err != nil {
		t.Fatal(err)
	}

	delOffset, err := w.AppendDelete([]byte("key"))
	if err != nil {
		t.Fatal(err)
	}

	rec, err := w.ReadAt(0, delOffset)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Flag != FlagDelete {
		t.Fatalf("expected delete flag, got %d", rec.Flag)
	}
}

func TestIterate(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWAL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	w.Append([]byte("a"), []byte("1"))
	w.Append([]byte("b"), []byte("2"))
	w.Append([]byte("c"), []byte("3"))

	count := 0
	w.Iterate(func(offset int64, rec *Record) error {
		count++
		return nil
	})

	if count != 3 {
		t.Fatalf("expected 3 records, got %d", count)
	}
}
