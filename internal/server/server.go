// Package server serves a live QORM app over HTTP. Button presses POST to
// /event; the server updates state, dispatches the action, re-renders, and
// returns the new body HTML which a tiny inline script swaps in. No cgo, no
// external deps — so the binary cross-compiles to every platform cleanly.
package server

import (
	"crypto/ed25519"
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
func (s *Server) bump() (int64, string) {
	rev := s.rev.Add(1)
	res := render.RenderScene(s.rt, s.rt.CurrentScene())
	s.handlers = res.Handlers
	s.broadcast(rev, res.HTML)
	return rev, res.HTML
}

// broadcast pushes a revision+HTML payload to every subscriber, dropping it for
// any client whose buffer is full rather than blocking.
func (s *Server) broadcast(rev int64, html string) {
	s.actMu.Lock()
	src, det := s.lastSrc, s.lastDet
	s.actMu.Unlock()
	payload, _ := json.Marshal(map[string]any{"rev": rev, "html": html, "theme": s.rt.CurrentTheme(), "source": src, "detail": det})
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
	rev, html := s.bump()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Qorm-Rev", strconv.FormatInt(rev, 10))
	w.Header().Set("X-Qorm-Theme", s.rt.CurrentTheme())
	fmt.Fprint(w, html)
}

// Page wraps rendered body HTML in a full document with the live shim.
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
  /* Page transition: a scene swapped in by navigation slides + fades in. */
  .qorm-scene-in { animation:qorm-scene-in 1000ms cubic-bezier(.32,.72,0,1) both; }
  @keyframes qorm-scene-in { from { opacity:0; transform:translateX(80px); } to { opacity:1; transform:none; } }
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
<script>
// Gather all state-bound controls into a typed map, dispatch handler h, and
// swap in the re-rendered UI. h === -1 means "just sync state" (no action).
// morphChildren diffs the new HTML into the live DOM in place, so unchanged
// nodes are never re-created — no flicker, entrance animations don't replay on
// every click, and input focus/scroll survive.
// Self-measurement: report each id'd element's rect + key styles to /measure,
// so the framework can verify its own layout/styles without an external browser.
function qormMeasure(){
  try{
    var out=[];
    document.querySelectorAll('[id]').forEach(function(el){
      if(el.id==='qorm-root'||el.id==='qorm-stage') return;
      var r=el.getBoundingClientRect(), cs=getComputedStyle(el);
      var vis = r.width>0 && r.height>0 && cs.display!=='none' && cs.visibility!=='hidden' && parseFloat(cs.opacity)>0.01;
      out.push({id:el.id, tag:el.tagName.toLowerCase(),
        x:Math.round(r.left), y:Math.round(r.top), w:Math.round(r.width), h:Math.round(r.height),
        visible:vis, text:(el.childElementCount===0?(el.textContent||'').trim().slice(0,60):''),
        display:cs.display, color:cs.color, background:cs.backgroundColor,
        fontSize:cs.fontSize, fontWeight:cs.fontWeight, textAlign:cs.textAlign,
        padding:cs.padding, margin:cs.margin, borderRadius:cs.borderRadius,
        border:(cs.borderTopWidth!=='0px'?cs.borderTopWidth+' '+cs.borderTopStyle+' '+cs.borderTopColor:'none'),
        opacity:cs.opacity, zIndex:cs.zIndex, position:cs.position,
        overflowX:el.scrollWidth>el.clientWidth+1});
    });
    fetch('/measure',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(out)});
  }catch(e){}
}
// qormFlash briefly outlines a node the AI just changed, so the human sees WHERE
// an edit landed (spatial attribution), not only the "AI edited" toast. No-op for
// the human's own edits and initial paint.
function qormFlash(el){
  if(!el || el.nodeType!==1 || window.__qormEditSrc!=='agent') return;
  el.classList.add('qorm-ai-touch');
  setTimeout(function(){ el.classList.remove('qorm-ai-touch'); }, 1300);
}
function qormMorphInto(root, html){
  var tmp=document.createElement('div'); tmp.innerHTML=html;
  var active=document.activeElement, activeId=active&&active.id;
  morphKids(root, tmp);
  if(activeId){ var el=document.getElementById(activeId); if(el&&el.focus) try{ el.focus(); }catch(e){} }
  setTimeout(qormMeasure, 30);
}
function morphKids(from, to){
  var fc=from.firstChild, tc=to.firstChild;
  while(tc){
    var nt=tc.nextSibling;
    if(!fc){ var an=document.importNode(tc,true); from.appendChild(an); qormFlash(an); tc=nt; continue; }
    var nf=fc.nextSibling;
    if(fc.nodeType!==tc.nodeType || (fc.nodeType===1 && fc.nodeName!==tc.nodeName)){
      var rn=document.importNode(tc,true); from.replaceChild(rn, fc); qormFlash(rn);
    } else if(fc.nodeType===3 || fc.nodeType===8){
      if(fc.nodeValue!==tc.nodeValue){ fc.nodeValue=tc.nodeValue; qormFlash(from); }
    } else if(fc.nodeType===1 && fc.getAttribute('data-scene')!==null && fc.getAttribute('data-scene')!==tc.getAttribute('data-scene')){
      // navigation swapped the scene: recreate the root so it plays a page transition
      var sn=document.importNode(tc,true); from.replaceChild(sn, fc); sn.classList.add('qorm-scene-in');
    } else if(fc.nodeType===1){
      morphEl(fc, tc);
    }
    fc=nf; tc=nt;
  }
  while(fc){ var n=fc.nextSibling; from.removeChild(fc); fc=n; }
}
function morphEl(from, to){
  // sync attributes; keep a transient page-transition class a redundant re-morph
  // (SSE + the POST response both apply the same update) would otherwise strip.
  var changed=false, hadAnim=from.classList&&from.classList.contains('qorm-scene-in');
  var ta=to.attributes, i, a;
  for(i=ta.length-1;i>=0;i--){ a=ta[i]; if(from.getAttribute(a.name)!==a.value){ from.setAttribute(a.name,a.value); changed=true; } }
  var fa=from.attributes;
  for(i=fa.length-1;i>=0;i--){ a=fa[i]; if(!to.hasAttribute(a.name)){ from.removeAttribute(a.name); changed=true; } }
  if(hadAnim && !from.classList.contains('qorm-scene-in')) from.classList.add('qorm-scene-in');
  if(changed) qormFlash(from);
  var focused=(document.activeElement===from);
  // form controls: keep the user's live value/checked unless they're not focused
  if(from.nodeName==='INPUT'){
    if(!focused){ if(to.hasAttribute('checked')!==from.checked) from.checked=to.hasAttribute('checked');
      if(to.getAttribute('value')!=null && from.value!==to.getAttribute('value')) from.value=to.getAttribute('value'); }
    return;
  }
  if(from.nodeName==='TEXTAREA'){ if(!focused) from.value=to.textContent; return; }
  morphKids(from, to);
}
function qorm(h){
  var inputs={};
  document.querySelectorAll('[data-state]').forEach(function(el){
    var k=el.getAttribute('data-state');
    if(el.type==='checkbox'){ inputs[k]=el.checked; }
    else if(el.type==='radio'){ if(el.checked) inputs[k]=el.value; }
    else if(el.type==='range'||el.type==='number'){ inputs[k]=parseFloat(el.value); }
    else { inputs[k]=el.value; }
  });
  fetch('/event',{method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({h:h,inputs:inputs})})
    .then(function(r){ var rv=parseInt(r.headers.get('X-Qorm-Rev'))||0; qormTheme(r.headers.get('X-Qorm-Theme')); return r.text().then(function(html){ return {rv:rv,html:html}; }); })
    .then(function(o){ if(o.rv && o.rv<=__rev) return; if(o.rv) __rev=o.rv; qormMorphInto(document.getElementById('qorm-root'), o.html); });
}
// Camera: open the device camera/photo picker, read the chosen image as a data
// URL, show it in the preview, sync it into bound state, and fire onChange.
function qormCameraInit(){
  if(!(navigator.mediaDevices && navigator.mediaDevices.getUserMedia && window.isSecureContext)) return;
  document.querySelectorAll('.qorm-camera').forEach(function(box){
    var live=box.querySelector('.qorm-cam-live'), file=box.querySelector('.qorm-cam-file');
    if(live) live.style.display='inline-block';
    if(file) file.style.display='none';
  });
}
function qormCameraLive(btn){
  var box=btn.closest('.qorm-camera'); if(!box) return;
  var video=box.querySelector('.qorm-cam-video');
  if(box._stream){
    var c=document.createElement('canvas'); c.width=video.videoWidth||640; c.height=video.videoHeight||480;
    c.getContext('2d').drawImage(video,0,0,c.width,c.height);
    var data=c.toDataURL('image/jpeg',0.9);
    var img=box.querySelector('.qorm-cam-preview'); if(img){ img.src=data; img.style.display='block'; }
    var hid=box.querySelector('input[type=hidden]'); if(hid){ hid.value=data; }
    box._stream.getTracks().forEach(function(t){ t.stop(); }); box._stream=null;
    video.style.display='none'; btn.textContent='Retake';
    var h=box.getAttribute('data-h'); qorm(h?parseInt(h):-1);
    return;
  }
  navigator.mediaDevices.getUserMedia({video:{facingMode:'environment'}}).then(function(stream){
    box._stream=stream; video.srcObject=stream; video.style.display='block'; video.play(); btn.textContent='Capture';
  }).catch(function(e){
    var wrap=box.querySelector('.qorm-cam-file'); var fi=wrap&&wrap.querySelector('input');
    if(wrap){ wrap.style.display='inline-block'; } if(fi){ fi.click(); }
  });
}
function qormCamera(input){
  var f=input.files&&input.files[0]; if(!f) return;
  var box=input.closest('.qorm-camera'); if(!box) return;
  var rd=new FileReader();
  rd.onload=function(){
    var img=box.querySelector('.qorm-cam-preview'); if(img){ img.src=rd.result; img.style.display='block'; }
    var hid=box.querySelector('input[type=hidden]'); if(hid){ hid.value=rd.result; }
    var h=box.getAttribute('data-h');
    qorm(h?parseInt(h):-1);
  };
  rd.readAsDataURL(f);
}
// Native hardware bridge (present in the QORM Dev app): call native
// CoreLocation/CoreMotion/etc. — no HTTPS/secure-context needed. Falls back to
// the Web API in a plain browser.
function qormHasNative(){ return !!((window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.qorm) || window.qormAndroid || window.qormDesktop); }
// Mobile bridge only (iOS/Android) — the full hardware bridge. Desktop implements
// just a subset, so camera/mic/location must use the Web API there, not the bridge.
function qormHasMobileNative(){ return !!((window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.qorm) || window.qormAndroid); }
function qormToNative(op,data){
  // The app's OWN Go middle-layer (compiled into the WASM) handles its custom
  // ops first — so one Go file runs on mobile/web WebViews. It returns a line
  // of JS (may itself call qormToNative(...) to reach framework hardware, or a
  // Web API); "" means "not mine"  fall through to the built-in bridge.
  if(window.qormWasmOp){ var r=window.qormWasmOp(op, JSON.stringify(data||{})); if(r){ try{ (0,eval)(r); }catch(e){} return; } }
  var msg = Object.assign({op:op}, data||{});
  if(window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.qorm){ window.webkit.messageHandlers.qorm.postMessage(msg); }
  else if(window.qormAndroid && typeof window.qormAndroid[op]==='function'){ window.qormAndroid[op](JSON.stringify(data||{})); }
  else if(window.qormDesktop){ window.qormDesktop(JSON.stringify(msg)); }
}
function qormOnScreens(list){ var t = (list||[]).map(function(s,i){ return 'Display '+(i+1)+': '+s.w+'×'+s.h+' @'+s.scale+'x'+(s.main?' (main)':''); }).join('\n'); document.querySelectorAll('.qorm-screens-out').forEach(function(o){ o.textContent = t || 'no display info'; }); }
function qormLoginItem(btn){ var box=btn.closest('.qorm-loginitem'); var on=box.getAttribute('data-on')==='1'; if(qormHasNative()){ qormToNative('loginItem',{enabled:!on}); } else { box.querySelector('.qorm-loginitem-out').textContent='desktop only'; } }
function qormOnLoginItem(on,ok){ document.querySelectorAll('.qorm-loginitem').forEach(function(box){ box.setAttribute('data-on', on?'1':'0'); box.querySelector('.qorm-loginitem-out').textContent='Start at Login: '+(on?'ON':'OFF')+(ok?'':' (install the .app first)'); }); }
function qormOnNotifyClick(id){ var box=document.getElementById(id); if(box){ var o=box.querySelector('.qorm-notify-out'); if(o) o.textContent='Notification clicked '; } }
function qormBadge(btn,d){ var box=btn.closest('.qorm-dockbadge'); var n=Math.max(0,(parseInt(box.getAttribute('data-count'))||0)+d); box.setAttribute('data-count',n); box.querySelector('.qorm-dockbadge-out').textContent='Badge: '+n; if(qormHasNative()){ qormToNative('badge',{count:n}); } }
function qormNotify(btn){
  var box=btn.closest('.qorm-notify'), title=box.getAttribute('data-title')||'QORM', body=box.getAttribute('data-body')||'Hello from your QORM app ';
  var out=box.querySelector('.qorm-notify-out');
  if(qormHasNative()){ qormToNative('notify',{title:title,body:body,id:box.id}); out.textContent='Sent '; }
  else if('Notification'in window){ Notification.requestPermission().then(function(p){ if(p==='granted'){ new Notification(title,{body:body}); out.textContent='Sent '; } else { out.textContent='permission denied'; } }); }
  else { out.textContent='not supported'; }
}
// Geolocation: read the device GPS and sync "lat, lng" into bound state.
function qormGeo(btn){
  var out=btn.closest('.qorm-location').querySelector('.qorm-loc-out');
  out.textContent='Locating…';
  if(qormHasMobileNative()){ qormToNative('location'); return; }
  if(!navigator.geolocation){ out.textContent='Geolocation not supported (needs the QORM Dev app or https)'; return; }
  navigator.geolocation.getCurrentPosition(function(p){ qormOnLocation(p.coords.latitude, p.coords.longitude, p.coords.accuracy); },
    function(e){ qormOnLocationError(e.message); }, {enableHighAccuracy:true, timeout:10000});
}
function qormOnLocation(lat,lng,acc){
  var s=lat.toFixed(5)+', '+lng.toFixed(5)+'  (±'+Math.round(acc)+'m)';
  document.querySelectorAll('.qorm-location').forEach(function(box){
    box.querySelector('.qorm-loc-out').textContent=s;
    var hid=box.querySelector('input[type=hidden]'); if(hid){ hid.value=s; }
  });
  qorm(-1);
}
function qormOnLocationError(msg){ document.querySelectorAll('.qorm-location .qorm-loc-out').forEach(function(o){ o.textContent='Error: '+msg; }); }
// Motion: stream device orientation (accelerometer/gyro) live.
function qormMotion(btn){
  var out=btn.closest('.qorm-motion').querySelector('.qorm-motion-out');
  if(qormHasNative()){ qormToNative('motionStart'); btn.textContent='Motion On'; return; }
  function start(){
    window.addEventListener('deviceorientation', function(e){ qormOnMotion(e.alpha||0, e.beta||0, e.gamma||0); });
    btn.textContent='Motion On';
  }
  if(typeof DeviceOrientationEvent!=='undefined' && typeof DeviceOrientationEvent.requestPermission==='function'){
    DeviceOrientationEvent.requestPermission().then(function(r){ if(r==='granted'){ start(); } else { out.textContent='Permission denied'; } }).catch(function(e){ out.textContent='Error: '+e; });
  } else { start(); }
}
function qormBio(btn){
  var out=btn.closest('.qorm-biometric').querySelector('.qorm-bio-out');
  out.textContent='Authenticating…';
  if(qormHasNative()){ qormToNative('biometric'); return; }
  out.textContent='Biometrics need the QORM Dev app';
}
function qormOnBiometric(ok, msg){
  document.querySelectorAll('.qorm-biometric').forEach(function(box){
    box.querySelector('.qorm-bio-out').textContent=(ok?'Authenticated':'Not authenticated')+(msg?' — '+msg:'');
    var hid=box.querySelector('input[type=hidden]'); if(hid){ hid.value=ok?'authenticated':'failed'; }
  });
  qormEmit('biometric', ok);
}
function qormBluetooth(btn){ var out=btn.closest('.qorm-bluetooth').querySelector('.qorm-bluetooth-out'); out.textContent='Scanning…'; if(qormHasNative()){ qormToNative('bluetoothScan'); } else { out.textContent='Bluetooth needs the QORM Dev app'; } }
function qormOnBluetoothState(on){ document.querySelectorAll('.qorm-bluetooth-out').forEach(function(o){ o.textContent='Bluetooth: '+(on?'ON':'OFF'); }); }
function qormOnBluetooth(json){ var list; try{ list=JSON.parse(json); }catch(e){ list=[]; }
  document.querySelectorAll('.qorm-bluetooth-out').forEach(function(o){ o.textContent = list.length ? list.map(function(d){ return (d.name||'(unknown)')+'  '+d.rssi+'dBm'; }).join('\n') : 'No devices found'; }); }
