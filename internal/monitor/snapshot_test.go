package monitor

import (
	"testing"
	"time"
)

func TestCacheNeedsRefreshLifecycle(t *testing.T) {
	c := NewCache(30*time.Second, 5*time.Second)
	now := time.Unix(1000, 0)
	c.SetClock(func() time.Time { return now })

	if !c.NeedsRefresh("a") {
		t.Fatal("empty cache should need refresh")
	}

	if !c.MarkInflight("a") {
		t.Fatal("first MarkInflight should win")
	}
	if c.MarkInflight("a") {
		t.Fatal("second MarkInflight should fail while in flight")
	}
	if c.NeedsRefresh("a") {
		t.Fatal("in-flight alias should not need refresh")
	}

	c.Put(&Snapshot{Alias: "a"})
	if c.NeedsRefresh("a") {
		t.Fatal("fresh snapshot should not need refresh")
	}
	if !c.MarkInflight("a") {
		t.Fatal("MarkInflight should succeed after Put cleared the flag")
	}
	c.Put(&Snapshot{Alias: "a"})

	now = now.Add(45 * time.Second)
	if !c.NeedsRefresh("a") {
		t.Fatal("snapshot beyond TTL should need refresh")
	}
}

func TestCachePutSetsFetchedAt(t *testing.T) {
	c := NewCache(time.Second, time.Second)
	now := time.Unix(2000, 0)
	c.SetClock(func() time.Time { return now })
	c.Put(&Snapshot{Alias: "a"})
	snap, ok := c.Get("a")
	if !ok || !snap.FetchedAt.Equal(now) {
		t.Fatalf("expected FetchedAt=%v, got snap=%+v", now, snap)
	}
}

func TestCacheSkipMarksError(t *testing.T) {
	c := NewCache(time.Minute, time.Second)
	c.Skip("a", "password auth")
	snap, ok := c.Get("a")
	if !ok {
		t.Fatal("expected skip snapshot to exist")
	}
	if snap.Err == nil || snap.Err.Error() != "password auth" {
		t.Fatalf("expected err='password auth', got %+v", snap.Err)
	}
	if c.NeedsRefresh("a") {
		t.Fatal("skipped alias should not need refresh until TTL")
	}
}

func TestCacheAge(t *testing.T) {
	c := NewCache(time.Minute, time.Second)
	now := time.Unix(3000, 0)
	c.SetClock(func() time.Time { return now })
	snap := &Snapshot{Alias: "a", FetchedAt: now.Add(-10 * time.Second)}
	if got := c.Age(snap); got != 10*time.Second {
		t.Fatalf("age=%v, want 10s", got)
	}
	if got := c.Age(nil); got != 0 {
		t.Fatalf("age(nil)=%v, want 0", got)
	}
}
