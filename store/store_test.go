package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPutGet(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	err = s.Put("foo", []byte("bar"))
	if err != nil {
		t.Fatal(err)
	}

	val, err := s.Get("foo")
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "bar" {
		t.Fatalf("got %s, want bar", val)
	}
}

func TestGetMissing(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	_, err = s.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	err = s.Put("foo", []byte("bar"))
	if err != nil {
		t.Fatal(err)
	}

	err = s.Delete("foo")
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.Get("foo")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestUpdate(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	err = s.Put("key", []byte("v1"))
	if err != nil {
		t.Fatal(err)
	}

	err = s.Put("key", []byte("v2"))
	if err != nil {
		t.Fatal(err)
	}

	val, err := s.Get("key")
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "v2" {
		t.Fatalf("got %s, want v2", val)
	}
}

func TestRecovery(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.Put("a", []byte("1"))
	s.Put("b", []byte("2"))
	s.Put("c", []byte("3"))
	s.Close()

	s2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	for _, key := range []string{"a", "b", "c"} {
		val, err := s2.Get(key)
		if err != nil {
			t.Fatalf("recovery failed for key %s: %v", key, err)
		}
		if key == "a" && string(val) != "1" {
			t.Fatalf("got %s, want 1", val)
		}
		if key == "b" && string(val) != "2" {
			t.Fatalf("got %s, want 2", val)
		}
		if key == "c" && string(val) != "3" {
			t.Fatalf("got %s, want 3", val)
		}
	}
}

func TestRecoveryTruncatesTornWrite(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.Put("a", []byte("1"))
	s.Put("b", []byte("2"))
	s.Close()

	// Append corrupt bytes to the existing WAL file
	path := filepath.Join(dir, "wal_000000.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("CORRUPT")
	f.Close()

	s2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	val, err := s2.Get("a")
	if err != nil {
		t.Fatalf("recovered key 'a' should exist: %v", err)
	}
	if string(val) != "1" {
		t.Fatalf("got %s, want 1", val)
	}

	val, err = s2.Get("b")
	if err != nil {
		t.Fatalf("recovered key 'b' should exist: %v", err)
	}
	if string(val) != "2" {
		t.Fatalf("got %s, want 2", val)
	}
}

func TestConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	done := make(chan bool)

	go func() {
		for i := 0; i < 100; i++ {
			key := string(rune('a' + i%26)) + string(rune('0' + i/26))
			s.Put(key, []byte("value"))
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			key := string(rune('a' + i%26)) + string(rune('0' + i/26))
			s.Has(key)
		}
		done <- true
	}()

	<-done
	<-done

	if s.Size() == 0 {
		t.Fatal("expected some keys to exist")
	}
}

func TestSize(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if s.Size() != 0 {
		t.Fatalf("expected size 0, got %d", s.Size())
	}

	s.Put("a", []byte("1"))
	s.Put("b", []byte("2"))

	if s.Size() != 2 {
		t.Fatalf("expected size 2, got %d", s.Size())
	}

	s.Delete("a")
	if s.Size() != 1 {
		t.Fatalf("expected size 1, got %d", s.Size())
	}
}

func TestChecksumCatchesCorruption(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.Put("a", []byte("1"))
	s.Put("b", []byte("2"))
	s.Close()

	// Find the actual WAL file path
	path := filepath.Join(dir, "wal_000000.log")
	data, _ := os.ReadFile(path)

	// First record: 13B header + len("a")=1 + len("1")=1 = 15 bytes
	firstRecSize := 13 + 1 + 1
	data[firstRecSize] ^= 0xFF
	data[firstRecSize+1] ^= 0xFF
	data[firstRecSize+2] ^= 0xFF
	data[firstRecSize+3] ^= 0xFF
	os.WriteFile(path, data, 0644)

	s2, err := New(dir)
	if err != nil {
		t.Fatal("expected store to open after recovery from corrupted tail")
	}
	defer s2.Close()

	val, err := s2.Get("a")
	if err != nil {
		t.Fatalf("recovered key 'a' should exist: %v", err)
	}
	if string(val) != "1" {
		t.Fatalf("got %s, want 1", val)
	}

	_, err = s2.Get("b")
	if err == nil {
		t.Fatal("key 'b' should be gone after corruption truncation")
	}
}