var QORM_CAPS = {
  'qorm-camera':'ios android mac linux windows web','qorm-location':'ios android mac linux windows web',
  'qorm-recorder':'ios android mac linux windows web','qorm-battery':'ios android mac linux web',
  'qorm-motion':'ios android','qorm-biometric':'ios android mac','qorm-bluetooth':'ios android mac',
  'qorm-wifi':'ios android mac','qorm-nfc':'ios android','qorm-vibrate':'ios android web','qorm-torch':'ios android',
  'qorm-volume':'ios android mac linux','qorm-brightness':'ios android mac','qorm-notify':'mac linux web',
  'qorm-dockbadge':'mac','qorm-loginitem':'mac','qorm-screens':'mac linux windows'
};
function qormOnPlatform(p){ window.qormPlatform=p; qormPlatformCheck(p); }
function qormPlatformCheck(platform){
  var missing=[];
  for(var cls in QORM_CAPS){
    if(document.querySelector('.'+cls) && QORM_CAPS[cls].split(' ').indexOf(platform)<0){ missing.push(cls.replace('qorm-','')); }
  }
  if(missing.length) qormPlatformBanner(platform, missing);
}
function qormPlatformBanner(platform, missing){
  if(document.getElementById('qorm-plat-banner')) return;
  var b=document.createElement('div'); b.id='qorm-plat-banner';
  b.style.cssText='position:fixed;top:0;left:0;right:0;z-index:99999;background:#b45309;color:#fff;font-size:13px;line-height:1.4;padding:8px 34px 8px 12px;box-shadow:0 1px 6px rgba(0,0,0,.25);';
  b.textContent='\u26a0 '+missing.length+'feature(s) not available on '+platform+': '+missing.join(', ');
  var x=document.createElement('button'); x.textContent='\u00d7'; x.setAttribute('aria-label','dismiss');
  x.style.cssText='position:absolute;right:6px;top:4px;background:none;border:none;color:#fff;font-size:20px;line-height:1;cursor:pointer;';
  x.onclick=function(){ b.remove(); }; b.appendChild(x); document.body.appendChild(b);
}
// --- native->UI event channel -------------------------------------------------
// The native/lower layer (OS listeners, the Go/WASM middle-layer, another window)
// EMITS a signal; the frontend just SUBSCRIBES. One channel for every push event
// so a widget never polls for something the system can tell it. Built-ins register
// as listeners too, so an app can also listen for the same signals.
window.__qormBus = window.__qormBus || {};
window.__qormQ = window.__qormQ || {};
function qormOn(evt, fn){
  (window.__qormBus[evt] = window.__qormBus[evt] || []).push(fn);
  var q = window.__qormQ[evt]; // deliver events emitted before this listener existed
  if(q && q.length){ window.__qormQ[evt] = []; q.forEach(function(d){ try{ fn(d); }catch(e){} }); }
  return fn;
}
function qormOff(evt, fn){ var a = window.__qormBus[evt]; if(a){ var i = a.indexOf(fn); if(i>=0) a.splice(i,1); } }
function qormEmit(evt, data){
  var a = window.__qormBus[evt];
  if(a && a.length){ a.slice().forEach(function(fn){ try{ fn(data); }catch(e){ if(window.console) console.error('qorm listener '+evt, e); } }); }
  else { var q = (window.__qormQ[evt] = window.__qormQ[evt] || []); q.push(data); if(q.length > 8) q.shift(); } // queue for a late listener
  // surface meaningful events in the Activity log (skip high-frequency sync)
  if(['volume','brightness','mute','tick','insets','hwsync'].indexOf(evt) < 0){
    var det = evt + (data && data.id ? ' ' + data.id : '');
    try{ fetch('/log', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({source:'app', detail:det})}); }catch(e){}
  }
}
function qormHwInit(){
  qormCameraInit();
  if(!window.__qormBusInit){ window.__qormBusInit=1;
    qormOn('volume', function(v){ qormOnVolume(v); });
    qormOn('mute', function(m){ qormOnMute(m); });
    qormOn('brightness', function(v){ qormOnBrightness(v); });
    qormOn('battery', function(d){ if(d&&typeof d==='object') qormOnBattery(d.level, d.charging); });
    qormOn('network', function(d){ qormOnNetwork(typeof d==='string'?d:JSON.stringify(d)); });
  }
  if(!qormHasNative()) return;
  if(document.querySelector('.qorm-volume')) qormToNative('volumeGet');
  if(document.querySelector('.qorm-brightness')) qormToNative('brightnessGet');
  if(document.querySelector('.qorm-battery')) qormToNative('battery');
  if(document.querySelector('.qorm-torch')) qormToNative('torchGet');
  // NOTE: do NOT auto-read bluetoothState on load — CBCentralManager init aborts a packaged .app via TCC on macOS. Bluetooth is click-to-scan.
  if(document.querySelector('.qorm-loginitem')) qormToNative('loginItemGet');
  if(document.querySelector('.qorm-screens')) qormToNative('screens');
  if(qormHasNative()){ qormToNative('platform'); qormToNative('pendingShortcut'); qormToNative('getInsets'); } else { qormPlatformCheck('web'); }
  // keep the externally-mutable readouts in sync (a volume key, a power cable,
  // a Wi-Fi drop) by re-reading them on an interval, not just once on load.
  if(!window.__qormHwSync){ window.__qormHwSync=setInterval(qormHwSync, 3000); }
}
function qormHwSync(){
  if(!qormHasNative() || document.hidden) return;
  if(document.querySelector('.qorm-volume')) qormToNative('volumeGet');
  if(document.querySelector('.qorm-brightness')) qormToNative('brightnessGet');
  if(document.querySelector('.qorm-battery')) qormToNative('battery');
  if(document.querySelector('.qorm-network')) qormToNative('networkStatus');
}
function qormVol(btn,d){ if(qormHasNative()){ qormToNative(d>0?'volumeUp':'volumeDown'); } else { btn.closest('.qorm-volume').querySelector('.qorm-volume-out').textContent='needs the QORM Dev app'; } }
window.__qv={level:0,muted:false};
function qormVolRender(){ document.querySelectorAll('.qorm-volume-out').forEach(function(o){ o.textContent='Volume: '+(window.__qv.muted?'Muted':(Math.round(window.__qv.level*100)+'%%')); }); }
function qormOnVolume(level){ window.__qv.level=level; qormVolRender(); }
function qormOnMute(muted){ window.__qv.muted=!!muted; qormVolRender(); }
function qormBright(btn,d){ if(qormHasNative()){ qormToNative(d>0?'brightnessUp':'brightnessDown'); } else { btn.closest('.qorm-brightness').querySelector('.qorm-brightness-out').textContent='needs the QORM Dev app'; } }
function qormOnBrightness(level){ document.querySelectorAll('.qorm-brightness-out').forEach(function(o){ o.textContent='Brightness: '+Math.round(level*100)+'%%'; }); }
function qormVibrate(btn){ var out=btn.closest('.qorm-vibrate').querySelector('.qorm-vibrate-out'); if(qormHasNative()){ qormToNative('vibrate'); out.textContent='Vibrated '; } else if(navigator.vibrate){ navigator.vibrate(200); out.textContent='Vibrated '; } else { out.textContent='not supported'; } }
function qormTorch(btn){ var out=btn.closest('.qorm-torch').querySelector('.qorm-torch-out'); if(qormHasNative()){ qormToNative('torchToggle'); } else { out.textContent='needs the QORM Dev app'; } }
function qormOnTorch(on){ document.querySelectorAll('.qorm-torch-out').forEach(function(o){ o.textContent='Flashlight: '+(on?'ON':'OFF'); }); }
function qormBattery(btn){ var out=btn.closest('.qorm-battery').querySelector('.qorm-battery-out'); out.textContent='…'; if(qormHasNative()){ qormToNative('battery'); } else if(navigator.getBattery){ navigator.getBattery().then(function(b){ qormOnBattery(b.level, b.charging); }); } else { out.textContent='needs the QORM Dev app'; } }
function qormOnBattery(level,charging){ document.querySelectorAll('.qorm-battery-out').forEach(function(o){ o.textContent='Battery: '+Math.round(level*100)+'%%'+(charging?' ':''); }); }
function qormScreenshot(btn){
  var out=btn.closest('.qorm-screenshot').querySelector('.qorm-screenshot-out'); out.textContent='capturing…';
  if(qormHasNative()){ qormToNative('screenshot'); return; }
  if(navigator.mediaDevices&&navigator.mediaDevices.getDisplayMedia){
    navigator.mediaDevices.getDisplayMedia({video:true}).then(function(stream){
      var v=document.createElement('video'); v.srcObject=stream; v.play();
      v.onloadedmetadata=function(){ var c=document.createElement('canvas'); c.width=v.videoWidth; c.height=v.videoHeight;
        c.getContext('2d').drawImage(v,0,0); stream.getTracks().forEach(function(t){t.stop();});
        qormOnScreenshot(c.toDataURL('image/png')); };
    }).catch(function(e){ out.textContent='denied: '+e.name; });
  } else { out.textContent='not supported here'; }
}
function qormOnScreenshot(dataURL){ document.querySelectorAll('.qorm-screenshot-out').forEach(function(o){ o.innerHTML = dataURL ? '<img src="'+dataURL+'" style="max-width:100%%;border-radius:8px;display:block">' : 'capture failed'; }); }
var __qormRec=null, __qormRecChunks=[];
function qormScreenRecord(btn){
  var box=btn.closest('.qorm-screenrecord'), out=box.querySelector('.qorm-screenrecord-out');
  if(qormHasNative()){ var on=box.getAttribute('data-rec')==='1'; box.setAttribute('data-rec',on?'0':'1'); btn.textContent=on?'Start Recording':'Stop Recording'; qormToNative(on?'screenRecordStop':'screenRecordStart'); return; }
  if(!__qormRec){
    if(!(navigator.mediaDevices&&navigator.mediaDevices.getDisplayMedia&&window.MediaRecorder)){ out.textContent='not supported here'; return; }
    navigator.mediaDevices.getDisplayMedia({video:true,audio:true}).then(function(stream){
      __qormRecChunks=[]; __qormRec=new MediaRecorder(stream);
      __qormRec.ondataavailable=function(e){ if(e.data.size) __qormRecChunks.push(e.data); };
      __qormRec.onstop=function(){ stream.getTracks().forEach(function(t){t.stop();});
        var blob=new Blob(__qormRecChunks,{type:'video/webm'}); var url=URL.createObjectURL(blob);
        out.innerHTML='<video src="'+url+'" controls style="max-width:100%%;border-radius:8px;display:block"></video>'; __qormRec=null; };
      __qormRec.start(); out.textContent='recording…'; btn.textContent='Stop Recording';
    }).catch(function(e){ out.textContent='denied: '+e.name; });
  } else { __qormRec.stop(); btn.textContent='Start Recording'; }
}
function qormOnScreenRecord(msg){ document.querySelectorAll('.qorm-screenrecord-out').forEach(function(o){ o.textContent=msg||''; }); }
function qormShare(btn){ var out=btn.closest('.qorm-share').querySelector('.qorm-share-out'); var d={text:'Shared from my QORM app',url:location.href};
  if(qormHasNative()){ qormToNative('share',d); out.textContent='opening share sheet…'; }
  else if(navigator.share){ navigator.share(d).then(function(){out.textContent='shared ';}).catch(function(e){out.textContent=e.name==='AbortError'?'cancelled':'error';}); }
  else { out.textContent='share not supported here'; } }
