// Package server serves a live QORM app over HTTP. Button presses POST to
// /event; the server updates state, dispatches the action, re-renders, and
// returns the new body HTML which a tiny inline script swaps in. No cgo, no
// external deps — so the binary cross-compiles to every platform cleanly.
package server

import (
	"crypto/ed25519"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qorm/qorm/internal/bundle"
	"github.com/qorm/qorm/internal/mcp"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/ota"
	"github.com/qorm/qorm/internal/render"
	"github.com/qorm/qorm/internal/runtime"
)

// Server is an HTTP handler wrapping a live runtime. It serves the browser UI,
// hot OTA updates with rollback, and an MCP endpoint over which an agent shares
// the *same* live app session — so an AI's edits appear in a human's browser
// live, and vice-versa.
type Server struct {
	mu       sync.Mutex
	rt       *runtime.Runtime
	handlers []render.Handler
	rev      atomic.Int64 // bumped on every mutation; drives browser live-sync
	agent    *mcp.Server  // MCP handler sharing rt + mu

	subsMu sync.Mutex               // guards subs
	subs   map[chan string]struct{} // SSE subscribers, each gets pushed updates

	// Collaboration activity log: who (human/agent) did what, newest last.
	actMu    sync.Mutex
	activity []LogEntry
	actSeq   int
	lastSrc  string // source of the most recent event (for live edit attribution)
	lastDet  string // its short detail

	// What the human is currently attending to (the focused / last-touched element),
	// surfaced to the agent via qorm_activity so it collaborates in context.
	humanFocus   string
	humanFocusAt time.Time
	// The human's last non-empty text entry, retained even after focus moves on (a
	// button tap must not erase what they just typed). Never a password value.
	humanTyping   string
	humanTypingAt time.Time
	// A hidden (password) field the human filled, retained by label only — the
	// value is never captured, but the agent may know the form is complete.
	humanFilled   string
	humanFilledAt time.Time

	measureMu sync.Mutex
	measure   []byte // latest self-reported layout (rects + key styles)

	// OTA state (populated when started from a bundle).
	trust   ed25519.PublicKey
	revoked bundle.RevocationList
	current *bundle.Bundle // active, verified bundle
	prev    *bundle.Bundle // last-good bundle for rollback

	// Window control (set by a native desktop host): the control engine (debug
	// window / agent) drives the app's native window.
	WindowMover func(id string, x, y, w, h int) // move + resize a window
	WindowOp    func(id, op string)             // focus/minimize/pin/unpin/close
	WindowOpen  func(id, url string, w, h int)  // open a secondary window
	WindowEval  func(id, js string)             // push JS to a window (window-to-window comms)
}

// New builds a server for a runtime (no OTA).
func New(rt *runtime.Runtime) *Server {
	s := &Server{rt: rt}
	s.initAgent()
	return s
}

// NewBundle builds a server from a verified bundle, enabling OTA updates
// against the given trusted key (nil = integrity-only) and revocation list.
func NewBundle(b *bundle.Bundle, trust ed25519.PublicKey, revoked bundle.RevocationList) *Server {
	s := &Server{rt: runtime.New(b.ToApp()), current: b, trust: trust, revoked: revoked}
	s.initAgent()
	return s
}

// initAgent (re)binds the shared MCP handler to the current runtime. Called on
// construction and whenever the runtime is swapped (OTA). afterMutate runs
// while the agent holds s.mu, so bump() must not re-take s.mu.
func (s *Server) initAgent() {
	s.agent = mcp.NewShared(s.rt, &s.mu, func() { s.bump() })
	s.agent.SetMeasureProvider(func() []byte {
		s.measureMu.Lock()
		defer s.measureMu.Unlock()
		return s.measure
	})
	// Let the agent read the shared activity log, so it can see what the human
	// just did in the live app and respond — the reverse of the human's "AI
	// edited" toast.
	s.agent.SetActivityProvider(func() string {
		s.actMu.Lock()
		defer s.actMu.Unlock()
		out := map[string]any{"events": s.activity}
		if s.humanFocus != "" {
			out["humanFocus"] = map[string]any{
				"element":    s.humanFocus,
				"secondsAgo": int(time.Since(s.humanFocusAt).Seconds()),
			}
		}
		if s.humanTyping != "" {
			out["humanTyping"] = map[string]any{
				"entry":      s.humanTyping,
				"secondsAgo": int(time.Since(s.humanTypingAt).Seconds()),
			}
		}
		if s.humanFilled != "" {
			out["humanFilled"] = map[string]any{
				"field":      s.humanFilled, // a hidden field they filled; value NOT captured
				"secondsAgo": int(time.Since(s.humanFilledAt).Seconds()),
			}
		}
		b, _ := json.Marshal(out)
		return string(b)
	})
}

