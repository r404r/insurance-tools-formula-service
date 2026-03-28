package engine

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"sync"
)

// CacheKey uniquely identifies a computation result.
type CacheKey struct {
	FormulaID string
	Version   string
	InputHash string
}

// cacheKeyString produces a canonical string for map lookups.
func (ck CacheKey) String() string {
	return ck.FormulaID + ":" + ck.Version + ":" + ck.InputHash
}

// cacheEntry stores a cached result and its insertion order for LRU eviction.
type cacheEntry struct {
	key     string
	results map[string]Decimal
	order   uint64
}

// ResultCache is a thread-safe LRU cache for formula computation results.
type ResultCache struct {
	mu       sync.RWMutex
	items    map[string]*cacheEntry
	maxSize  int
	counter  uint64
}

// NewResultCache creates a ResultCache with the given maximum entry count.
// If maxSize <= 0, it defaults to 1000.
func NewResultCache(maxSize int) *ResultCache {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &ResultCache{
		items:   make(map[string]*cacheEntry, maxSize),
		maxSize: maxSize,
	}
}

// Get retrieves a cached result for the given key. Returns the result map
// and true on a cache hit, or nil and false on a miss.
func (rc *ResultCache) Get(key CacheKey) (map[string]Decimal, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	entry, ok := rc.items[key.String()]
	if !ok {
		return nil, false
	}

	// Return a copy to prevent callers from mutating cached data.
	result := make(map[string]Decimal, len(entry.results))
	for k, v := range entry.results {
		result[k] = v
	}
	return result, true
}

// Set stores a computation result in the cache. If the cache is at capacity,
// the oldest entry is evicted.
func (rc *ResultCache) Set(key CacheKey, results map[string]Decimal) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	k := key.String()

	// If key already exists, update it.
	if entry, ok := rc.items[k]; ok {
		entry.results = copyResults(results)
		rc.counter++
		entry.order = rc.counter
		return
	}

	// Evict oldest if at capacity.
	if len(rc.items) >= rc.maxSize {
		rc.evictOldest()
	}

	rc.counter++
	rc.items[k] = &cacheEntry{
		key:     k,
		results: copyResults(results),
		order:   rc.counter,
	}
}

// evictOldest removes the entry with the smallest order value. Caller must
// hold the write lock.
func (rc *ResultCache) evictOldest() {
	var oldestKey string
	var oldestOrder uint64 = ^uint64(0) // max uint64

	for k, entry := range rc.items {
		if entry.order < oldestOrder {
			oldestOrder = entry.order
			oldestKey = k
		}
	}

	if oldestKey != "" {
		delete(rc.items, oldestKey)
	}
}

// Len returns the current number of entries in the cache.
func (rc *ResultCache) Len() int {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return len(rc.items)
}

// Clear removes all entries from the cache.
func (rc *ResultCache) Clear() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.items = make(map[string]*cacheEntry, rc.maxSize)
	rc.counter = 0
}

// ComputeInputHash produces a deterministic SHA-256 hex digest of the input
// map. Keys are sorted lexicographically to ensure consistency.
func ComputeInputHash(inputs map[string]Decimal) string {
	keys := make([]string, 0, len(inputs))
	for k := range inputs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		fmt.Fprintf(h, "%s=%s;", k, inputs[k].String())
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// copyResults creates a shallow copy of a results map.
func copyResults(m map[string]Decimal) map[string]Decimal {
	c := make(map[string]Decimal, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}