function qormOnShare(ok){ document.querySelectorAll('.qorm-share-out').forEach(function(o){ o.textContent=ok?'shared':'cancelled'; }); }
function qormClipboard(btn){ var out=btn.closest('.qorm-clipboard').querySelector('.qorm-clipboard-out'); var text='QORM  '+new Date().toLocaleTimeString();
  if(qormHasNative()){ qormToNative('clipboardSet',{text:text}); out.textContent='copied: '+text; }
  else if(navigator.clipboard){ navigator.clipboard.writeText(text).then(function(){out.textContent='copied: '+text;}).catch(function(){out.textContent='denied';}); }
  else { out.textContent='clipboard not supported'; } }
function qormOnClipboard(text){ document.querySelectorAll('.qorm-clipboard-out').forEach(function(o){ o.textContent='clipboard: '+text; }); }
function qormDeviceInfo(btn){ var out=btn.closest('.qorm-deviceinfo').querySelector('.qorm-deviceinfo-out'); out.textContent='…';
  if(qormHasNative()){ qormToNative('deviceInfo'); }
  else { qormOnDeviceInfo(JSON.stringify({platform:'web',ua:navigator.platform,lang:navigator.language,screen:screen.width+'x'+screen.height})); } }
function qormOnDeviceInfo(json){ var d; try{d=JSON.parse(json);}catch(e){d={};} var t=Object.keys(d).map(function(k){return k+': '+d[k];}).join('\n'); document.querySelectorAll('.qorm-deviceinfo-out').forEach(function(o){ o.textContent=t||'—'; }); }
function qormNetwork(btn){ var out=btn.closest('.qorm-network').querySelector('.qorm-network-out'); out.textContent='…';
  if(qormHasNative()){ qormToNative('networkStatus'); }
  else { qormOnNetwork(JSON.stringify({online:navigator.onLine,type:(navigator.connection&&navigator.connection.effectiveType)||'unknown'})); } }
