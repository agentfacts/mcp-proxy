package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"time"
)

// DecisionCache provides multi-tier caching for policy decisions.
type DecisionCache struct {
	// L2 cache - session-scoped, longer TTL
	l2Cache map[string]*cacheEntry
	l2Mu    sync.RWMutex
	l2TTL   time.Duration

	// Configuration
	maxEntries int
	enabled    bool

	// Metrics
	l1Hits  int64
	l2Hits  int64
	misses  int64
	evicted int64
}

type cacheEntry struct {
	decision  *PolicyDecision
	expiresAt time.Time
}

// CacheConfig holds cache configuration.
type CacheConfig struct {
	Enabled    bool
	TTL        time.Duration
	MaxEntries int
}

// NewDecisionCache creates a new decision cache.
func NewDecisionCache(cfg CacheConfig) *DecisionCache {
	if cfg.TTL == 0 {
		cfg.TTL = 5 * time.Minute
	}
	if cfg.MaxEntries == 0 {
		cfg.MaxEntries = 10000
	}

	c := &DecisionCache{
		l2Cache:    make(map[string]*cacheEntry),
		l2TTL:      cfg.TTL,
		maxEntries: cfg.MaxEntries,
		enabled:    cfg.Enabled,
	}

	// Start background cleanup
	if cfg.Enabled {
		go c.cleanupLoop()
	}

	return c
}

// Get retrieves a cached decision.
func (c *DecisionCache) Get(key string) (*PolicyDecision, bool, string) {
	if !c.enabled {
		return nil, false, ""
	}

	// Check L2 cache
	c.l2Mu.RLock()
	entry, ok := c.l2Cache[key]
	c.l2Mu.RUnlock()

	if ok && time.Now().Before(entry.expiresAt) {
		c.l2Hits++
		return entry.decision, true, "L2"
	}

	c.misses++
	return nil, false, ""
}

// Set stores a decision in the cache.
func (c *DecisionCache) Set(key string, decision *PolicyDecision) {
	if !c.enabled {
		return
	}

	c.l2Mu.Lock()
	defer c.l2Mu.Unlock()

	// Evict if at capacity
	if len(c.l2Cache) >= c.maxEntries {
		c.evictOldest()
	}

	c.l2Cache[key] = &cacheEntry{
		decision:  decision,
		expiresAt: time.Now().Add(c.l2TTL),
	}
}

// Invalidate removes all cached entries (e.g., on policy reload).
func (c *DecisionCache) Invalidate() {
	if !c.enabled {
		return
	}

	c.l2Mu.Lock()
	c.l2Cache = make(map[string]*cacheEntry)
	c.l2Mu.Unlock()
}

// ComputeKey generates a cache key from the policy input.
// Key format: agent_id:tool:capabilities_hash
func (c *DecisionCache) ComputeKey(input *PolicyInput) string {
	// Sort capabilities for consistent hashing
	caps := make([]string, len(input.Agent.Capabilities))
	copy(caps, input.Agent.Capabilities)
	sort.Strings(caps)

	capsHash := hashString(strings.Join(caps, ","))

	return input.Agent.ID + ":" + input.Request.Tool + ":" + capsHash[:8]
}

// Stats returns cache statistics.
func (c *DecisionCache) Stats() CacheStats {
	c.l2Mu.RLock()
	entries := len(c.l2Cache)
	c.l2Mu.RUnlock()

	total := c.l1Hits + c.l2Hits + c.misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(c.l1Hits+c.l2Hits) / float64(total)
	}

	return CacheStats{
		L1Hits:  c.l1Hits,
		L2Hits:  c.l2Hits,
		Misses:  c.misses,
		Entries: entries,
		HitRate: hitRate,
		Evicted: c.evicted,
	}
}

// CacheStats contains cache performance statistics.
type CacheStats struct {
	L1Hits  int64
	L2Hits  int64
	Misses  int64
	Entries int
	HitRate float64
	Evicted int64
}

// evictOldest removes the oldest entries to make room.
func (c *DecisionCache) evictOldest() {
	// Simple eviction: remove expired entries first
	now := time.Now()
	for key, entry := range c.l2Cache {
		if now.After(entry.expiresAt) {
			delete(c.l2Cache, key)
			c.evicted++
		}
	}

	// If still over capacity, remove oldest 10%
	if len(c.l2Cache) >= c.maxEntries {
		toRemove := c.maxEntries / 10
		removed := 0
		for key := range c.l2Cache {
			delete(c.l2Cache, key)
			c.evicted++
			removed++
			if removed >= toRemove {
				break
			}
		}
	}
}

// cleanupLoop periodically removes expired entries.
func (c *DecisionCache) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup removes expired entries.
func (c *DecisionCache) cleanup() {
	c.l2Mu.Lock()
	defer c.l2Mu.Unlock()

	now := time.Now()
	for key, entry := range c.l2Cache {
		if now.After(entry.expiresAt) {
			delete(c.l2Cache, key)
			c.evicted++
		}
	}
}

// hashString returns a SHA256 hash of the input string.
func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
