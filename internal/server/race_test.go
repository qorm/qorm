package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/qorm/qorm/internal/keys"
)

// TestServeIndexEventNoRace hammers serveIndex (GET /) and serveEvent
// (POST /event) concurrently. Under `-race` it guards the fix for reading
// rt.State (locale/theme/rtl, via Page) outside the lock while an event
// dispatch writes it — a concurrent map read+write that crashed the process.
func TestServeIndexEventNoRace(t *testing.T) {
	ts := httptest.NewServer(counterServer(t).Handler())
	defer ts.Close()

	// Prime the handler table and pick up the human event token — without it
	// serveEvent rejects the POST before touching state and the race guard is
	// silently neutered.
	token := pageEventToken(t, ts.URL)

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if resp, err := http.Get(ts.URL + "/"); err == nil {
				resp.Body.Close()
			}
		}()
		go func() {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodPost, ts.URL+"/event", strings.NewReader(`{"h":1,"inputs":{}}`))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Qorm-Token", token)
			if resp, err := http.DefaultClient.Do(req); err == nil {
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()
}

// TestServeUpdateCurrentNoRace hammers /update and /rollback concurrently.
// Under `-race` it guards the fix for serveUpdate reading s.current without
// holding s.mu while activate() (a competing /update) and Rollback() write it
// under s.mu — serveUpdate must snapshot the OTA gate state under the lock.
func TestServeUpdateCurrentNoRace(t *testing.T) {
	pub, priv, _ := keys.Generate()
	s, err := NewBundle(signedBundle(t, "1.0.0", priv, pub), pub, nil)
	if err != nil {
		t.Fatalf("NewBundle: %v", err)
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	srcV2, _ := json.Marshal(map[string]string{"source": writeBundle(t, signedBundle(t, "2.0.0", priv, pub))})

	// POST-and-drain helpers that ignore the outcome: every status is legal
	// here (200 applied, 409 kept-current / nothing to roll back). The point
	// is concurrent access; the assertions come from the race detector.
	update := func(body string) {
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/update", strings.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if resp, err := http.DefaultClient.Do(req); err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}
	rollback := func() {
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/rollback", strings.NewReader(""))
		if err != nil {
			return
		}
		if resp, err := http.DefaultClient.Do(req); err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); update(string(srcV2)) }()          // activate() writes s.current
		go func() { defer wg.Done(); rollback() }()                     // Rollback() writes s.current
		go func() { defer wg.Done(); update(`{"source":"missing"}`) }() // serveUpdate reads s.current
	}
	wg.Wait()
}