function qormOnNetwork(json){ var d; try{d=JSON.parse(json);}catch(e){d={};} document.querySelectorAll('.qorm-network-out').forEach(function(o){ o.textContent=(d.online?'online':'offline')+' · '+(d.type||'?'); }); }
function qormKeepAwake(btn){ var box=btn.closest('.qorm-keepawake'), out=box.querySelector('.qorm-keepawake-out'); var on=box.getAttribute('data-on')==='1'; box.setAttribute('data-on',on?'0':'1'); btn.textContent=on?'Keep Screen Awake':'Allow Sleep';
  if(qormHasNative()){ qormToNative('keepAwake',{on:!on}); out.textContent=on?'sleep allowed':'staying awake '; }
  else if('wakeLock'in navigator){ if(!on){ navigator.wakeLock.request('screen').then(function(w){ window.__qormWake=w; out.textContent='staying awake '; }).catch(function(){out.textContent='denied';}); } else { if(window.__qormWake){window.__qormWake.release();window.__qormWake=null;} out.textContent='sleep allowed'; } }
  else { out.textContent='wake lock not supported'; } }
function qormHaptic(btn){ var out=btn.closest('.qorm-haptics').querySelector('.qorm-haptics-out'); var type=btn.getAttribute('data-type')||'success';
  if(qormHasNative()){ qormToNative('haptic',{type:type}); out.textContent='haptic: '+type; }
  else if(navigator.vibrate){ navigator.vibrate(type==='error'?[80,40,80]:type==='warning'?[40,40]:30); out.textContent='vibrated: '+type; }
  else { out.textContent='haptics not supported'; } }
