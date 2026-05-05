package monitor

import (
	"errors"
	"sync"
	"time"
)

type Snapshot struct {
	Alias     string
	FetchedAt time.Time
	Uptime    string
	Load      string
	Mem       string
	Disk      string
	TopProcs  string
	Conns     string
	Err       error
}

type Cache struct {
	ttl     time.Duration
	timeout time.Duration

	mu       sync.RWMutex
	now      func() time.Time
	snaps    map[string]*Snapshot
	inflight map[string]struct{}
}

func NewCache(ttl, timeout time.Duration) *Cache {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Cache{
		ttl:      ttl,
		timeout:  timeout,
		now:      time.Now,
		snaps:    map[string]*Snapshot{},
		inflight: map[string]struct{}{},
	}
}

func (c *Cache) TTL() time.Duration     { return c.ttl }
func (c *Cache) Timeout() time.Duration { return c.timeout }

func (c *Cache) Get(alias string) (*Snapshot, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	snap, ok := c.snaps[alias]
	return snap, ok
}

func (c *Cache) Age(snap *Snapshot) time.Duration {
	if snap == nil || snap.FetchedAt.IsZero() {
		return 0
	}
	c.mu.RLock()
	now := c.now
	c.mu.RUnlock()
	return now().Sub(snap.FetchedAt)
}

func (c *Cache) NeedsRefresh(alias string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if _, busy := c.inflight[alias]; busy {
		return false
	}
	snap, ok := c.snaps[alias]
	if !ok {
		return true
	}
	return c.now().Sub(snap.FetchedAt) >= c.ttl
}

// MarkInflight returns true if the caller acquired the in-flight lock.
// Returns false if another goroutine already owns it for this alias.
func (c *Cache) MarkInflight(alias string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, busy := c.inflight[alias]; busy {
		return false
	}
	c.inflight[alias] = struct{}{}
	return true
}

func (c *Cache) Put(snap *Snapshot) {
	if snap == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if snap.FetchedAt.IsZero() {
		snap.FetchedAt = c.now()
	}
	c.snaps[snap.Alias] = snap
	delete(c.inflight, snap.Alias)
}

// Skip records a sentinel snapshot so the same alias is not retried until TTL elapses.
func (c *Cache) Skip(alias, reason string) {
	c.Put(&Snapshot{Alias: alias, Err: errors.New(reason)})
}

// SetClock injects a deterministic clock for tests.
func (c *Cache) SetClock(now func() time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if now != nil {
		c.now = now
	}
}