// bump increments the revision, re-renders, refreshes the handler table and
// pushes the new UI to all SSE subscribers. Caller must hold s.mu.
func (s *Server) bump() (int64, string, string) {
	rev := s.rev.Add(1)
	res := render.RenderScene(s.rt, s.rt.CurrentScene())
	s.handlers = res.Handlers
	nav := s.rt.TakeNavDir()
	s.broadcast(rev, res.HTML, nav)
	return rev, res.HTML, nav
}

// broadcast pushes a revision+HTML payload to every subscriber, dropping it for
// any client whose buffer is full rather than blocking.
func (s *Server) broadcast(rev int64, html, nav string) {
	s.actMu.Lock()
	src, det := s.lastSrc, s.lastDet
	s.actMu.Unlock()
	m := map[string]any{"rev": rev, "html": html, "theme": s.rt.CurrentTheme(), "source": src, "detail": det}
	if nav != "" {
		m["nav"] = nav
	}
	payload, _ := json.Marshal(m)
	msg := string(payload)
	s.subsMu.Lock()
	for ch := range s.subs {
		select {
		case ch <- msg:
		default:
		}
	}
	s.subsMu.Unlock()
}

// LogEntry is one line in the shared-session activity log.
type LogEntry struct {
	Seq    int    `json:"seq"`
	Time   string `json:"time"`
	Source string `json:"source"` // "human" | "agent" | "system"
	Detail string `json:"detail"`
}

// logEvent records a collaboration event (keeps the last 200).
func (s *Server) logEvent(source, detail string) {
	s.actMu.Lock()
	s.actSeq++
	s.activity = append(s.activity, LogEntry{Seq: s.actSeq, Time: time.Now().Format("15:04:05"), Source: source, Detail: detail})
	if len(s.activity) > 200 {
		s.activity = s.activity[len(s.activity)-200:]
	}
	s.lastSrc, s.lastDet = source, detail // for live edit attribution in the broadcast
	s.actMu.Unlock()
}

// serveLog returns activity entries after ?since=<seq> as JSON.
func (s *Server) serveLog(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var e struct{ Source, Detail string }
		if json.NewDecoder(r.Body).Decode(&e) == nil && e.Detail != "" {
			src := e.Source
			if src != "agent" && src != "human" {
				src = "app"
			}
			s.logEvent(src, e.Detail)
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	since, _ := strconv.Atoi(r.URL.Query().Get("since"))
	s.actMu.Lock()
	out := make([]LogEntry, 0, len(s.activity))
	for _, e := range s.activity {
		if e.Seq > since {
			out = append(out, e)
		}
	}
	s.actMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// servePresence records what the human is currently attending to (the focused or
// just-touched element), so the agent sees it via qorm_activity — the human side
// of presence, mirroring the human's "AI edited" flash.
func (s *Server) servePresence(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// The human's own panel reads this to show what is shared with the agent.
		s.actMu.Lock()
		out := map[string]any{}
		if s.humanFocus != "" {
			out["focus"] = s.humanFocus
		}
		if s.humanTyping != "" {
			out["typing"] = s.humanTyping
		}
		if s.humanFilled != "" {
			out["filled"] = s.humanFilled
		}
		s.actMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
		return
	}
	var p struct{ Element string }
	if json.NewDecoder(r.Body).Decode(&p) == nil {
		el := strings.TrimSpace(p.Element)
		if len(el) > 120 {
			el = el[:120]
		}
		s.actMu.Lock()
		s.humanFocus = el
		s.humanFocusAt = time.Now()
		// A typed entry ("<field> = <value>") is retained separately so a later tap
		// doesn't erase it; "(hidden)" password markers are not.
		if strings.HasSuffix(el, "= (hidden)") {
			s.humanFilled = strings.TrimSuffix(el, " = (hidden)")
			s.humanFilledAt = time.Now()
		} else if strings.Contains(el, " = ") {
			s.humanTyping = el
			s.humanTypingAt = time.Now()
		}
		s.actMu.Unlock()
	}
	w.WriteHeader(http.StatusNoContent)
}

// serveMeasure stores (POST) or returns (GET) the app's self-reported layout:
// each element's bounding rect + key computed styles, gathered by the running
// app in its own runtime. Lets the framework verify its own styles/positions
// without an external browser.
// userWebJS returns the app's native/web.js (custom callbacks + wiring for its
// own native ops), injected into the page so qormToNative(customOp) round-trips.
func userWebJS(rt *runtime.Runtime) string {
	if rt.App.BaseDir == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(rt.App.BaseDir, "native", "web.js"))
	if err != nil {
		return ""
	}
	return string(b)
}