function qormStorage(btn){ var out=btn.closest('.qorm-storage').querySelector('.qorm-storage-out'); var v='saved@'+new Date().toLocaleTimeString();
  if(qormHasNative()){ qormToNative('storageSet',{key:'qorm_demo',value:v}); qormToNative('storageGet',{key:'qorm_demo'}); out.textContent='saving…'; }
  else { try{ localStorage.setItem('qorm_demo',v); qormOnStorage('qorm_demo', localStorage.getItem('qorm_demo')); }catch(e){ out.textContent='storage denied'; } } }
function qormOnStorage(key,value){ document.querySelectorAll('.qorm-storage-out').forEach(function(o){ o.textContent=key+' = '+value; }); }
var __qormSR=null;
function qormListen(btn){ var out=btn.closest('.qorm-stt').querySelector('.qorm-stt-out'); var lang=btn.getAttribute('data-lang')||navigator.language||'en-US';
  if(qormHasNative()){ qormToNative('listenStart',{lang:lang}); out.textContent='listening'; return; }
  var SR=window.SpeechRecognition||window.webkitSpeechRecognition;
  if(!SR){ out.textContent='STT not supported here'; return; }
  __qormSR=new SR(); __qormSR.interimResults=true; __qormSR.lang=lang;
  __qormSR.onresult=function(e){ var t=''; for(var i=0;i<e.results.length;i++) t+=e.results[i][0].transcript; qormOnSpeech(t); };
  __qormSR.onerror=function(e){ out.textContent='error: '+e.error; };
  __qormSR.start(); out.textContent='listening'; }
function qormOnSpeech(text){ document.querySelectorAll('.qorm-stt-out').forEach(function(o){ o.textContent = text||'(no speech)'; }); }
function qormSecureSave(btn){ var out=btn.closest('.qorm-securestorage').querySelector('.qorm-securestorage-out'); var v='secret@'+new Date().toLocaleTimeString();
  if(qormHasNative()){ qormToNative('secureSet',{key:'qorm_secret',value:v}); qormToNative('secureGet',{key:'qorm_secret'}); out.textContent='saving securely'; }
  else { try{ localStorage.setItem('qorm_secret',v); qormOnSecure('qorm_secret', localStorage.getItem('qorm_secret')); }catch(e){ out.textContent='denied'; } } }
function qormOnSecure(key,value){ document.querySelectorAll('.qorm-securestorage-out').forEach(function(o){ o.textContent='secure['+key+'] = '+value; }); }
function qormPickFile(btn){ var out=btn.closest('.qorm-filepicker').querySelector('.qorm-filepicker-out');
  if(qormHasNative()){ qormToNative('pickFile'); out.textContent='opening picker'; return; }
  var inp=document.createElement('input'); inp.type='file';
  inp.onchange=function(){ var f=inp.files[0]; if(!f) return; var r=new FileReader(); r.onload=function(){ qormOnFile(JSON.stringify({name:f.name,size:f.size,dataURL:r.result})); }; r.readAsDataURL(f); };
  inp.click(); }
function qormOnFile(json){ var d; try{d=JSON.parse(json);}catch(e){d={};} document.querySelectorAll('.qorm-filepicker-out').forEach(function(o){ o.textContent = d.name ? (d.name+' ('+(d.size||0)+' bytes)') : 'no file'; }); }
function qormPickPhoto(btn){ var out=btn.closest('.qorm-photopicker').querySelector('.qorm-photopicker-out');
  if(qormHasNative()){ qormToNative('pickPhoto'); out.textContent='opening picker'; return; }
  var inp=document.createElement('input'); inp.type='file'; inp.accept='image/*';
  inp.onchange=function(){ var f=inp.files[0]; if(!f) return; var r=new FileReader(); r.onload=function(){ qormOnPhoto(r.result); }; r.readAsDataURL(f); };
  inp.click(); }
function qormOnPhoto(dataURL){ document.querySelectorAll('.qorm-photopicker-out').forEach(function(o){ o.innerHTML = dataURL ? '<img src="'+dataURL+'" style="max-width:100%%;border-radius:8px;display:block">' : 'no photo'; }); }
function qormOrientation(btn){ var box=btn.closest('.qorm-orientation'), out=box.querySelector('.qorm-orientation-out'); var mode=box.getAttribute('data-mode')==='landscape'?'portrait':'landscape'; box.setAttribute('data-mode',mode); btn.textContent='Lock '+(mode==='portrait'?'Landscape':'Portrait');
  if(qormHasNative()){ qormToNative('lockOrientation',{mode:mode}); out.textContent='locked '+mode; }
  else if(screen.orientation&&screen.orientation.lock){ screen.orientation.lock(mode).then(function(){out.textContent='locked '+mode;}).catch(function(e){out.textContent='needs fullscreen'; }); }
  else { out.textContent='orientation lock not supported'; } }
var __qormVR=null,__qormVRChunks=[];
function qormRecordVideo(btn){ var box=btn.closest('.qorm-videocapture'), out=box.querySelector('.qorm-videocapture-out');
  if(qormHasNative()){ qormToNative('recordVideo'); out.textContent='opening camera'; return; }
  if(!__qormVR){
    if(!(navigator.mediaDevices&&window.MediaRecorder)){ out.textContent='not supported here'; return; }
    navigator.mediaDevices.getUserMedia({video:true,audio:true}).then(function(stream){
      __qormVRChunks=[]; __qormVR=new MediaRecorder(stream);
      __qormVR.ondataavailable=function(e){ if(e.data.size) __qormVRChunks.push(e.data); };
      __qormVR.onstop=function(){ stream.getTracks().forEach(function(t){t.stop();}); qormOnVideo(URL.createObjectURL(new Blob(__qormVRChunks,{type:'video/webm'}))); __qormVR=null; };
      __qormVR.start(); out.textContent='recording'; btn.textContent='Stop';
    }).catch(function(e){ out.textContent='denied'; });
  } else { __qormVR.stop(); btn.textContent='Record Video'; } }
