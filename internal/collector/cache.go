package collector

import (
	"strings"
	"sync"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// maxCacheEntries bounds the number of cached FetchHistory results to prevent
// unbounded memory growth. When exceeded, the oldest entry is evicted.
const maxCacheEntries = 256

// cacheEntry holds a cached FetchHistory result together with the time it was
// stored (for TTL) and a monotonic sequence number (for oldest-first eviction).
type cacheEntry struct {
	data     []core.OHLCV
	storedAt time.Time
	seq      uint64
}

// CachedCollector decorates any Collector with a TTL cache for FetchHistory.
// All other Collector methods are transparently delegated to the embedded
// collector. FetchQuote is intentionally not cached (real-time freshness).
//
// Cache semantics:
//   - key = symbol|start|end|interval, with start/end truncated to the minute.
//   - A hit within ttl returns a copy of the cached slice so callers cannot
//     mutate the shared backing array.
//   - Errors from the underlying collector are never cached.
//   - At most maxCacheEntries entries are retained; the oldest is evicted.
//   - Safe for concurrent use.
type CachedCollector struct {
	Collector // embedded: passes through Name/SupportedMarkets/Init/Start/Stop/FetchQuote

	ttl     time.Duration
	mu      sync.Mutex
	entries map[string]cacheEntry
	seq     uint64
}

// NewCached wraps c with a TTL cache for FetchHistory.
func NewCached(c Collector, ttl time.Duration) *CachedCollector {
	return &CachedCollector{
		Collector: c,
		ttl:       ttl,
		entries:   make(map[string]cacheEntry),
	}
}

// FetchHistory returns a cached copy when a fresh entry exists, otherwise it
// delegates to the underlying collector and caches the (successful) result.
func (c *CachedCollector) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	key := cacheKey(symbol, start, end, interval)

	c.mu.Lock()
	if e, ok := c.entries[key]; ok && time.Since(e.storedAt) < c.ttl {
		out := cloneOHLCV(e.data)
		c.mu.Unlock()
		return out, nil
	}
	c.mu.Unlock()

	data, err := c.Collector.FetchHistory(symbol, start, end, interval)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.store(key, data)
	c.mu.Unlock()

	return cloneOHLCV(data), nil
}

// store inserts an entry, evicting the oldest one if at capacity.
// Caller must hold c.mu.
func (c *CachedCollector) store(key string, data []core.OHLCV) {
	if _, exists := c.entries[key]; !exists && len(c.entries) >= maxCacheEntries {
		c.evictOldest()
	}
	c.seq++
	c.entries[key] = cacheEntry{
		data:     cloneOHLCV(data),
		storedAt: time.Now(),
		seq:      c.seq,
	}
}

// evictOldest removes the entry with the smallest sequence number.
// Caller must hold c.mu. Every stored entry has a non-zero seq, so an
// oldestSeq of 0 means the map was empty and there is nothing to evict.
func (c *CachedCollector) evictOldest() {
	var oldestKey string
	var oldestSeq uint64
	for k, e := range c.entries {
		if oldestSeq == 0 || e.seq < oldestSeq {
			oldestKey, oldestSeq = k, e.seq
		}
	}
	if oldestSeq != 0 {
		delete(c.entries, oldestKey)
	}
}

// entryCount returns the current number of cached entries (test helper).
func (c *CachedCollector) entryCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// cacheKey builds the cache key, truncating times to the minute so that
// sub-minute differences share a cache slot.
func cacheKey(symbol string, start, end time.Time, interval string) string {
	var b strings.Builder
	b.WriteString(symbol)
	b.WriteByte('|')
	b.WriteString(start.Truncate(time.Minute).UTC().Format(time.RFC3339))
	b.WriteByte('|')
	b.WriteString(end.Truncate(time.Minute).UTC().Format(time.RFC3339))
	b.WriteByte('|')
	b.WriteString(interval)
	return b.String()
}

// cloneOHLCV returns an independent copy of the slice. OHLCV is a flat value
// type, so a shallow element copy is a deep copy.
func cloneOHLCV(in []core.OHLCV) []core.OHLCV {
	if in == nil {
		return nil
	}
	out := make([]core.OHLCV, len(in))
	copy(out, in)
	return out
}
