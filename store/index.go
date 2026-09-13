package store

import (
	"sync"
)

type EntryLoc struct {
	FileID  uint64
	Offset  int64
	Size    int
	Deleted bool
}

type Index struct {
	mu   sync.RWMutex
	data map[string]*EntryLoc
}

func NewIndex() *Index {
	return &Index{data: make(map[string]*EntryLoc)}
}

func (i *Index) Put(key string, loc *EntryLoc) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.data[key] = loc
}

func (i *Index) Get(key string) (*EntryLoc, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	loc, ok := i.data[key]
	return loc, ok
}

func (i *Index) Delete(key string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.data, key)
}

func (i *Index) Has(key string) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	_, ok := i.data[key]
	return ok
}

func (i *Index) Size() int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return len(i.data)
}

func (i *Index) Entries() map[string]*EntryLoc {
	i.mu.RLock()
	defer i.mu.RUnlock()
	clone := make(map[string]*EntryLoc, len(i.data))
	for k, v := range i.data {
		clone[k] = v
	}
	return clone
}