function qormOnVideo(url){ document.querySelectorAll('.qorm-videocapture-out').forEach(function(o){ o.innerHTML = url ? '<video src="'+url+'" controls style="max-width:100%%;border-radius:8px;display:block"></video>' : 'no video'; }); }
function qormScanQR(btn){ var out=btn.closest('.qorm-qrscan').querySelector('.qorm-qrscan-out');
  if(qormHasNative()){ qormToNative('scanQR'); out.textContent='scanning'; return; }
  if(!('BarcodeDetector' in window)){ out.textContent='QR scan not supported here'; return; }
  navigator.mediaDevices.getUserMedia({video:{facingMode:'environment'}}).then(function(stream){
    var v=document.createElement('video'); v.srcObject=stream; v.setAttribute('playsinline',''); v.play();
    v.style.cssText='max-width:100%%;border-radius:8px'; out.innerHTML=''; out.appendChild(v);
    var det=new BarcodeDetector(); var stop=false;
    (function loop(){ if(stop) return; det.detect(v).then(function(codes){ if(codes.length){ stop=true; stream.getTracks().forEach(function(t){t.stop();}); qormOnScan(codes[0].rawValue); } else setTimeout(loop,300); }).catch(function(){ setTimeout(loop,300); }); })();
  }).catch(function(e){ out.textContent='camera denied'; }); }
function qormOnScan(text){ document.querySelectorAll('.qorm-qrscan-out').forEach(function(o){ o.textContent = text ? ('scanned: '+text) : 'no code'; }); }
function qormSpeak(btn){ var out=btn.closest('.qorm-tts').querySelector('.qorm-tts-out'); var text=btn.getAttribute('data-text')||'Hello from your QORM app.'; var lang=btn.getAttribute('data-lang')||navigator.language||'en-US';
  if(qormHasNative()){ qormToNative('speak',{text:text,lang:lang}); out.textContent='speaking'; }
  else if(window.speechSynthesis){ window.speechSynthesis.cancel(); var u=new SpeechSynthesisUtterance(text); u.lang=lang; window.speechSynthesis.speak(u); out.textContent='speaking'; }
  else { out.textContent='TTS not supported'; } }
// Canonical full-word trigger aliases — the abbreviated qormVol/qormGeo/... stay
// for existing rendered HTML, but qorm<Capability> is the documented, derivable
// name a developer or agent can reach for without memorizing an abbreviation.
var qormVolume=qormVol,qormLocation=qormGeo,qormRecorder=qormRec,qormBiometric=qormBio,qormBrightness=qormBright,qormSensors=qormMotion;
function qormHeading(btn){ var out=btn.closest('.qorm-compass').querySelector('.qorm-compass-out');
  if(qormHasNative()){ qormToNative('headingStart'); out.textContent='reading'; return; }
  function h(e){ var d=e.webkitCompassHeading!=null?e.webkitCompassHeading:(e.alpha!=null?360-e.alpha:null); if(d!=null) qormOnHeading(d); }
  if(window.DeviceOrientationEvent){ window.addEventListener('deviceorientationabsolute',h,{once:false}); window.addEventListener('deviceorientation',h,{once:false}); out.textContent='reading'; }
  else { out.textContent='compass not supported here'; } }
function qormOnHeading(deg){ document.querySelectorAll('.qorm-compass-out').forEach(function(o){ o.textContent=Math.round(deg)+'°'; }); }
function qormProximity(btn){ var out=btn.closest('.qorm-proximity').querySelector('.qorm-proximity-out'); if(qormHasNative()){ qormToNative('proximityStart'); out.textContent='reading'; } else { out.textContent='needs the QORM app'; } }
function qormOnProximity(near){ document.querySelectorAll('.qorm-proximity-out').forEach(function(o){ o.textContent=near?'near':'far'; }); }
function qormPedometer(btn){ var out=btn.closest('.qorm-pedometer').querySelector('.qorm-pedometer-out'); if(qormHasNative()){ qormToNative('pedometerStart'); out.textContent='counting'; } else { out.textContent='needs the QORM app'; } }
function qormOnSteps(n){ document.querySelectorAll('.qorm-pedometer-out').forEach(function(o){ o.textContent=n+' steps'; }); }
function qormBarometer(btn){ var out=btn.closest('.qorm-barometer').querySelector('.qorm-barometer-out'); if(qormHasNative()){ qormToNative('barometerStart'); out.textContent='reading'; } else { out.textContent='needs the QORM app'; } }
function qormOnPressure(kpa){ document.querySelectorAll('.qorm-barometer-out').forEach(function(o){ o.textContent=(+kpa).toFixed(2)+' kPa'; }); }
function qormPickContact(btn){ var out=btn.closest('.qorm-contacts').querySelector('.qorm-contacts-out');
  if(qormHasNative()){ qormToNative('pickContact'); out.textContent='opening picker'; return; }
  if(navigator.contacts&&navigator.contacts.select){ navigator.contacts.select(['name','tel'],{multiple:false}).then(function(cs){ if(cs.length){ qormOnContact(JSON.stringify({name:(cs[0].name||[''])[0],phone:(cs[0].tel||[''])[0]})); } }).catch(function(){ out.textContent='cancelled'; }); }
  else { out.textContent='contact picker not supported here'; } }
function qormOnContact(json){ var d; try{d=JSON.parse(json);}catch(e){d={};} document.querySelectorAll('.qorm-contacts-out').forEach(function(o){ o.textContent=(d.name||'?')+' '+(d.phone||''); }); }
function qormAddEvent(btn){ var out=btn.closest('.qorm-calendar').querySelector('.qorm-calendar-out'); if(qormHasNative()){ qormToNative('addEvent',{title:'QORM Event'}); out.textContent='adding'; } else { out.textContent='needs the QORM app'; } }
function qormOnCalendar(msg){ document.querySelectorAll('.qorm-calendar-out').forEach(function(o){ o.textContent=msg||''; }); }
function qormGetModes(btn){ var out=btn.closest('.qorm-systemmodes').querySelector('.qorm-systemmodes-out');
  if(qormHasNative()){ qormToNative('getModes'); out.textContent='reading'; return; }
  var m={lowPower:null,darkMode:window.matchMedia&&window.matchMedia('(prefers-color-scheme: dark)').matches,airplane:null,dnd:null,online:navigator.onLine};
  qormOnModes(JSON.stringify(m)); }
function qormOnModes(json){ var d; try{d=JSON.parse(json);}catch(e){d={};}
  var parts=[]; function add(k,v){ if(v===null||v===undefined) return; parts.push(k+': '+(v===true?'on':v===false?'off':v)); }
  add('low-power',d.lowPower); add('dark',d.darkMode); add('airplane',d.airplane); add('DND',d.dnd);
  document.querySelectorAll('.qorm-systemmodes-out').forEach(function(o){ o.textContent=parts.join('  ·  ')||'no modes readable here'; }); }
function qormUpdateWidget(title, lines){ if(qormHasNative()){ qormToNative('updateWidget',{title:title,lines:lines||[]}); return true; } return false; }
function qormOnWidget(msg){}
function qormReadCSSInsets(){ var d=document.createElement('div'); d.style.cssText='position:fixed;top:0;left:0;padding-top:env(safe-area-inset-top);padding-bottom:env(safe-area-inset-bottom);padding-left:env(safe-area-inset-left);padding-right:env(safe-area-inset-right);visibility:hidden;'; document.body.appendChild(d); var s=getComputedStyle(d); var r={top:parseFloat(s.paddingTop)||0,bottom:parseFloat(s.paddingBottom)||0,left:parseFloat(s.paddingLeft)||0,right:parseFloat(s.paddingRight)||0}; d.parentNode.removeChild(d); return r; }
function qormGetInsets(btn){ var out=btn.closest('.qorm-insets').querySelector('.qorm-insets-out'); if(qormHasNative()){ qormToNative('getInsets'); out.textContent='reading'; return; } qormOnInsets(JSON.stringify(qormReadCSSInsets())); }
function qormOnInsets(json){ var d; try{d=JSON.parse(json);}catch(e){d={};} document.querySelectorAll('.qorm-insets-out').forEach(function(o){ o.textContent='top '+(d.top||0)+' · bottom '+(d.bottom||0)+' · left '+(d.left||0)+' · right '+(d.right||0); });
  document.documentElement.style.setProperty('--safe-top',(d.top||0)+'px'); document.documentElement.style.setProperty('--safe-bottom',(d.bottom||0)+'px'); document.documentElement.style.setProperty('--safe-left',(d.left||0)+'px'); document.documentElement.style.setProperty('--safe-right',(d.right||0)+'px'); }