// SetAppBaseDir sets where the app was loaded from, so native/web.js (a sibling
// of a bundle) is injected on desktop even when loaded from a compiled bundle.
func (s *Server) SetAppBaseDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rt.App.BaseDir = dir
}

// AppWindow returns the app's desktop window config (size/style).
func (s *Server) AppWindow() model.Window {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rt.App.Window
}

// AppShortcutsJSON returns the app-icon quick actions as a JSON array ("[]" if
// none), for the native launcher/Dock menu.
func (s *Server) AppShortcutsJSON() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.rt.App.Shortcuts) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(s.rt.App.Shortcuts)
	return string(b)
}

// AppMenuJSON is the desktop system-menu (menu bar) config as JSON.
func (s *Server) AppMenuJSON() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.rt.App.DesktopMenu) == 0 {
		return ""
	}
	b, _ := json.Marshal(s.rt.App.DesktopMenu)
	return string(b)
}

// AppTrayJSON is the desktop tray config as JSON ("" when no tray configured).
func (s *Server) AppTrayJSON() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.rt.App.Tray.Items) == 0 {
		return ""
	}
	b, _ := json.Marshal(s.rt.App.Tray)
	return string(b)
}

// SetWindowControl registers native window control (also exposes qorm_window MCP).
func (s *Server) SetWindowControl(mover func(id string, x, y, w, h int), op func(id, op string), open func(id, url string, w, h int), eval func(id, js string)) {
	s.WindowMover = mover
	s.WindowOp = op
	s.WindowOpen = open
	s.WindowEval = eval
	if s.agent != nil {
		s.agent.SetWindowControl(mover, op, open, eval)
	}
}

// serveWindow lets the control engine move/resize the native app window.
func (s *Server) serveWindow(w http.ResponseWriter, r *http.Request) {
	if s.WindowMover == nil {
		http.Error(w, "window control unavailable (not a native desktop app)", http.StatusNotImplemented)
		return
	}
	var m struct {
		ID, Op, URL, JS, Event string
		Data                   json.RawMessage
		X, Y, W, H             int
	}
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if m.ID == "" {
		m.ID = "main"
	}
	switch m.Op {
	case "open":
		if s.WindowOpen != nil {
			s.WindowOpen(m.ID, m.URL, m.W, m.H)
		}
	case "eval":
		if s.WindowEval != nil {
			s.WindowEval(m.ID, m.JS)
		}
	case "emit":
		if s.WindowEval != nil {
			data := "null"
			if len(m.Data) > 0 {
				data = string(m.Data)
			}
			s.WindowEval(m.ID, "window.qormOnWindowEvent&&qormOnWindowEvent("+strconv.Quote(m.Event)+","+data+")")
		}
	case "", "move":
		s.WindowMover(m.ID, m.X, m.Y, m.W, m.H)
	default:
		if s.WindowOp != nil {
			s.WindowOp(m.ID, m.Op)
		}
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) serveMeasure(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		s.measureMu.Lock()
		s.measure = body
		s.measureMu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.measureMu.Lock()
	b := s.measure
	s.measureMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if b == nil {
		w.Write([]byte("[]"))
		return
	}
	w.Write(b)
}

// recordAgentCall inspects an incoming MCP JSON-RPC request and logs mutating
// tool calls so the human sees what the agent is doing in the shared session.
func (s *Server) recordAgentCall(body []byte) {
	var req struct {
		Method string `json:"method"`
		Params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		} `json:"params"`
	}
	if json.Unmarshal(body, &req) != nil || req.Method != "tools/call" {
		return
	}
	a := req.Params.Arguments
	switch req.Params.Name {
	case "qorm_set_state":
		s.logEvent("agent", fmt.Sprintf("set_state %v = %v", a["path"], a["value"]))
	case "qorm_dispatch":
		s.logEvent("agent", fmt.Sprintf("dispatch %v", a["action"]))
	case "qorm_apply_patch":
		s.logEvent("agent", "apply_patch (UI edit)")
	case "qorm_undo":
		s.logEvent("agent", "undo")
	}
}

