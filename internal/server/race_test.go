package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
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