// Chromeless-window dragging: a [data-qorm-drag] region moves the desktop window.
if (typeof document !== 'undefined') document.addEventListener('mousedown', function(e){
  if (e.button !== 0 || !window.qormDesktop) return;
  var h = e.target.closest && e.target.closest('[data-qorm-drag]');
  if (!h) return;
  var sx = e.screenX, sy = e.screenY;
  qormToNative('winDragStart');
  function mv(ev){ qormToNative('winDragMove', {dx: ev.screenX - sx, dy: ev.screenY - sy}); }
  function up(){ document.removeEventListener('mousemove', mv); document.removeEventListener('mouseup', up); }
  document.addEventListener('mousemove', mv); document.addEventListener('mouseup', up);
});
// Desktop right-click context menu: position at cursor, hover submenus, select.
if (typeof document !== 'undefined') {
  document.addEventListener('contextmenu', function(e){
    var host = e.target.closest && e.target.closest('.qorm-ctxmenu');
    if(!host) return;
    e.preventDefault();
    document.querySelectorAll('.qorm-ctxmenu-panel').forEach(function(p){ p.style.display='none'; });
    var panel = host.querySelector('.qorm-ctxmenu-panel');
    if(!panel) return;
    panel.style.display='block';
    var x=Math.min(e.clientX, window.innerWidth - panel.offsetWidth - 8);
    var y=Math.min(e.clientY, window.innerHeight - panel.offsetHeight - 8);
    panel.style.left=Math.max(4,x)+'px'; panel.style.top=Math.max(4,y)+'px';
  });
  document.addEventListener('click', function(e){
    var item = e.target.closest && e.target.closest('.qorm-ctxmenu-item');
    if(item && !item.parentElement.classList.contains('qorm-ctxmenu-sub')){
      var id=item.getAttribute('data-id'); if(id) qormEmit('context', {id:id});
    }
    if(!(e.target.closest && e.target.closest('.qorm-ctxmenu-sub')))
      document.querySelectorAll('.qorm-ctxmenu-panel').forEach(function(p){ p.style.display='none'; });
  });
  document.addEventListener('mouseover', function(e){
    if(!(e.target.closest && e.target.closest('.qorm-ctxmenu-panel'))) return;
    var sub = e.target.closest('.qorm-ctxmenu-sub');
    document.querySelectorAll('.qorm-ctxmenu-subpanel').forEach(function(p){ if(!sub || !sub.contains(p)) p.style.display='none'; });
    if(sub){ var sp=sub.querySelector('.qorm-ctxmenu-subpanel'); if(sp) sp.style.display='block'; }
  });
  document.addEventListener('keydown', function(e){ if(e.key==='Escape') document.querySelectorAll('.qorm-ctxmenu-panel').forEach(function(p){ p.style.display='none'; }); });
}
function qormOpenUrl(btn){ var out=btn.closest('.qorm-openurl').querySelector('.qorm-openurl-out'); var url=btn.getAttribute('data-url')||'https://example.com';
  if(qormHasNative()){ qormToNative('openURL',{url:url}); out.textContent='opening '+url; }
  else { window.open(url,'_blank'); out.textContent='opened '+url; } }
function qormOnOpenUrl(ok){ document.querySelectorAll('.qorm-openurl-out').forEach(function(o){ o.textContent=ok?'opened':'could not open'; }); }
function qormNfc(btn){ var out=btn.closest('.qorm-nfc').querySelector('.qorm-nfc-out'); out.textContent='Hold a tag near the phone…'; if(qormHasNative()){ qormToNative('nfcRead'); } else { out.textContent='NFC needs the QORM Dev app'; } }
function qormOnNfc(json){ var d; try{ d=JSON.parse(json); }catch(e){ d={}; } document.querySelectorAll('.qorm-nfc-out').forEach(function(o){ o.textContent = d.error ? d.error : ('Tag: '+(d.text||d.id||'read')); }); }
function qormWifi(btn){ var out=btn.closest('.qorm-wifi').querySelector('.qorm-wifi-out'); out.textContent='…'; if(qormHasNative()){ qormToNative('wifiInfo'); } else { out.textContent='Wi-Fi needs the QORM Dev app'; } }
function qormOnWifi(json){ var d; try{ d=JSON.parse(json); }catch(e){ d={}; }
  document.querySelectorAll('.qorm-wifi-out').forEach(function(o){ o.textContent = d.error ? d.error : ('SSID: '+(d.ssid||'unknown')+(typeof d.networks!=='undefined' ? ('\n'+d.networks+'networks nearby') : '')); }); }