// serveEvents streams live updates to the browser over Server-Sent Events.
func (s *Server) serveEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ch := make(chan string, 8)
	s.subsMu.Lock()
	if s.subs == nil {
		s.subs = map[chan string]struct{}{}
	}
	s.subs[ch] = struct{}{}
	s.subsMu.Unlock()
	s.logEvent("system", "client connected ("+clientHost(r)+")")
	defer func() {
		s.subsMu.Lock()
		delete(s.subs, ch)
		s.subsMu.Unlock()
		s.logEvent("system", "client disconnected ("+clientHost(r)+")")
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

// Handler returns the HTTP mux.
// blockCrossOrigin rejects requests carrying a cross-origin (non-loopback)
// Origin header — the CSRF / DNS-rebind vector against a localhost server that
// exposes native power (/window eval, /update, /mcp). Requests with no Origin
// (local agents, curl, custom-scheme webviews) and loopback-origin requests
// (the app's own page) pass untouched, so MCP + dev-client workflows still work.
func blockCrossOrigin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && o != "null" {
			bad := true
			if u, err := url.Parse(o); err == nil {
				h := u.Hostname()
				bad = !(h == "localhost" || h == "127.0.0.1" || h == "::1")
			}
			if bad {
				http.Error(w, "cross-origin request rejected", http.StatusForbidden)
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.serveIndex)
	mux.HandleFunc("/event", blockCrossOrigin(s.serveEvent))
	mux.HandleFunc("/events", s.serveEvents)
	mux.HandleFunc("/poll", s.servePoll)
	mux.HandleFunc("/log", s.serveLog)
	mux.HandleFunc("/presence", blockCrossOrigin(s.servePresence))
	mux.HandleFunc("/console", s.serveConsole)
	mux.HandleFunc("/logwindow", s.serveLogWindow)
	mux.HandleFunc("/window", blockCrossOrigin(s.serveWindow))
	mux.HandleFunc("/measure", s.serveMeasure)
	mux.HandleFunc("/mcp", blockCrossOrigin(s.serveMCP))
	mux.HandleFunc("/update", blockCrossOrigin(s.serveUpdate))
	mux.HandleFunc("/rollback", blockCrossOrigin(s.serveRollback))
	mux.HandleFunc("/dev/state", blockCrossOrigin(s.serveDevState))
	mux.HandleFunc("/dev/tree", blockCrossOrigin(s.serveDevTree))
	mux.HandleFunc("/dev/highlight", blockCrossOrigin(s.serveDevHighlight))
	return mux
}

// activate swaps in a new bundle, remembering the previous one for rollback.
// Caller must hold s.mu.
func (s *Server) activate(b *bundle.Bundle) {
	s.prev = s.current
	s.current = b
	s.rt = runtime.New(b.ToApp())
	s.handlers = nil
	s.initAgent()
	s.bump()
}

// Update fetches, verifies and activates a bundle from source. On any failure
// the current app is left untouched (rollback by inaction). Returns a status
// line describing the transition.
func (s *Server) Update(source string) (string, error) {
	next, err := ota.FetchVerified(source, s.trust, s.revoked)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	from := "(none)"
	if s.current != nil {
		from = versionOr(s.current)
	}
	s.activate(next)
	return fmt.Sprintf("updated %s -> %s (%s)", from, versionOr(next), next.ContentHash), nil
}

// Rollback reactivates the previous bundle, if any.
func (s *Server) Rollback() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.prev == nil {
		return "", fmt.Errorf("no previous bundle to roll back to")
	}
	restored := s.prev
	s.prev = nil
	from := versionOr(s.current)
	s.current = restored
	s.rt = runtime.New(restored.ToApp())
	s.handlers = nil
	s.initAgent()
	s.bump()
	return fmt.Sprintf("rolled back %s -> %s", from, versionOr(restored)), nil
}

func versionOr(b *bundle.Bundle) string {
	if v := b.Version(); v != "" {
		return v
	}
	return "unversioned"
}

