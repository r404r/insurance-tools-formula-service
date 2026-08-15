package engine

import "sync"

// resultCacheCoordinator gives the decimal and tagged-value result caches one
// shared entry budget. The two caches keep different result representations,
// but EngineConfig.CacheSize is a limit for the engine as a whole rather than
// a limit per representation.
type resultCacheCoordinator struct {
	mu      sync.Mutex
	maxSize int
	entries map[string]resultCacheKind
	order   []string
}

type resultCacheKind uint8

const (
	decimalResultCache resultCacheKind = iota
	typedResultCacheKind
)

func newResultCacheCoordinator(maxSize int) *resultCacheCoordinator {
	return &resultCacheCoordinator{
		maxSize: maxSize,
		entries: make(map[string]resultCacheKind, maxSize),
	}
}

// prepareInsert records a new or replaced entry. The caller must hold mu. If
// inserting a new entry would exceed the shared budget, it returns the oldest
// entry so its owning cache can remove it before the new result is stored.
func (c *resultCacheCoordinator) prepareInsert(key string, kind resultCacheKind) (evictedKey string, evictedKind resultCacheKind, evicted bool) {
	if _, exists := c.entries[key]; exists {
		c.touch(key)
		return "", 0, false
	}
	if len(c.entries) >= c.maxSize {
		evictedKey = c.order[0]
		evictedKind = c.entries[evictedKey]
		delete(c.entries, evictedKey)
		c.order = c.order[1:]
		evicted = true
	}
	c.entries[key] = kind
	c.order = append(c.order, key)
	return evictedKey, evictedKind, evicted
}

func (c *resultCacheCoordinator) touch(key string) {
	for i, existing := range c.order {
		if existing != key {
			continue
		}
		copy(c.order[i:], c.order[i+1:])
		c.order[len(c.order)-1] = key
		return
	}
}

func (c *resultCacheCoordinator) clear() {
	c.entries = make(map[string]resultCacheKind, c.maxSize)
	c.order = nil
}
