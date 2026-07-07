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

	// Prime the handler table so POST /event has a handler to fire.
	if resp, err := http.Get(ts.URL + "/"); err == nil {
		resp.Body.Close()
	}

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
			if resp, err := http.Post(ts.URL+"/event", "application/json", strings.NewReader(`{"h":1,"inputs":{}}`)); err == nil {
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()
}