func (s *Server) serveUpdate(w http.ResponseWriter, r *http.Request) {
	if s.current == nil {
		http.Error(w, "OTA not enabled (run from a bundle)", http.StatusBadRequest)
		return
	}
	if s.trust == nil {
		http.Error(w, "OTA disabled: authenticity is not verifiable without a trusted key — restart with --trust <key.pub>", http.StatusForbidden)
		return
	}
	var req struct {
		Source string `json:"source"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Source == "" {
		http.Error(w, "missing source", http.StatusBadRequest)
		return
	}
	status, err := s.Update(req.Source)
	if err != nil {
		// Update refused: the live app keeps running the previous bundle.
		http.Error(w, "update rejected (kept current): "+err.Error(), http.StatusConflict)
		return
	}
	fmt.Fprintln(w, status)
}

func (s *Server) serveRollback(w http.ResponseWriter, _ *http.Request) {
	status, err := s.Rollback()
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	fmt.Fprintln(w, status)
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	scene := r.URL.Query().Get("scene")
	s.mu.Lock()
	if scene == "" { // no explicit scene (a desktop window may pin one): follow navigation
		scene = s.rt.CurrentScene()
	}
	res := render.RenderScene(s.rt, scene)
	s.handlers = res.Handlers
	rev := s.rev.Load()
	rt := s.rt
	// Build the page while still holding the lock: Page/userWebJS read rt.State
	// (locale/theme/rtl), which a concurrent POST /event mutates — reading it
	// unlocked is a concurrent-map read+write and crashes the process.
	html := Page(rt, res.HTML, rev)
	if js := userWebJS(rt); js != "" {
		html = strings.Replace(html, "</body>", "<script>"+js+"</script></body>", 1)
	}
	transparent := rt.App.Window.Transparent
	s.mu.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if transparent {
		// transparent window  clear the page/stage background so the app's own
		// shaped content defines the visible window shape (rest is click-through).
		html = strings.Replace(html, "</head>", "<style>html,body,#qorm-stage{background:transparent!important;box-shadow:none!important;}</style></head>", 1)
	}
	fmt.Fprint(w, html)
}

// serveMCP is the HTTP transport for the shared MCP session: one JSON-RPC
// request in, one response out. The agent operates the same runtime the browser
// renders, guarded by the same mutex.
func (s *Server) serveMCP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.recordAgentCall(body)
	resp := s.agent.HandleHTTP(body)
	w.Header().Set("Content-Type", "application/json")
	if resp == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	_, _ = w.Write(resp)
}

// servePoll lets the browser observe out-of-band changes (e.g. an agent edit):
// it returns the current revision, plus fresh HTML when the revision advanced.
func (s *Server) servePoll(w http.ResponseWriter, r *http.Request) {
	clientRev, _ := strconv.ParseInt(r.URL.Query().Get("rev"), 10, 64)
	s.mu.Lock()
	cur := s.rev.Load()
	var html string
	if cur != clientRev {
		res := render.RenderScene(s.rt, s.rt.CurrentScene())
		s.handlers = res.Handlers
		html = res.HTML
	}
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	out := map[string]any{"rev": cur}
	if html != "" {
		out["html"] = html
	}
	_ = json.NewEncoder(w).Encode(out)
}

type eventReq struct {
	H      int            `json:"h"`
	Inputs map[string]any `json:"inputs"`
}

func (s *Server) serveEvent(w http.ResponseWriter, r *http.Request) {
	var req eventReq
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Ensure the handler table exists: a client that POSTs /event before ever
	// GETting / (a reconnect, or an out-of-order request) would otherwise find
	// an empty table and silently drop the action.
	if s.handlers == nil {
		s.handlers = render.RenderScene(s.rt, s.rt.CurrentScene()).Handlers
	}
	// Fold current input values back into state before dispatching.
	for path, val := range req.Inputs {
		s.rt.State[path] = val
	}
	if req.H >= 0 && req.H < len(s.handlers) {
		h := s.handlers[req.H]
		if h.Name != "" {
			s.logEvent("human", "dispatch "+h.Name)
		}
		// Re-evaluate args in the handler's captured scope + fresh state.
		ctx := map[string]any{"state": s.rt.State}
		for k, v := range h.Scope {
			ctx[k] = v
		}
		args := map[string]any{}
		for name, exprStr := range h.Args {
			args[name] = runtime.EvalBinding(exprStr, ctx)
		}
		s.rt.Dispatch(h.Name, args)
	}
	rev, html, nav := s.bump()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Qorm-Rev", strconv.FormatInt(rev, 10))
	w.Header().Set("X-Qorm-Theme", s.rt.CurrentTheme())
	if nav != "" {
		w.Header().Set("X-Qorm-Nav", nav)
	}
	fmt.Fprint(w, html)
}

// Page wraps rendered body HTML in a full document with the live shim.
//
//go:embed app.js
var appJS string

// qormAppJS returns the client script with the current revision substituted.
func qormAppJS(rev int64) string {
	return strings.ReplaceAll(appJS, "__QORM_REV__", strconv.FormatInt(rev, 10))
}

func Page(rt *runtime.Runtime, body string, rev int64) string {
	w := rt.App.Window
	width := w.Width
	if width == 0 {
		width = 420
	}
	height := w.Height
	if height == 0 {
		height = 720
	}
	title := w.Title
	if title == "" {
		title = rt.App.Name
	}
	lang := rt.CurrentLocale()
	if lang == "" {
		lang = "en"
	}
	dir := "ltr"
	if rt.IsRTL() {
		dir = "rtl"
	}
	theme := rt.CurrentTheme()
	return fmt.Sprintf(`<!doctype html>
<html lang="%s" dir="%s">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1, user-scalable=no, viewport-fit=cover">
<title>%s</title>
<style>
  /* ---- Design tokens (themes). Apple/iOS is the default; a manifest theme
     or state.theme selects another. Switch by class on the stage. ---- */
  :root, .qorm-theme-apple {
    --accent:#007aff; --on-accent:#fff; --success:#34c759; --danger:#ff3b30; --warning:#ff9500;
    --bg:#f2f2f7; --surface:#fff; --label:#000; --label2:#3c3c4399; --sep:#3c3c4949;
    --fill:#78788033; --radius:12px; --radius-lg:20px; --stage-radius:38px;
    --font:-apple-system,BlinkMacSystemFont,'SF Pro Text','SF Pro Display','Helvetica Neue',Arial,sans-serif; color-scheme:light; }
  .qorm-theme-material {
    --accent:#2e7df6; --on-accent:#fff; --success:#16a34a; --danger:#dc2626; --warning:#f59e0b;
    --bg:#eef0f4; --surface:#fff; --label:#111827; --label2:#6b7280; --sep:#e5e7eb;
    --fill:#e5e7eb; --radius:8px; --radius-lg:12px; --stage-radius:14px;
    --font:'Segoe UI',Roboto,-apple-system,BlinkMacSystemFont,sans-serif; color-scheme:light; }
  .qorm-theme-dark {
    --accent:#0a84ff; --on-accent:#fff; --success:#30d158; --danger:#ff453a; --warning:#ff9f0a;
    --bg:#000; --surface:#1c1c1e; --label:#fff; --label2:#ebebf599; --sep:#54545899;
    --fill:#7676803d; --radius:12px; --radius-lg:20px; --stage-radius:38px;
    --font:-apple-system,BlinkMacSystemFont,'SF Pro Text','SF Pro Display','Helvetica Neue',Arial,sans-serif; color-scheme:dark; }
  * { margin:0; padding:0; box-sizing:border-box; -webkit-font-smoothing:antialiased;
      touch-action:manipulation; -webkit-touch-callout:none;
      -webkit-user-select:none; user-select:none; -webkit-tap-highlight-color:transparent; }
  /* re-enable text selection only where it's real content input */
  input, textarea, [contenteditable="true"], .qorm-selectable { -webkit-user-select:text; user-select:text; }
  html, body { overscroll-behavior:none; -webkit-overflow-scrolling:touch; }
  body { background:var(--bg); color:var(--label); font-family:var(--font); letter-spacing:-0.01em;
         min-height:100vh; display:flex; align-items:flex-start; justify-content:center; padding:24px; }
  /* live collaborator presence: "AI edited" toast when an agent changes the shared app */
  #qorm-presence { position:fixed; left:50%%; bottom:20px; transform:translate(-50%%,20px);
    display:flex; align-items:center; gap:7px; padding:9px 15px; border-radius:20px;
    background:var(--accent); color:var(--on-accent,#fff); font-weight:600; font-size:13px;
    box-shadow:0 8px 24px rgba(0,0,0,.28); opacity:0; pointer-events:none; z-index:99999;
    transition:opacity .2s ease, transform .2s ease; }
  #qorm-presence.show { opacity:1; transform:translate(-50%%,0); }
  /* Responsive: a centered device frame on PC, full-bleed on phones. */
  #qorm-stage { width:%dpx; max-width:100%%; min-height:%dpx; background:var(--bg); color:var(--label);
                border-radius:var(--stage-radius); box-shadow:0 12px 48px rgba(0,0,0,.18);
                overflow:hidden; display:flex; }
  @media (max-width:640px) {
    body { padding:0; align-items:stretch; }
    #qorm-stage { width:100%%; max-width:100%%; min-height:100vh; border-radius:0; box-shadow:none; }
  }
  /* Desktop: expand into the available width instead of a lone phone frame,
     and mark the stage so widgets can switch to their desktop form. */
  @media (min-width:1024px) {
    body { padding:32px; align-items:flex-start; }
    #qorm-stage { width:min(1080px,92vw); border-radius:18px; }
    #qorm-stage.qorm-fluid { width:min(1400px,96vw); }
    /* Hybrid content: the main column is centered and capped for readability;
       naturally-wide widgets (grids, tables, charts, media) fill the width. */
    .qorm-body > * { max-width:960px; margin-left:auto; margin-right:auto; }
    .qorm-body .qorm-wide { max-width:none; }
    /* Bottom tab bar is a mobile idiom — on desktop lift it to a top bar. */
    .qorm-bottomnav { order:-1; border-top:0; border-bottom:1px solid var(--sep); justify-content:center; gap:6px; }
    .qorm-bottomnav .qorm-navitem { flex:0 0 auto; flex-direction:row; gap:8px; padding:12px 18px; }
    /* iOS action sheet rises from the bottom on phones; center it on desktop. */
    .qorm-sheet { align-items:center; }
  }
  /* Desktop hover feedback (pointer devices only). */
  @media (hover:hover) {
    button:hover { filter:brightness(0.96); }
    a:hover { opacity:0.85; }
    .qorm-tab:hover, .qorm-acc:hover, .qorm-menu-panel button:hover, .qorm-navitem:hover { background:var(--fill); }
    .qorm-datatable tbody tr:hover { background:var(--fill); }
  }
  input,button,select,textarea { font-family:inherit; letter-spacing:inherit; }
  /* iOS switch: a 51x31 pill, green when checked. */
  .qorm-switch { position:relative; width:51px; height:31px; flex:none; }
  .qorm-switch input { position:absolute; opacity:0; width:0; height:0; }
  .qorm-switch span { position:absolute; inset:0; background:var(--fill); border-radius:16px; transition:background .25s; }
  .qorm-switch span::before { content:""; position:absolute; top:2px; left:2px; width:27px; height:27px; border-radius:50%%;
    background:#fff; box-shadow:0 2px 5px rgba(0,0,0,.25); transition:transform .25s; }
  .qorm-switch input:checked + span { background:var(--success); }
  .qorm-switch input:checked + span::before { transform:translateX(20px); }
  #qorm-root { flex:1; display:flex; }
  #qorm-root > * { flex:1; }
  @keyframes qorm-spin { to { transform: rotate(360deg); } }
  .qorm-spin { animation: qorm-spin .8s linear infinite; }
  .qorm-tab-active { border-bottom:2px solid #007aff !important; color:#007aff; font-weight:600; }
  .qorm-table { border-collapse:collapse; width:100%%; }
  .qorm-table th, .qorm-table td { border:1px solid var(--sep); padding:8px 12px; text-align:left; font-size:14px; }
  .qorm-table th { background:var(--fill); font-weight:600; }
  .qorm-datatable { border-collapse:collapse; width:100%%; font-size:14px; }
  .qorm-datatable th { text-align:left; font-weight:600; color:var(--label2); padding:10px 12px; border-bottom:1px solid var(--sep); white-space:nowrap; }
  .qorm-datatable td { padding:10px 12px; border-bottom:1px solid var(--sep); color:var(--label); }
  .qorm-datatable tbody tr.qdt-sel { background:var(--fill); }
  .qorm-datatable .qdt-check { width:36px; text-align:center; cursor:pointer; }
  .qorm-datatable button.qdt-sort { background:none; border:none; font:inherit; font-weight:600; color:var(--label2); cursor:pointer; padding:0; display:inline-flex; align-items:center; gap:4px; }
  @keyframes qorm-shimmer { 0%% { background-position:200%% 0; } 100%% { background-position:-200%% 0; } }
  .qorm-skel { background:linear-gradient(90deg,#e9ecef 25%%,#f3f5f7 50%%,#e9ecef 75%%); background-size:200%% 100%%; animation:qorm-shimmer 1.3s ease-in-out infinite; }
  /* Range slider: two overlaid thumbs must stay interactive over a pass-through track. */
  .qorm-range-lo, .qorm-range-hi { -webkit-appearance:none; appearance:none; background:transparent; pointer-events:none; }
  .qorm-range-lo::-webkit-slider-thumb, .qorm-range-hi::-webkit-slider-thumb { -webkit-appearance:none; pointer-events:auto;
    width:22px; height:22px; border-radius:50%%; background:#fff; box-shadow:0 1px 4px rgba(0,0,0,.3); cursor:pointer; margin-top:-9px; }
  .qorm-range-lo::-moz-range-thumb, .qorm-range-hi::-moz-range-thumb { pointer-events:auto; width:22px; height:22px; border:none; border-radius:50%%; background:#fff; box-shadow:0 1px 4px rgba(0,0,0,.3); }
  /* iOS slider: thin track, round white thumb. */
  .qorm-slider { -webkit-appearance:none; appearance:none; height:28px; background:transparent; outline:none; cursor:pointer; }
  .qorm-slider::-webkit-slider-runnable-track { height:4px; border-radius:2px;
    background:linear-gradient(90deg,var(--accent) var(--pct,0%%),var(--fill) var(--pct,0%%)); }
  .qorm-slider::-webkit-slider-thumb { -webkit-appearance:none; width:28px; height:28px; border-radius:50%%;
    background:#fff; box-shadow:0 1px 5px rgba(0,0,0,.3); cursor:pointer; margin-top:-12px; }
  .qorm-slider::-moz-range-track { height:4px; border-radius:2px;
    background:linear-gradient(90deg,var(--accent) var(--pct,0%%),var(--fill) var(--pct,0%%)); }
  .qorm-slider::-moz-range-thumb { width:28px; height:28px; border:none; border-radius:50%%; background:#fff; box-shadow:0 1px 5px rgba(0,0,0,.3); }
  /* iOS activity indicator: 8 spokes ticking around. */
  .qorm-activity svg { animation:qorm-ios-spin 1s steps(8) infinite; }
  @keyframes qorm-ios-spin { to { transform:rotate(360deg); } }
  /* ---- Motion catalog: named keyframe animations. A widget's animation
     prop selects one; being bindable, an agent switches the effect live. ---- */
  /* Interactive: iOS press feedback (tap-to-scale) on buttons & tappables. */
  .qorm-tap { transition:transform .12s ease, opacity .12s ease; -webkit-tap-highlight-color:transparent; }
  .qorm-tap:active { transform:scale(.96); opacity:.7; }
  /* Spatial attribution: a node the AI just changed pulses a blue outline. */
  .qorm-ai-touch { animation:qorm-ai-flash 1.3s ease-out; border-radius:inherit; }
  @keyframes qorm-ai-flash { 0%% { box-shadow:0 0 0 2px rgba(10,132,255,.9); } 60%% { box-shadow:0 0 0 2px rgba(10,132,255,.45); } 100%% { box-shadow:0 0 0 2px rgba(10,132,255,0); } }
  @keyframes qa-fade { from { opacity:0; } to { opacity:1; } }
  @keyframes qa-fadeup { from { opacity:0; transform:translateY(16px); } to { opacity:1; transform:none; } }
  @keyframes qa-fadedown { from { opacity:0; transform:translateY(-16px); } to { opacity:1; transform:none; } }
  @keyframes qa-slideup { from { transform:translateY(100%%); } to { transform:none; } }
  @keyframes qa-slidedown { from { transform:translateY(-100%%); } to { transform:none; } }
  @keyframes qa-slideleft { from { transform:translateX(100%%); } to { transform:none; } }
  @keyframes qa-slideright { from { transform:translateX(-100%%); } to { transform:none; } }
  @keyframes qa-scale { from { opacity:0; transform:scale(.8); } to { opacity:1; transform:none; } }
  @keyframes qa-zoomout { from { opacity:0; transform:scale(1.15); } to { opacity:1; transform:none; } }
  @keyframes qa-rotate { from { opacity:0; transform:rotate(-180deg) scale(.6); } to { opacity:1; transform:none; } }
  @keyframes qa-flip { from { opacity:0; transform:perspective(600px) rotateY(90deg); } to { opacity:1; transform:none; } }
  @keyframes qa-pop { 0%% { opacity:0; transform:scale(.5); } 70%% { transform:scale(1.06); } 100%% { opacity:1; transform:none; } }
  @keyframes qa-bounce { 0%% { transform:translateY(-120%%); } 60%% { transform:translateY(8%%); } 80%% { transform:translateY(-4%%); } 100%% { transform:none; } }
  @keyframes qa-shake { 0%%,100%% { transform:none; } 20%%,60%% { transform:translateX(-8px); } 40%%,80%% { transform:translateX(8px); } }
  @keyframes qa-pulse { 0%%,100%% { transform:none; } 50%% { transform:scale(1.08); } }
  @keyframes qa-spin { to { transform:rotate(360deg); } }
  @keyframes qa-size { from { transform:scaleY(0); transform-origin:top; } to { transform:scaleY(1); transform-origin:top; } }
  [data-tooltip] { position:relative; }
  [data-tooltip]:hover::after { content:attr(data-tooltip); position:absolute; bottom:100%%; left:50%%; transform:translateX(-50%%);
    background:#111827; color:#fff; padding:4px 8px; border-radius:6px; font-size:12px; white-space:nowrap; margin-bottom:6px; z-index:100; pointer-events:none; }
</style>
</head>
<body>
<div id="qorm-stage" class="qorm-theme-%s"><div id="qorm-root">%s</div></div>
<script>%s</script>
</body>
</html>`, lang, dir, htmlEscape(title), width, height, theme, body, qormAppJS(rev))
}

func htmlEscape(s string) string {
	repl := map[rune]string{'&': "&amp;", '<': "&lt;", '>': "&gt;"}
	out := make([]rune, 0, len(s))
	for _, c := range s {
		if r, ok := repl[c]; ok {
			out = append(out, []rune(r)...)
		} else {
			out = append(out, c)
		}
	}
	return string(out)
}

// clientHost returns a readable client identifier for the activity log — the
// remote IP, so a physical device joining the session is visible.
func clientHost(r *http.Request) string {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "::1" || host == "127.0.0.1" {
		return "local"
	}
	return host
}