function qormOnMotion(a,b,g){ document.querySelectorAll('.qorm-motion .qorm-motion-out').forEach(function(o){ o.textContent='α '+Math.round(a)+'°  β '+Math.round(b)+'°  γ '+Math.round(g)+'°'; }); }
// Audio recorder: getUserMedia + MediaRecorder, toggling record/stop; the clip
// is played inline and synced (data URL) into bound state.
function qormRec(btn){
  var box=btn.closest('.qorm-recorder');
  if(qormHasMobileNative()){
    if(box._recording){ qormToNative('recordStop'); btn.textContent='Record'; btn.style.background='var(--danger)'; box._recording=false; }
    else { qormToNative('recordStart'); btn.textContent='Stop'; btn.style.background='#555'; box._recording=true; }
    return;
  }
  if(box._mr && box._mr.state==='recording'){ box._mr.stop(); btn.textContent='Record'; btn.style.background='var(--danger)'; return; }
  navigator.mediaDevices.getUserMedia({audio:true}).then(function(stream){
    var chunks=[], mr=new MediaRecorder(stream); box._mr=mr;
    mr.ondataavailable=function(e){ if(e.data.size) chunks.push(e.data); };
    mr.onstop=function(){
      stream.getTracks().forEach(function(t){ t.stop(); });
      var blob=new Blob(chunks, {type: mr.mimeType || 'audio/webm'}), rd=new FileReader();
      rd.onload=function(){
        var au=box.querySelector('.qorm-rec-audio'); if(au){ au.src=rd.result; au.style.display='block'; }
        var hid=box.querySelector('input[type=hidden]'); if(hid){ hid.value=rd.result; } qorm(-1);
      };
      rd.readAsDataURL(blob);
    };
    mr.start(); btn.textContent='Stop'; btn.style.background='#555';
  }).catch(function(e){ alert('Microphone error: '+e); });
}
function qormOnAudio(dataURL){
  document.querySelectorAll('.qorm-recorder').forEach(function(box){
    var au=box.querySelector('.qorm-rec-audio'); if(au){ au.src=dataURL; au.style.display='block'; }
    var hid=box.querySelector('input[type=hidden]'); if(hid){ hid.value=dataURL; }
    var btn=box.querySelector('button'); if(btn){ btn.textContent='Record'; btn.style.background='var(--danger)'; }
    box._recording=false;
  });
  qorm(-1);
}
function qormOnAudioError(msg){ document.querySelectorAll('.qorm-recorder').forEach(function(box){ var b=box.querySelector('button'); if(b){b.textContent='Record';b.style.background='var(--danger)';} box._recording=false; }); alert('Recorder: '+msg); }
// Client-side tab switching (no server round-trip).
function qormTab(btn){
  var bar=btn.parentNode, panels=bar.parentNode.querySelectorAll('.qorm-tabpanel');
  bar.querySelectorAll('.qorm-tab').forEach(function(b){ b.classList.remove('qorm-tab-active'); });
  btn.classList.add('qorm-tab-active');
  var idx=btn.getAttribute('data-tab');
  panels.forEach(function(p){ p.style.display = (p.getAttribute('data-panel')===idx)?'block':'none'; });
}
// Accordion: toggle the panel following the clicked header.
function qormAcc(btn){
  var p=btn.nextElementSibling;
  if(p){ p.style.display = (p.style.display==='none')?'block':'none'; }
}
// Menu: toggle the dropdown panel; close others.
function qormMenu(btn){
  var panel=btn.nextElementSibling;
  document.querySelectorAll('.qorm-menu-panel').forEach(function(p){ if(p!==panel) p.style.display='none'; });
  if(panel){ panel.style.display = (panel.style.display==='none')?'block':'none'; }
}
// Context menu (CupertinoContextMenu): long-press to reveal the action panel.
function qormCtx(el){
  var t=null, panel=el.querySelector('.qorm-ctx-panel');
  el.addEventListener('pointerdown',function(){ t=setTimeout(function(){ if(panel){ panel.style.display='flex'; } },480); });
  ['pointerup','pointerleave','pointermove'].forEach(function(ev){ el.addEventListener(ev,function(){ if(t){ clearTimeout(t); t=null; } }); });
}
// Pull-to-refresh (RefreshIndicator): drag down from the top past threshold to
// fire handler h.
function qormRefresh(el,h){
  var y0=null, dy=0, sp=el.querySelector('.qorm-refresh-spin');
  el.addEventListener('pointerdown',function(e){ if(el.scrollTop<=0){ y0=e.clientY; } });
  el.addEventListener('pointermove',function(e){ if(y0===null) return; dy=Math.max(0,e.clientY-y0);
    if(sp){ sp.style.height=Math.min(dy,60)+'px'; sp.style.opacity=Math.min(1,dy/60); } });
  var end=function(){ if(y0===null) return; var go=dy>70; if(sp){ sp.style.height=''; sp.style.opacity=''; }
    y0=null; dy=0; if(go) qorm(h); };
  el.addEventListener('pointerup',end); el.addEventListener('pointerleave',end);
}
// Swipe-to-dismiss (Dismissible): drag the content left; past threshold,
// collapse the row and fire handler h (onDismissed).
function qormSwipe(el,h){
  var c=el.querySelector('.qorm-dismiss-content'); if(!c) return;
  var x0=null,dx=0;
  el.addEventListener('pointerdown',function(e){ x0=e.clientX; c.style.transition='none'; });
  el.addEventListener('pointermove',function(e){ if(x0===null) return; dx=Math.min(0,e.clientX-x0); c.style.transform='translateX('+dx+'px)'; });
  var end=function(){ if(x0===null) return; c.style.transition='transform .2s';
    if(dx<-100){ el.style.height=el.offsetHeight+'px'; el.style.overflow='hidden';
      requestAnimationFrame(function(){ el.style.height='0'; el.style.opacity='0'; }); setTimeout(function(){ qorm(h); },210); }
    else { c.style.transform='translateX(0)'; } x0=null; dx=0; };
  el.addEventListener('pointerup',end); el.addEventListener('pointerleave',end);
}
// Long-press: fire handler h after 500ms of a sustained press (GestureDetector).
function qormLong(el,h){
  var t=null;
  var start=function(){ t=setTimeout(function(){ t=null; qorm(h); },500); };
  var cancel=function(){ if(t){ clearTimeout(t); t=null; } };
  el.addEventListener('pointerdown',start);
  el.addEventListener('pointerup',cancel);
  el.addEventListener('pointerleave',cancel);
}
// Live-sync: observe out-of-band changes (e.g. an AI agent editing the same
// session over /mcp) and swap in the new UI. Prefer Server-Sent Events for
// instant multi-client push; fall back to polling.
var __rev=%d;
function qormTheme(t){ if(!t) return; var st=document.getElementById('qorm-stage'); if(st) st.className='qorm-theme-'+t; }
function qormApply(d){
  if(d&&d.theme) qormTheme(d.theme);
  if(!d||typeof d.rev==='undefined') return;
  if(d.rev<=__rev) return;   // already applied (e.g. via the POST /event response) — no double morph
  __rev=d.rev;
  window.__qormEditSrc=d.source;   // so morph can flag AI-touched nodes for a flash
  if(typeof d.html!=='undefined'){ qormMorphInto(document.getElementById('qorm-root'), d.html); }
  window.__qormEditSrc=null;
  if(d.source==='agent') qormPresence(d.detail);   // a collaborator (AI) edited — show it live
  __rev=d.rev;
}
// Live edit attribution: when the AI edits the shared app, the human sees it.
function qormPresence(detail){
  var el=document.getElementById('qorm-presence');
  if(!el){ el=document.createElement('div'); el.id='qorm-presence'; document.body.appendChild(el); }
  el.innerHTML='<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M13 2L4 14h7l-1 8 9-12h-7z"/></svg><span>AI edited'+(detail?' · '+detail:'')+'</span>';
  el.classList.add('show');
  clearTimeout(el._t); el._t=setTimeout(function(){ el.classList.remove('show'); }, 2600);
}
if(window.EventSource){
  var es=new EventSource('/events');
  es.onmessage=function(e){ try{ qormApply(JSON.parse(e.data)); }catch(_){} };
}else{
  setInterval(function(){
    fetch('/poll?rev='+__rev).then(function(r){return r.json();}).then(qormApply).catch(function(){});
  }, 800);
}
// Human presence: report the element the human focuses or taps, so the agent can
// see (via qorm_activity) what the human is attending to — the reverse direction
// of the "AI edited" flash. Only the nearest interactive element, deduped.
(function(){
  var last='';
  function ping(el){
    var t=el&&el.closest&&el.closest('button,a,input,textarea,select,[data-state]');
    if(!t) return;
    var isPw=(t.tagName==='INPUT' && t.type==='password');
    var lab=(t.getAttribute('aria-label')||(isPw?'password':t.getAttribute('placeholder'))||t.textContent||'').replace(/\s+/g,' ').trim().slice(0,40);
    var d=t.tagName.toLowerCase()+(lab?': '+lab:'');
    // include what the human typed — but a password's value is never sent, only
    // a "(hidden)" marker so the agent knows the field was filled, not its content
    if(isPw){ if(t.value) d+=' = (hidden)'; }
    else if((t.tagName==='INPUT'||t.tagName==='TEXTAREA'||t.tagName==='SELECT') && t.value){ d+=' = '+String(t.value).slice(0,60); }
    if(d===last) return; last=d;
    fetch('/presence',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({element:d})}).catch(function(){});
  }
  document.addEventListener('focusin',function(e){ ping(e.target); });
  document.addEventListener('pointerdown',function(e){ ping(e.target); });
  document.addEventListener('input',function(e){ ping(e.target); });   // live typing
})();
if(document.readyState!=='loading'){ setTimeout(qormMeasure,60); setTimeout(qormHwInit,300); } else { window.addEventListener('load',function(){ setTimeout(qormMeasure,60); setTimeout(qormHwInit,300); }); }
</script>
</body>
</html>`, lang, dir, htmlEscape(title), width, height, theme, body, rev)
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
