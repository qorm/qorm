//go:build desktop

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/qorm/qorm/internal/measure"
	qrt "github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/server"
	webview "github.com/qorm/qorm/internal/webview"
)

// measureRows renders appDir in a WebView, waits for the app to self-report its
// layout, and returns the raw measured rows + the runtime (for intent joining).
// The provided step callback runs once measurement is captured, then the window
// closes.
func measureRows(appDir string, width int, use func(rt *qrt.Runtime, url string, measured []byte)) error {
	rt, err := loadRuntime(appDir, "", "")
	if err != nil {
		return err
	}
	srv := server.New(rt)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String() + "/"
	go func() { _ = http.Serve(ln, srv.Handler()) }()

	runtime.LockOSThread()
	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle("qorm measure")
	w.SetSize(width, 820, webview.HintNone)
	w.Navigate(url)
	go func() {
		var measured []byte
		for i := 0; i < 120; i++ {
			time.Sleep(80 * time.Millisecond)
			resp, e := http.Get(url + "measure")
			if e != nil {
				continue
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if len(b) > 2 {
				measured = b
				break
			}
		}
		use(rt, url, measured)
		w.Terminate()
	}()
	w.Run()
	return nil
}

// runMeasure prints the complete intent+result report.
func runMeasure(appDir, out string, width int) error {
	return measureRows(appDir, width, func(rt *qrt.Runtime, _ string, measured []byte) {
		report, _ := measure.Report(rt, measured)
		if out != "" {
			_ = os.WriteFile(out, report, 0o644)
			fmt.Fprintf(os.Stderr, "measured %s -> %s\n", appDir, out)
		} else {
			fmt.Println(string(report))
		}
	})
}

// runCheck measures the app and evaluates the checks against the rendered
// reality, printing a precise pass/fail report.
func runCheck(appDir, checksPath, out string, audit bool, width int) error {
	var checks []byte
	if !audit {
		var err error
		checks, err = os.ReadFile(checksPath)
		if err != nil {
			return err
		}
	}
	var rerr error
	e := measureRows(appDir, width, func(rt *qrt.Runtime, url string, measured []byte) {
		var report []byte
		if audit {
			report, rerr = measure.Audit(rt, measured)
		} else if isFlow(checks) {
			report, rerr = evalFlow(rt, url, checks)
		} else {
			report, rerr = measure.Eval(rt, measured, checks)
		}
		if rerr != nil {
			return
		}
		if out != "" {
			_ = os.WriteFile(out, report, 0o644)
			fmt.Fprintf(os.Stderr, "checked %s -> %s\n", appDir, out)
		} else {
			fmt.Println(string(report))
		}
	})
	if e != nil {
		return e
	}
	return rerr
}

// isFlow reports whether the checks JSON is a step-flow object ({"steps":[…]})
// rather than a flat array of static checks.
func isFlow(b []byte) bool {
	t := bytesTrimLeadingSpace(b)
	return len(t) > 0 && t[0] == '{'
}

func bytesTrimLeadingSpace(b []byte) []byte {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\n' || b[i] == '\t' || b[i] == '\r') {
		i++
	}
	return b[i:]
}

func postMCP(url, tool string, args map[string]any) {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args}})
	resp, err := http.Post(url+"mcp", "application/json", bytesReader(body))
	if err == nil {
		resp.Body.Close()
	}
}

func httpGetBytes(u string) []byte {
	resp, err := http.Get(u)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return b
}

// evalFlow applies each step's action to the live app, waits for the re-render
// and re-measure, then evaluates that step's checks — verifying interactions.
func evalFlow(rt *qrt.Runtime, url string, checksJSON []byte) ([]byte, error) {
	var flow struct {
		Steps []struct {
			Name string `json:"name"`
			Do   struct {
				SetState *struct {
					Path  string `json:"path"`
					Value any    `json:"value"`
				} `json:"setState"`
				Dispatch string         `json:"dispatch"`
				Args     map[string]any `json:"args"`
			} `json:"do"`
			Checks []map[string]any `json:"checks"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(checksJSON, &flow); err != nil {
		return nil, fmt.Errorf("bad flow JSON: %w", err)
	}
	var steps []map[string]any
	allPass := true
	for i, st := range flow.Steps {
		action := "(none)"
		if st.Do.SetState != nil {
			postMCP(url, "qorm_set_state", map[string]any{"path": st.Do.SetState.Path, "value": st.Do.SetState.Value})
			action = fmt.Sprintf("set_state %s=%v", st.Do.SetState.Path, st.Do.SetState.Value)
		} else if st.Do.Dispatch != "" {
			postMCP(url, "qorm_dispatch", map[string]any{"action": st.Do.Dispatch, "args": st.Do.Args})
			action = "dispatch " + st.Do.Dispatch
		}
		// wait until the app re-measures after the morph (poll until it changes)
		before := httpGetBytes(url + "measure")
		measured := before
		for j := 0; j < 25; j++ {
			time.Sleep(80 * time.Millisecond)
			now := httpGetBytes(url + "measure")
			if len(now) > 2 && !bytes.Equal(now, before) {
				measured = now
				break
			}
			measured = now
		}
		cb, _ := json.Marshal(st.Checks)
		rep, err := measure.Eval(rt, measured, cb)
		if err != nil {
			return nil, err
		}
		var rd map[string]any
		json.Unmarshal(rep, &rd)
		if ok, _ := rd["ok"].(bool); !ok {
			allPass = false
		}
		steps = append(steps, map[string]any{"step": i + 1, "name": st.Name, "action": action, "result": rd})
	}
	return json.MarshalIndent(map[string]any{"app": rt.App.Name, "ok": allPass, "steps": steps}, "", "  ")
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

// runPreview serves a packaged (static, offline) app directory and loads it in
// the WebView — the app boots its WASM runtime and renders client-side with no
// server. It captures the app's self-measurement (POST /measure) plus, if an
// eval is given, runs it (e.g. "qorm(0)") to exercise interactivity, then
// writes the measurement to out and closes.
func runPreview(dir string, width int, eval, out string) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	var mu sync.Mutex
	var measured []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/measure", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		mu.Lock()
		measured = b
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("/", http.FileServer(http.Dir(dir)))
	url := "http://" + ln.Addr().String() + "/"
	go func() { _ = http.Serve(ln, mux) }()

	runtime.LockOSThread()
	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle("qorm preview")
	w.SetSize(width, 820, webview.HintNone)
	w.Navigate(url)
	go func() {
		read := func() []byte { mu.Lock(); defer mu.Unlock(); return measured }
		for i := 0; i < 100 && len(read()) <= 2; i++ {
			time.Sleep(80 * time.Millisecond)
		}
		if eval != "" {
			mu.Lock()
			measured = nil
			mu.Unlock()
			w.Dispatch(func() { w.Eval(eval) })
			for i := 0; i < 100 && len(read()) <= 2; i++ {
				time.Sleep(80 * time.Millisecond)
			}
		}
		m := read()
		if out != "" {
			_ = os.WriteFile(out, m, 0o644)
			fmt.Fprintf(os.Stderr, "preview measured -> %s (%d bytes)\n", out, len(m))
		} else {
			fmt.Println(string(m))
		}
		w.Terminate()
	}()
	w.Run()
	return nil
}
