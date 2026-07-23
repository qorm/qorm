package expr

// Tests for the two bounded caches: the AST parse cache (astCache/astCount,
// capped at maxASTCache) and the compiled-regex cache (reCache/reCount, capped
// at 1024). Both guard against a pathological app growing memory without bound
// while still evaluating correctly once saturated.

import (
	"strconv"
	"sync"
	"testing"
)

// TestASTCacheConsistency verifies repeated evaluation of the same source is
// stable for both successful parses and cached parse errors.
func TestASTCacheConsistency(t *testing.T) {
	ctx := map[string]any{"x": 5.0}

	v1, err1 := Eval("x * 2 + 1", ctx)
	v2, err2 := Eval("x * 2 + 1", ctx) // cache hit
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v, %v", err1, err2)
	}
	if v1 != v2 || v1 != 11.0 {
		t.Fatalf("cached results differ: %v vs %v (want 11)", v1, v2)
	}

	// A parse error is cached and returned consistently on the second call.
	_, e1 := Eval("1 +", ctx)
	_, e2 := Eval("1 +", ctx)
	if e1 == nil || e2 == nil {
		t.Fatalf("expected cached parse errors, got %v / %v", e1, e2)
	}
}

// TestASTCacheBound fills the cache far past its cap with distinct sources and
// asserts the entry count never exceeds maxASTCache, and that evaluation keeps
// working (via the uncached path) after saturation.
func TestASTCacheBound(t *testing.T) {
	ctx := map[string]any{"x": 1.0}

	const extra = maxASTCache + 1000
	for i := 0; i < extra; i++ {
		src := "x + " + strconv.Itoa(i)
		if v, err := Eval(src, ctx); err != nil || v != 1.0+float64(i) {
			t.Fatalf("Eval(%q) = %v, %v", src, v, err)
		}
	}

	if got := astCount.Load(); got > maxASTCache {
		t.Fatalf("astCount = %d, exceeds bound %d", got, maxASTCache)
	}
	if got := astCount.Load(); got != maxASTCache {
		// We inserted well over the cap of distinct sources, so the cache must
		// have saturated exactly at the bound.
		t.Fatalf("astCount = %d, want saturation at %d", got, maxASTCache)
	}

	// Post-saturation: a brand-new source is still evaluated correctly even
	// though it can no longer be stored.
	if v, err := Eval("x + 987654321", ctx); err != nil || v != 1.0+987654321 {
		t.Fatalf("post-saturation Eval = %v, %v", v, err)
	}
}

// TestASTCacheConcurrent hammers the cache from many goroutines to confirm the
// sync.Map/atomic bookkeeping is race-free (run with -race) and stays near the
// bound. A small slack over the cap is permitted because the store decision is
// a check-then-act and the bound is intentionally soft under concurrency.
func TestASTCacheConcurrent(t *testing.T) {
	ctx := map[string]any{"x": 1.0}
	const goroutines = 32
	const iters = 200

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				n := i % 50
				src := "x + " + strconv.Itoa(n)
				v, err := Eval(src, ctx)
				if err != nil || v != 1.0+float64(n) {
					t.Errorf("Eval(%q) = %v, %v", src, v, err)
					return
				}
			}
		}()
	}
	wg.Wait()

	if got := astCount.Load(); got > maxASTCache+goroutines {
		t.Fatalf("concurrent astCount = %d, blew past soft bound %d", got, maxASTCache+goroutines)
	}
}

// TestRegexCache covers the compile-once reuse, the cached-nil path for an
// invalid pattern, and the 1024-entry bound.
func TestRegexCache(t *testing.T) {
	// A valid pattern is compiled once and reused (pointer identity).
	re1 := compileCached(`^\d+$`)
	re2 := compileCached(`^\d+$`)
	if re1 == nil || re1 != re2 {
		t.Fatalf("expected cached regex reuse, got %p and %p", re1, re2)
	}
	if !re1.MatchString("123") || re1.MatchString("abc") {
		t.Fatalf("cached regex matches incorrectly")
	}

	// An invalid pattern compiles to nil and stays nil on a cached repeat.
	if re := compileCached(`(`); re != nil {
		t.Fatalf("invalid pattern: want nil, got %v", re)
	}
	if re := compileCached(`(`); re != nil {
		t.Fatalf("cached invalid pattern: want nil, got %v", re)
	}

	// Many distinct patterns never exceed the cap.
	for i := 0; i < 1100; i++ {
		if re := compileCached("^p" + strconv.Itoa(i) + "$"); re == nil {
			t.Fatalf("valid pattern %d compiled to nil", i)
		}
	}
	if got := reCount.Load(); got > 1024 {
		t.Fatalf("reCount = %d, exceeds cap 1024", got)
	}
}

// TestMatchesInvalidRegex drives the matches() builtin's nil-regex branch
// through the public Eval API: a bad pattern yields false (never a panic), and
// a valid pattern still matches.
func TestMatchesInvalidRegex(t *testing.T) {
	ctx := map[string]any{"s": "anything"}

	if v, err := Eval(`matches(s, '[')`, ctx); err != nil || v != false {
		t.Errorf("invalid regex: got %v, %v; want false, nil", v, err)
	}
	// Repeat to exercise the cached-nil lookup path.
	if v, _ := Eval(`matches(s, '[')`, ctx); v != false {
		t.Errorf("cached invalid regex: got %v, want false", v)
	}
	if v, _ := Eval(`matches(s, 'any')`, ctx); v != true {
		t.Errorf("valid regex: got %v, want true", v)
	}
	// A pattern that compiles but does not match returns false.
	if v, _ := Eval(`matches(s, '^zzz$')`, ctx); v != false {
		t.Errorf("non-matching regex: got %v, want false", v)
	}
}
