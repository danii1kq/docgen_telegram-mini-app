package api

import (
	"sync"
	"testing"
	"time"
)

// TestLimiterWindow checks the fixed-window accounting.
func TestLimiterWindow(t *testing.T) {
	l := newLimiter()
	for i := 0; i < 3; i++ {
		allowed, _ := l.allow("promo|1.2.3.4", 3, time.Minute)
		if !allowed {
			t.Fatalf("request %d rejected, want allowed", i+1)
		}
	}
	if allowed, retry := l.allow("promo|1.2.3.4", 3, time.Minute); allowed || retry <= 0 {
		t.Fatalf("fourth request: allowed=%v retry=%d, want rejected with a Retry-After", allowed, retry)
	}
	// Another key has its own window.
	if allowed, _ := l.allow("orders|1.2.3.4", 3, time.Minute); !allowed {
		t.Error("separate bucket must not be affected")
	}
}

// TestLimiterResets checks that the window reopens after its size.
func TestLimiterResets(t *testing.T) {
	l := newLimiter()
	if allowed, _ := l.allow("k", 1, 20*time.Millisecond); !allowed {
		t.Fatal("first request rejected")
	}
	if allowed, _ := l.allow("k", 1, 20*time.Millisecond); allowed {
		t.Fatal("second request accepted inside the window")
	}
	time.Sleep(30 * time.Millisecond)
	if allowed, _ := l.allow("k", 1, 20*time.Millisecond); !allowed {
		t.Fatal("window did not reset")
	}
}

// TestLimiterConcurrent checks the mutex under parallel load: exactly `limit`
// requests pass per window.
func TestLimiterConcurrent(t *testing.T) {
	l := newLimiter()
	const limit = 25
	const workers = 100

	var mu sync.Mutex
	passed := 0
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, _ := l.allow("k", limit, time.Minute)
			if allowed {
				mu.Lock()
				passed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if passed != limit {
		t.Fatalf("passed = %d, want exactly %d", passed, limit)
	}
}
